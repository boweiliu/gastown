# Worker Lifecycle and Sweep Coordination

> **Bead:** gt-t6muy
> **Date:** 2026-02-20
> **Author:** capable (gastown worker)
> **Status:** Implemented — core lifecycle shipped, branch cleanup shipped, coordinator notify pending
> **Updated:** 2026-03-07 (gt-o8g8 implementation audit by bear)
> **Related:** gt-dtw9u (Watcher monitoring), gt-qpwv4 (Completion detection),
> gt-6qyt1 (Merger queue), gt-budeb (Auto-nuke), gt-5j3ia (Swarm aggregation),
> gt-1dbcp (Worker auto-start), w-gt-004 (Archive lifecycle item)

---

## 1. Overview

This document formalizes how Supervisor, Watcher, Merger, and Workers coordinate
to move work through the Gas Town propulsion system. It captures the
session-per-step model, defines the two cleanup stages, designs the per-project
lifecycle channel, and resolves open design questions about step granularity,
recycling, and spawning.

**Core insight:** Workers do NOT complete complex workflows end-to-end. Instead,
each workflow step gets one worker session. The sandbox (branch, worktree)
persists across sessions. Sessions are the pistons; sandboxes are the cylinders.

---

## 2. Session-Per-Step Model

### 2.1 The Relay Race

See [concepts/worker-lifecycle.md](../concepts/worker-lifecycle.md) for the relay race model.

### 2.2 Session Cycling vs Step Cycling

These are distinct concepts:

| Concept | Trigger | What Changes | What Persists |
|---------|---------|-------------|---------------|
| **Session cycle** | Transfer, compaction, crash | Claude context window | Branch, worktree, workflow state |
| **Step cycle** | Step bead closed | Current step focus | Branch, worktree, remaining steps |

A single step may span multiple session cycles (if the step is complex or
compaction occurs). Multiple steps may fit in a single session (if steps are
small and context permits). The session-per-step model is a design target, not a
hard constraint.

### 2.3 When Sessions Cycle

| Trigger | Who Initiates | What Happens |
|---------|--------------|-------------|
| Step completion | Worker | `bd close <step>` then `gt transfer` for next step |
| Context filling | Claude Code | Auto-compaction; PreCompact hook saves state |
| Crash/timeout | Infrastructure | Watcher detects, respawns session |
| `gt done` | Worker | Final step; submit to MQ, go idle (sandbox preserved) |

### 2.4 State Continuity

Between sessions, state is preserved through:

- **Git state:** Commits, staged changes, branch position
- **Beads state:** Workflow progress (which steps are closed)
- **Hook state:** `assignment_bead` on agent bead persists across sessions
- **Agent bead:** `agent_state`, `cleanup_status`, `assignment_bead` fields

The new session discovers its position via:

```bash
gt prime --hook    # Loads role context, reads hook
bd workflow current     # Discovers which step is next
bd show <step-id>  # Reads step instructions
```

No explicit "transfer payload" is needed. The beads state IS the transfer.

---

## 3. Two Cleanup Stages

### 3.1 Step Cleanup (Session Dies, Sandbox Lives)

Triggered when a step completes but more steps remain in the workflow.

| Action | Result |
|--------|--------|
| Close step bead | `bd close <step-id>` |
| Session cycles | `gt transfer` (voluntary) or crash recovery |
| Sandbox persists | Branch, worktree, uncommitted work all survive |
| Workflow persists | Remaining steps still open, hook still set |
| Identity persists | Agent bead unchanged, CV accumulates |

**Who handles it:**
- Worker initiates via `gt transfer`
- Watcher respawns if crash (via `SessionManager.Start`)
- Daemon triggers if session is dead (`LIFECYCLE:Shutdown` → watcher)

### 3.2 Workflow Cleanup (Worker Goes Idle)

Triggered when the workflow's final step completes and work is submitted.

| Action | Result |
|--------|--------|
| Worker runs `gt done` | Pushes branch, submits MR, sets `cleanup_status=clean` |
| Worker sets agent state | `agent_state=idle`, `assignment_bead` cleared |
| Worker kills session | Session terminated, sandbox preserved |
| Watcher receives `worker_done` | Acknowledges idle transition |
| Merger merges | Squash-merge to main, closes MR and source issue |
| Identity survives | Agent bead still exists; CV chain has new entry; worker ready for reuse |

```
STEP CLEANUP (intermediate)          Workflow CLEANUP (final)
┌────────────────────┐               ┌────────────────────────────┐
│ Step bead: closed  │               │ All step beads: closed     │
│ Session: terminated│               │ Session: terminated        │
│ Sandbox: ALIVE     │               │ Sandbox: PRESERVED (idle)  │
│ Workflow: ACTIVE   │               │ Workflow: SQUASHED         │
│ Hook: SET          │               │ Hook: CLEARED              │
│ Agent bead: working│               │ Agent bead: nuked          │
│ Branch: ALIVE      │               │ Branch: PUSHED (idle)      │
└────────────────────┘               └────────────────────────────┘
```

### 3.3 The Cleanup Pipeline

The cleanup pipeline is a chain of transfers, not a monolithic operation:

```
Worker calls gt done
    │
    ├── Sets cleanup_status=clean on agent bead
    ├── Pushes branch to origin
    ├── Creates MR bead (label: gt:merge-request)
    ├── Sends worker_done mail to watcher
    └── Session exits
         │
         ▼
Watcher receives worker_done
    │
    ├── Checks cleanup_status (ZFC: trust worker self-report)
    ├── If clean → sends MERGE_READY to merger
    ├── If dirty → creates cleanup ephemeral (cannot auto-nuke)
    └── Messages merger session
         │
         ▼
Merger processes MERGE_READY
    │
    ├── Claims MR (sets assignee)
    ├── Acquires merge slot (serialized push lock)
    ├── Runs quality gates
    ├── Squash-merges to main
    ├── Closes MR bead and source issue
    ├── Sends MERGED mail to watcher
    └── Releases merge slot
         │
         ▼
Watcher receives MERGED
    │
    ├── Verifies commit is on main (all remotes)
    ├── Checks cleanup_status
    ├── Acknowledges merge (worker already idle, sandbox preserved)
    └── If dirty → warns (shouldn't happen post-merge)
```

### 3.4 Failure Recovery in the Cleanup Pipeline

Each stage can fail independently. Recovery is handled by the next sweep cycle:

| Failure | Detection | Recovery |
|---------|-----------|---------|
| `gt done` fails mid-execution | Zombie state: session alive, done-intent label | Watcher `DetectZombieWorkers()` finds stuck-in-done, recovers |
| `worker_done` mail lost | Watcher sweep: finds dead session with `assignment_bead` | `DetectZombieWorkers()` with agent-dead-in-session |
| Merge conflict | Merger `doMerge()` detects | Creates conflict resolution task, blocks MR |
| `MERGED` mail lost | Merger closed the bead; watcher sweep finds closed bead with live session | `DetectZombieWorkers()` bead-closed-still-running |
| Nuke fails | Session still running after kill attempt | Next sweep detects zombie, retries nuke |

---

## 4. Per-Project Worker Channel

### 4.1 Design Decision: Mail-Based Channel

The per-project worker channel is implemented using the existing `gt mail` system.
This was chosen over beads-based queues or state files because:

1. **Consistency:** Mail is already the coordination primitive for all Gas Town agents
2. **Persistence:** Messages survive process crashes and session cycles
3. **Routing:** Mail addresses (`gastown/watcher`) already map to project-level agents
4. **Audit trail:** Mail creates beads entries (observable, discoverable)
5. **No new infrastructure:** No new Dolt tables, no file-based queues

### 4.2 Channel Addresses

Each project has implicit lifecycle channels via existing mail routing:

| Channel | Address | Purpose | Serviced By |
|---------|---------|---------|-------------|
| Worker lifecycle | `<project>/watcher` | Recycle, nuke, health requests | Watcher sweep |
| Merge queue | `<project>/merger` | MERGE_READY, conflict reports | Merger sweep |
| Project coordination | `<project>/watcher` | Spawn requests, escalations | Watcher |
| Workspace coordination | `coordinator/` | Cross-project, strategic | Coordinator |

### 4.3 Lifecycle Message Protocol

Messages in the worker lifecycle channel follow the existing watcher protocol
(`protocol.go`):

| Subject Pattern | Type | Sender | Action |
|----------------|------|--------|--------|
| `worker_done <name>` | Completion | Worker | Verify clean, forward to merger |
| `LIFECYCLE:Shutdown <name>` | External shutdown | Daemon | Auto-nuke or cleanup ephemeral |
| `LIFECYCLE:Cycle <name>` | Session restart | Daemon | Kill and restart session |
| `HELP: <topic>` | Escalation | Worker | Watcher evaluates, relays if needed |
| `MERGED <id>` | Post-merge | Merger | Nuke worker sandbox |
| `MERGE_FAILED <id>` | Merge failure | Merger | Notify worker, rework needed |
| `RECOVERED_BEAD <id>` | Orphan recovery | Watcher | Supervisor re-dispatches work |
| `AUTO_EXECUTE_VIOLATION: <name>` | Stall detected | Daemon | Watcher investigates |
| `ORPHANED_WORK: <name>` | Dead session + work | Daemon | Watcher recovers or nukes |

### 4.4 Channel Processing

The watcher processes its channel during sweep cycles. Processing is
first-come-first-served within each cycle. The sweep pattern:

```
Watcher sweep cycle:
    │
    ├── 1. Check inbox (gt mail inbox)
    │   └── Process lifecycle messages in order
    │
    ├── 2. Detect zombie workers
    │   └── For each zombie: nuke or escalate
    │
    ├── 3. Detect orphaned beads
    │   └── For each orphan: reset status, mail supervisor
    │
    ├── 4. Detect stalled workers
    │   └── For each stalled: message or escalate
    │
    ├── 5. Check for pending spawns
    │   └── Process spawn requests from daemon
    │
    └── 6. Write sweep receipt
        └── Machine-readable summary of findings
```

### 4.5 Who Services the Channel

The watcher is the primary consumer, but the design supports opportunistic
servicing by other sweep agents:

| Agent | When It Services | What It Can Do |
|-------|-----------------|---------------|
| **Watcher** | Every sweep cycle | Full lifecycle: spawn, nuke, escalate |
| **Supervisor** | During project-wide sweep | Detect unserviced requests, message watcher |
| **Daemon** | Every heartbeat tick | Detect dead sessions, send LIFECYCLE messages |
| **Merger** | During merge processing | Send MERGED/MERGE_FAILED to watcher |

This creates redundant monitoring: if the watcher misses a message, the supervisor or
daemon detects the resulting state (dead session, orphaned bead) and either
handles it directly or messages the watcher.

---

## 5. Auto-Execute Rule + Pinned Work = Completion Guarantee

### 5.1 The Completion Invariant

As long as three conditions hold, a workflow WILL eventually complete:

1. **Work is pinned** (`assignment_bead` set on agent bead)
2. **Sandbox persists** (branch + worktree exist)
3. **Someone keeps spawning sessions** (watcher respawn on crash)

Auto-Execute Rule ensures that when a session starts with a hook, it executes. The hook
persists across session cycles. The sandbox provides continuity. The watcher
provides resurrection. Together, these guarantee eventual completion.

### 5.2 The Completion Loop

```
┌─────────────────────────────────────────────┐
│              COMPLETION LOOP                 │
│                                              │
│   Session spawns → gt prime → discovers hook │
│        │                                     │
│        ▼                                     │
│   Auto-Execute Rule fires → execute current step          │
│        │                                     │
│        ▼                                     │
│   Step complete → bd close → transfer         │
│        │                                     │
│        ▼                                     │
│   More steps? ──yes──▶ Respawn session ──┐   │
│        │                                 │   │
│        no                                │   │
│        │                                 │   │
│        ▼                                 │   │
│   gt done → merge → nuke                 │   │
│                                          │   │
│   Session crashes? ──▶ Watcher respawns ─┘   │
│                                              │
└─────────────────────────────────────────────┘
```

### 5.3 What Breaks the Guarantee

| Failure | Effect | Recovery |
|---------|--------|---------|
| Watcher down | No respawn on crash | Supervisor detects, restarts watcher |
| Sandbox corrupted | Branch or worktree broken | `RepairWorktree()` or nuke and respawn |
| Hook cleared accidentally | Auto-Execute Rule doesn't fire | Watcher `DetectOrphanedBeads()` finds in-progress bead, resets for re-dispatch |
| Dolt server down | Cannot read beads state | Daemon auto-restarts Dolt; worker retries |
| Crash loop (3+ crashes) | Same step keeps failing | Watcher escalates to coordinator; filed as bug |

### 5.4 Liveness vs Safety

The system prioritizes **liveness** (work eventually completes) over strict safety
(no duplicate work). This means:

- **Duplicate detection is best-effort.** If two sessions somehow run the same
  step, the git branch serializes writes and one will fail to push.
- **Idempotent operations are preferred.** Closing an already-closed bead is a
  no-op. Pushing an already-pushed branch is safe.
- **Crash recovery may re-execute partial work.** A step that crashed mid-way
  will be re-executed from the start. Git state helps: if commits were made,
  the new session sees them.

---

## 6. Sweep Coordination

### 6.1 The Four Sweep Agents

Gas Town has four agents that perform sweep (periodic health monitoring):

| Agent | Scope | Frequency | Key Checks |
|-------|-------|-----------|-----------|
| **Daemon** | Workspace-wide | 3-minute heartbeat | Session liveness, Auto-Execute Rule violations, orphaned work |
| **Boot/Supervisor** | Workspace-wide | Per daemon tick | Supervisor health, watcher health, cross-project issues |
| **Watcher** | Per-project | Continuous | Worker health, zombie detection, completion handling |
| **Merger** | Per-project | On demand | Merge queue processing, conflict detection |

### 6.2 Sweep Overlap as Resilience

Multiple agents observing overlapping state is intentional redundancy:

```
               Daemon                          Supervisor
           (mechanical)                    (intelligent)
                │                               │
    ┌───────────┼───────────┐       ┌──────────┼──────────┐
    │           │           │       │          │          │
 Session    Auto-Execute Rule         Orphan   Watcher   Merger    Cross-project
 liveness   violations   work    health    health      batch
    │           │           │       │          │
    └───────────┤           │       │          │
                │           │       │          │
                ▼           ▼       ▼          ▼
              Watcher               Watcher    Merger
           (per-project sweep)      (responds)   (responds)
                │
    ┌───────────┼───────────┐
    │           │           │
 Zombie      Orphaned     Stalled
 detection   beads        workers
```

**Key property:** If any single sweep agent fails, the others detect the
resulting state degradation and compensate. The daemon detects dead sessions.
The supervisor detects dead watchers. The watcher detects dead workers.

### 6.3 Information Flow Between Sweep Agents

```
Daemon ───LIFECYCLE:──────▶ Watcher inbox
Daemon ───AUTO_EXECUTE_VIOLATION:─▶ Watcher inbox
Daemon ───ORPHANED_WORK:──▶ Watcher inbox

Supervisor ◀──heartbeat.json──── Daemon
Supervisor ───message────────────▶ Watcher (if stale)
Supervisor ───message────────────▶ Merger (if stale)

Watcher ──MERGE_READY:────▶ Merger inbox
Watcher ──RECOVERED_BEAD:─▶ Supervisor (for re-dispatch)
Watcher ──sweep receipt───▶ Beads (audit trail)

Merger ─MERGED:─────────▶ Watcher inbox
Merger ─MERGE_FAILED:───▶ Watcher inbox
Merger ─batch check─────▶ Supervisor (for stranded batches)
```

### 6.4 Convergent State

All sweep agents converge on the same observable state: beads (via Dolt), git
(via branches and worktrees), and tmux (via session liveness). No agent maintains
private state that others depend on. This is the "discover, don't track" principle
applied to monitoring.

If state diverges (e.g., a message is lost), the next sweep cycle re-derives
state from observables and self-heals.

---

## 7. Resolved Design Questions

### Q1: Spoon-Feeding and Step Granularity

**Question:** How many logical steps per physical workflow step? How many steps
per worker session?

**Answer:** Use templates to define granularity, and let context pressure determine
session boundaries.

**Step granularity guidelines:**

| Step Type | Granularity | Example |
|-----------|-------------|---------|
| Setup / teardown | One physical step | "Set up working branch" |
| Implementation | One per logical unit | "Implement the solution" (may span sessions) |
| Verification | One per check type | "Run quality checks", "Self-review" |
| Transfer | One per lifecycle event | "Commit changes", "Submit work" |

The `wf-worker-work` template currently uses 10 steps. This is appropriate for
most work because:

- Each step has clear entry/exit criteria
- Steps are independently resumable (a crash mid-step loses at most one step's work)
- Context stays focused (one step's instructions, not the whole workflow)

**Session-per-step is a guideline, not a rule.** A worker may complete multiple
steps in one session if context permits. The key constraint is that each step
is closed individually (no batch-closing — the Batch-Closure Heresy).

**Anti-patterns:**
- Steps so small they're just `git add` commands (overhead exceeds value)
- Steps so large they exhaust context (implementation + testing + review in one step)
- Steps that can't be independently resumed (step 3 requires step 2's context window)

### Q2: Mechanical vs Agent-Driven Recycling

**Question:** When is mechanical intervention (daemon-driven) appropriate vs
agent-driven (worker requests its own recycle)?

**Answer:** Prefer explicit self-recycling. Use mechanical intervention only as a
safety net.

**The spectrum:**

```
AGENT-DRIVEN (preferred)              MECHANICAL (safety net)
├── gt done (worker goes idle)       ├── Daemon detects dead session
├── gt transfer (worker self-cycles)  ├── Daemon detects Auto-Execute Rule violation
├── gt escalate (worker asks help)   ├── Watcher zombie sweep
└── HELP mail (worker signals)       └── Supervisor restart on stale heartbeat
```

**Design principle:** The worker is the authority on its own state. External
intervention should only occur when the worker cannot speak for itself (dead
session, hung process, stuck-in-done).

**Concrete thresholds (agent-determined, not hardcoded):**

The daemon uses broad thresholds for safety-net detection:
- **Auto-Execute Rule violation:** 30 minutes with `assignment_bead` but no progress
- **Hung session:** 30 minutes of no tmux output (`HungSessionThresholdMinutes`)
- **Stuck-in-done:** 60 seconds with `done-intent` label

These thresholds are intentionally generous. The goal is to catch truly stuck
workers, not workers that are thinking hard. False positives (the "Supervisor
murder spree" bug) are worse than slow detection.

**The murder spree lesson:** Mechanical detection of "stuck" is fragile because
distinguishing "thinking deeply" from "hung" requires intelligence. This is why
Boot exists (intelligent triage) and why the daemon's thresholds are conservative.
Only the watcher (an AI agent) should make judgment calls about whether a worker
is truly stuck.

### Q3: Channel Implementation

**Question:** Mail-based, beads-based, or state file?

**Answer:** Mail-based. See [Section 4](#4-per-project-worker-channel) for full design.

**Why not beads-based (special issue type)?**
- Beads issues are durable work artifacts. Lifecycle requests are transient signals.
- Creating/closing beads for "recycle me" adds unnecessary Dolt write pressure.
- Mail is already the coordination primitive and has the right lifecycle (read → process → delete).

**Why not state files (project/worker-queue.json)?**
- State files require explicit locking for concurrent access.
- No audit trail (file gets overwritten).
- Doesn't integrate with existing sweep patterns (agents already check mail).
- Recovery after crash is harder (partially-written JSON).

### Q4: Who Spawns the Next Step?

**Question:** After a worker completes a step and hands off, who spawns the
next session to continue the workflow?

**Answer:** The watcher, triggered by either transfer detection or daemon lifecycle
request.

**The spawn chain:**

```
Worker completes step
    │
    ├── Closes step bead
    ├── Calls gt transfer (creates transfer mail)
    └── Session exits
         │
         ▼
Daemon heartbeat tick
    │
    ├── Detects dead worker session
    ├── Finds assignment_bead still set (work isn't done)
    └── Triggers session restart
         │
         ▼
SessionManager.Start()
    │
    ├── Creates new tmux session in existing worktree
    ├── Injects env vars (GT_worker, GT_RIG)
    ├── SessionStart hook fires: gt prime --hook
    └── New session discovers next step via bd workflow current
```

**Current implementation:** The daemon's `processLifecycleRequests()` handles
this. When a session dies but the hook is still set, the daemon either sends a
`LIFECYCLE:` message to the watcher or directly restarts the session (depending
on configuration). Worker startup is handled end-to-end by the Auto-Execute Rule/beacon
flow (SessionManager → StartupNudge → BuildStartupPrompt → SessionStart hook
→ gt prime).

**Future (AT integration):** The watcher spawns replacement teammates directly
via `Teammate({ operation: "spawn" })`. The SubagentStop hook detects teammate
death and triggers respawn. See `docs/design/watcher-at-team-lead.md` for details.

---

## 8. Edge Cases and Failure Modes

### 8.1 The Stuck-in-Done Zombie

A worker runs `gt done` but the session hangs before cleanup completes.

**Detection:** Watcher `DetectZombieWorkers()` checks for `done-intent` label
older than 60 seconds with a live session.

**Recovery:** Watcher kills the session and continues the cleanup pipeline
(verify `cleanup_status`, forward to merger if MR exists).

### 8.2 The Orphaned Sandbox

A worker directory exists but no tmux session and no `assignment_bead`.

**Detection:** `Manager.ReconcilePool()` finds directories without sessions.
`DetectStaleWorkers()` identifies sandboxes far behind main with no work.

**Recovery:** If no uncommitted work and no active MR, nuke the sandbox. If
uncommitted work exists, escalate (someone needs to decide if the work matters).

### 8.3 The Split-Brain Merge

The merger starts merging while the worker is still pushing.

**Prevention:** The `cleanup_status=clean` field on the agent bead serializes
this. The watcher only sends `MERGE_READY` after verifying the worker has
exited and the branch is clean. The merge slot provides additional serialization.

### 8.4 The Infinite Cycle

A step keeps failing and the session keeps restarting.

**Detection:** Track crash count per worker (via `ReconcilePool` or
ephemeral state). Three crashes on the same step triggers escalation.

**Recovery:** Watcher stops respawning, creates a bug bead, mails the coordinator.
The workflow stays in its current state (recoverable when the bug is fixed).

### 8.5 Concurrent Workers on Same Issue

Should not happen because the hook is exclusive (one `assignment_bead` per agent bead,
one agent bead per worker name). But if it does:

**Prevention:** Git branch naming includes a unique suffix (`@<timestamp>`).
The TOCTOU guard in `DetectZombieWorkers()` (records `detectedAt`, re-verifies
before destructive action) prevents racing between detection and action.

**Recovery:** The second session fails to push (branch diverged) and escalates.

---

## 9. Future: AT Integration Impact

The Agent Teams (AT) integration (see `docs/design/watcher-at-team-lead.md`)
changes the transport layer but preserves the lifecycle model:

| Aspect | Current (tmux) | Future (AT) |
|--------|---------------|-------------|
| Session management | tmux sessions | AT teammates |
| Spawning | `SessionManager.Start()` | `Teammate({ operation: "spawn" })` |
| Health monitoring | tmux liveness + pane output | AT lifecycle hooks (SubagentStop) |
| Messaging | `gt message` (tmux send-keys) | AT messaging |
| Cleanup | Session kill (sandbox preserved) | `Teammate({ operation: "requestShutdown" })` (sandbox preserved) |

**What stays the same:**
- Beads as the durable ledger
- Workflows as workflow templates
- `gt done` as the worker idle signal
- Two-stage cleanup (step vs workflow)
- Mail for cross-project communication
- The completion guarantee (Auto-Execute Rule + pinned work + respawn)

**What changes:**
- The watcher becomes an AT team lead (delegate mode)
- Zombie detection becomes structural (hooks vs polling)
- Worker-to-worker isolation is hook-enforced, not tmux-enforced
- Real-time coordination moves from tmux to AT (ephemeral), reducing Dolt pressure

---

## 10. Implementation Status (gt-o8g8 audit, 2026-03-07)

### Shipped

All core lifecycle operations are implemented and running in production:

| Operation | Command/Component | Key Implementation |
|-----------|------------------|-------------------|
| Spawn/assign | `gt dispatch` | `dispatch.go`, `worker_spawn.go` — finds idle worker or allocates new slot |
| Work execution | `gt prime --hook` | Session discovers hook via `bd workflow current`, Auto-Execute Rule fires |
| Session cycling | `gt transfer` | `transfer.go` — all roles, preserves sandbox and identity |
| Step completion | `bd close` + `gt transfer` | Step cleanup: session dies, sandbox lives |
| Work submission | `gt done` | `done.go` — push, MR, sandbox sync, set idle |
| Idle worker reuse | `gt dispatch` | `worker/manager.go`: `FindIdleWorker()` + `ReuseIdleWorker()` — branch-only repair |
| Zombie detection | Watcher sweep | `watcher/handlers.go`: `DetectZombieWorkers()` — restart-first, no auto-nuke |
| Stale detection | Watcher sweep | `worker/manager.go`: `DetectStaleWorkers()` — tmux-based, protects paused states |
| Orphan recovery | Watcher sweep | `watcher/handlers.go`: `DetectOrphanedBeads()` — reset and re-dispatch |
| Cleanup pipeline | Mail-based | worker_done → Watcher → MERGE_READY → Merger → MERGED |
| Merge queue | Merger | Squash-merge, close MR and issue, batch check |

### Pending

| Feature | Description | Impact |
|---------|-------------|--------|
| Merger notifies coordinator after merge | PRs #2436/#2437 closed; branch cleanup shipped, coordinator notify not yet | Unblocks dependent work dispatch |

### Deferred (design only)

| Feature | Rationale for deferral |
|---------|----------------------|
| Pool size enforcement | On-demand allocation works; fixed pool is optimization, not correctness |
| `gt worker pool init` | Workers created naturally by first `gt dispatch`; pre-allocation unnecessary |
| `ReconcilePool()` | Watcher sweep already detects state drift via zombie/stale/orphan checks |

---

## 11. Summary

See [concepts/worker-lifecycle.md](../concepts/worker-lifecycle.md) for the
complete lifecycle model (three layers, four states, persistent worker design).
This document covers the implementation details: cleanup stages, mail channels,
sweep coordination, and edge case handling.
