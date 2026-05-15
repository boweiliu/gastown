# Watcher AT Team Lead: Implementation Spec

> **Status: Future architecture — NOT YET IMPLEMENTED**
> The current system uses tmux-based session management. This document describes
> a planned architectural change to use Claude Code Agent Teams (AT) as the
> transport layer. No code for this exists yet.

> **Bead:** gt-ky4jf
> **Date:** 2026-02-08
> **Author:** furiosa (gastown worker)
> **Depends on:** AT spike report (gt-3nqoz), AT integration design (agent-teams-integration.md)
> **Status:** Phase 1 implementation spec

---

## Overview

This document specifies how the Watcher becomes an AT team lead, replacing the
current tmux-based worker session management with Claude Code Agent Teams.

The Watcher enters delegate mode (structurally enforced coordination-only), spawns
worker teammates for assigned work, monitors them via AT's native lifecycle hooks,
and syncs completions to beads at task boundaries.

**What changes:** Session management layer (tmux → AT).
**What stays:** Beads as ledger, gt mail for cross-project, workflows/templates, `gt done`.

---

## AT Spike Findings Summary

> Condensed from the AT spike report (gt-3nqoz, 2026-02-08, author: nux).

**Recommendation: CONDITIONAL GO for Phase 1 experiment.**

### Go/No-Go Decision Matrix

| Criterion | Status | Notes |
|-----------|--------|-------|
| Teammate working directories | WORKAROUND | PreToolUse hook for enforcement |
| Hooks fire for teammates | GO | All relevant hooks confirmed |
| Custom agent definitions | GO | `.claude/agents/*.md` works |
| Delegate mode enforcement | GO | Structural, not behavioral |
| Teammate cycling | WORKAROUND | Transfer + respawn pattern |
| Token cost acceptable | CONDITIONAL | Sonnet teammates reduce cost |
| gt/bd command access | GO | PATH via SessionStart hook |
| Task list with dependencies | GO | Native match to Gas Town workflow |

5/8 clear GO. 2 require workarounds (viable mitigations). 1 conditional on Phase 1 cost validation.

### Critical Blockers

1. **No per-teammate working directory** — AT teammates inherit lead's cwd. Workaround: `cd` in spawn prompt + PreToolUse hook (`gt validate-worktree-scope`) for structural enforcement.
2. **No session resumption for teammates** — Crashed teammates cannot resume. Workaround: PreCompact transfer + beads state recovery + Watcher respawn.
3. **Token cost ~7x per teammate** — Mitigated by using Sonnet for worker teammates, Opus for Watcher lead only.

### Risk Register Summary

| Risk Level | Key Risks |
|------------|-----------|
| **High** | No per-teammate cwd, no session resumption, experimental feature |
| **Medium** | 7x token cost, hook compatibility gaps, AT API changes |
| **Low** | PATH/env setup, task list mapping, delegate mode gaps |

### Key Advantage

AT's file-locked task claiming eliminates Dolt write contention (estimated 80-90% reduction). This is the strongest argument for adoption.

---

## 1. Watcher in Delegate Mode

### What Tools the Watcher Keeps

In delegate mode, the Watcher has access to:

| Tool | Purpose |
|------|---------|
| `Teammate` | Spawn/shutdown teammates, send messages, manage team |
| `TaskCreate` | Create AT tasks for worker work |
| `TaskUpdate` | Update task status, set dependencies |
| `TaskList` | Monitor team progress |
| `TaskGet` | Read task details |
| `Bash` | **Not available** in delegate mode |
| `Read/Write/Edit` | **Not available** in delegate mode |
| `Glob/Grep` | **Not available** in delegate mode |

### The ZFC Upgrade

Current state: "Watcher doesn't implement" is enforced by CLAUDE.md instructions.
Agents can and do violate this under pressure.

New state: Delegate mode structurally removes implementation tools. The Watcher
literally *cannot* edit files. This is the strongest possible ZFC compliance —
the constraint is in the machinery, not in the instructions.

### Watcher Needs Bash for gt/bd Commands

**Problem:** Delegate mode removes Bash access, but the Watcher needs to run
`gt mail`, `bd show`, `bd close`, and other coordination commands.

**Solution options (in order of preference):**

1. **Custom agent definition with selective tools.** Create
   `.claude/agents/watcher-lead.md` that uses `permissionMode: delegate` but
   adds back Bash via the `tools` allowlist. This gives structural enforcement
   for file editing while preserving command access:

   ```yaml
   ---
   name: watcher-lead
   permissionMode: delegate
   tools: Teammate, TaskCreate, TaskUpdate, TaskList, TaskGet, Bash
   ---
   ```

   **Risk:** Bash access means the Watcher *could* edit files via sed/echo.
   Mitigated by: PreToolUse hook on Bash that rejects file-modifying commands.

2. **Hooks as command proxy.** The Watcher doesn't run commands directly.
   Instead, hooks fire at turn boundaries and execute gt/bd commands based on
   AT task state. The Watcher coordinates purely through AT tools; the hooks
   handle the beads bridge.

   **Risk:** Less flexible — Watcher can't make ad-hoc bd queries. But it's
   the purest delegate mode implementation.

3. **Teammate as command runner.** Spawn a lightweight "ops" teammate whose
   sole job is running gt/bd commands on the Watcher's behalf. The Watcher
   sends commands via AT messaging; the ops teammate executes and returns results.

   **Risk:** Token overhead for a simple command proxy. But it preserves
   strict delegate mode for the Watcher.

**Recommendation:** Option 1 (custom agent with selective tools). It's pragmatic,
preserves the Watcher's ability to query beads state, and the PreToolUse hook
provides sufficient guardrails. Pure delegate mode is aspirational but the
Watcher genuinely needs to read beads state for coordination decisions.

### PreToolUse Guard for Watcher Bash

```json
{
  "PreToolUse": [{
    "matcher": "Bash",
    "hooks": [{
      "type": "command",
      "command": "gt watcher-bash-guard"
    }]
  }]
}
```

The `gt watcher-bash-guard` script:
- Allows: `gt *`, `bd *`, `git status`, `git log`, read-only commands
- Blocks: `echo >`, `cat >`, `sed -i`, `vim`, `nano`, any write operation
- Returns exit code 2 with reason on block

---

## 2. Teammate Spawn: Work Assignment → AT Task Creation

### The Spawn Flow

When work arrives (via batch, gt dispatch, or direct assignment):

```
1. Watcher receives work (mail, batch dispatch, bd ready)
2. Watcher creates AT team (if not already active)
3. For each issue to dispatch:
   a. Create AT task with issue details and dependencies
   b. Spawn worker teammate assigned to that task
4. Teammates self-claim tasks and begin execution
```

### Team Creation

```
Teammate({
  operation: "spawnTeam",
  team_name: "<project-name>-work",
  description: "Worker work team for <batch/sprint description>"
})
```

Team naming convention: `<project>-work` for the primary work team.
One team per project per active batch. Multiple batches = multiple teams
(AT limitation: one team per session, so Watcher manages one batch
at a time).

### AT Task Creation from Beads Issues

For each issue dispatched to a worker:

```
TaskCreate({
  subject: "<issue title>",
  description: "Issue: <issue-id>\n<issue description>\n\nWorktree: /path/to/<worker>/\nFormula: wf-worker-work",
  activeForm: "Working on <issue title>",
  metadata: {
    "bead_id": "<issue-id>",
    "worktree": "/path/to/worktree",
    "workflow": "<wf-id>"
  }
})
```

**Key fields in metadata:**
- `bead_id`: Links AT task back to the beads issue for sync
- `worktree`: The git worktree path this worker should use
- `workflow`: The wf-worker-work instance for this issue

### Dependency Mapping

Beads issue dependencies map to AT task dependencies:

```
# If issue B depends on issue A:
# After creating both tasks:
TaskUpdate({
  taskId: "<task-B-id>",
  addBlockedBy: ["<task-A-id>"]
})
```

This enables AT's native self-claim: when task A completes, task B becomes
unblocked and the next idle teammate picks it up automatically.

### Worker Teammate Spawn

```
Task({
  subagent_type: "worker",
  team_name: "<project>-work",
  name: "<worker-name>",
  model: "sonnet",
  prompt: "You are worker <name>. Your worktree is <path>.\n\nAssigned issue: <id> - <title>\n<description>\n\nWorkflow:\n1. cd <worktree>\n2. Run `gt prime` for full context\n3. Follow wf-worker-work steps\n4. When done: commit, push, run `gt done`"
})
```

**Model selection:**
- Worker teammates: `model: "sonnet"` (execution-focused, cost-efficient)
- Watcher lead: Opus (judgment, coordination, quality review)
- Merger teammate (Phase 2): `model: "sonnet"` (mechanical merge work)

### The `.claude/agents/worker.md` Definition

```yaml
---
name: worker
description: Gas Town worker worker agent (persistent identity, ephemeral sessions)
model: sonnet
hooks:
  SessionStart:
    - hooks:
        - type: command
          command: "export PATH=\"$HOME/go/bin:$HOME/.local/bin:$PATH\" && gt prime --hook"
  PreToolUse:
    - matcher: "Write|Edit"
      hooks:
        - type: command
          command: "gt validate-worktree-scope"
  PreCompact:
    - matcher: "auto"
      hooks:
        - type: command
          command: "gt transfer --reason compaction"
  Stop:
    - hooks:
        - type: command
          command: "gt signal stop"
---

You are a Gas Town worker (persistent identity, ephemeral sessions).

## Startup
1. `cd` to your assigned worktree (given in your spawn prompt)
2. Run `gt prime` for full context
3. Check your hook: `gt assignment`
4. Follow workflow steps: `bd workflow current`

## Work Protocol
- Mark steps in_progress before starting: `bd update <id> --status=in_progress`
- Close steps when done: `bd close <id>`
- Commit frequently with descriptive messages
- Never batch-close steps

## Completion
When all steps done:
1. `git status` — must be clean
2. `git push`
3. `gt done` — submits to merge queue, nukes your sandbox
```

### Worktree Assignment

Each worker teammate operates in its own git worktree. Since AT doesn't support
per-teammate working directories natively, enforcement is via:

1. **Spawn prompt:** First instruction is `cd /path/to/worktree`
2. **PreToolUse hook:** `gt validate-worktree-scope` rejects Write/Edit operations
   targeting paths outside the assigned worktree
3. **Environment variable:** `GT_WORKTREE=/path/to/worktree` set via SessionStart hook

The Watcher creates worktrees before spawning teammates:
```bash
git worktree add /path/to/workers/<name>/<project> -b worker/<name>/<issue-id>
```

This matches the current worktree management — the change is WHO creates them
(Watcher via AT, not `gt dispatch` via Go daemon).

---

## 3. Bead Sync Protocol

### The Two-Layer Model

```
Layer 1 (AT, ephemeral):     Task claiming, status, messaging
Layer 2 (Beads/Dolt, durable): Issue creation, completion, audit trail
```

### Sync Points

| AT Event | Beads Action | Trigger |
|----------|-------------|---------|
| Task claimed (in_progress) | `bd update <id> --status=in_progress` | TaskCompleted hook / worker prompt |
| Task completed | `bd close <step-id>` | TaskCompleted hook |
| New issue discovered | AT task created by Watcher | Watcher reads worker message |
| Teammate idle | Check beads for more work | TeammateIdle hook |
| Team shutdown | Verify all beads synced | Watcher cleanup routine |

### TaskCompleted Hook for Bead Sync

The `TaskCompleted` hook fires when an AT task is marked complete. This is the
primary sync mechanism:

```bash
#!/bin/bash
# .claude/hooks/task-completed-sync.sh
# Fires on TaskCompleted hook

BEAD_ID=$(echo "$TASK_METADATA" | jq -r '.bead_id // empty')
if [ -n "$BEAD_ID" ]; then
  export PATH="$HOME/go/bin:$HOME/.local/bin:$PATH"
  bd close "$BEAD_ID" 2>/dev/null
fi
exit 0
```

Hook configuration:
```json
{
  "TaskCompleted": [{
    "hooks": [{
      "type": "command",
      "command": ".claude/hooks/task-completed-sync.sh"
    }]
  }]
}
```

**Important:** The hook should NOT block task completion (exit 0 always). If the
`bd close` fails (Dolt contention), it will be retried at the next sync point.
The AT task list is the real-time truth; beads catches up at boundaries.

### Worker-Side Bead Updates

Workers still run `bd update` and `bd close` directly as part of their workflow
workflow. The TaskCompleted hook is a safety net, not the primary mechanism. This
means:

- Worker marks workflow step in_progress → `bd update --status=in_progress`
- Worker completes workflow step → `bd close <step-id>`
- AT task completion → TaskCompleted hook also fires `bd close` (idempotent)

Double-close is safe: `bd close` on an already-closed bead is a no-op.

### Sync Verification at Team Shutdown

Before the Watcher shuts down the team, it verifies beads are in sync:

```
For each AT task marked completed:
  1. Read task metadata for bead_id
  2. Verify bead is closed (bd show <id> | check status)
  3. If bead still open: bd close <id> with notes
  4. If close fails: log warning, continue (Dolt retry will handle)
```

This is the "boundary sync" pattern from the integration design: AT handles
real-time coordination, beads catches up at lifecycle boundaries (team shutdown,
batch completion).

---

## 4. Session Cycling: Compaction → Respawn → Resume

### The Problem

AT teammates cannot be resumed after shutdown. When a teammate hits context
limits and compacts, or crashes, a new teammate must be spawned.

### The Lifecycle

```
Teammate running
    │
    ├── Context filling → PreCompact hook fires
    │   │
    │   └── gt transfer --reason compaction
    │       ├── Saves current workflow step to beads
    │       ├── Saves progress notes
    │       └── Saves git branch state
    │
    ├── Auto-compaction occurs
    │   │
    │   └── SessionStart hook fires (source: "compact")
    │       └── gt prime --compact-resume
    │           └── Reads beads state, restores context
    │
    └── Teammate continues with compressed context
```

### When Compaction Isn't Enough (Teammate Death)

If a teammate crashes or is shut down (not just compacted):

```
Teammate stops
    │
    └── SubagentStop hook fires on Watcher (lead)
        │
        ├── Read teammate's last known state from beads
        │   └── Which workflow step was in_progress?
        │   └── What branch was being worked on?
        │
        ├── Assess: recoverable or escalate?
        │   ├── Normal completion: AT task done, beads synced → no action
        │   ├── Incomplete work: respawn with resume context
        │   └── Repeated crashes: escalate to Watcher mail → Coordinator
        │
        └── If recoverable: spawn replacement teammate
            └── Task({ subagent_type: "worker", ... resume prompt ... })
```

### SubagentStop Hook (Watcher Side)

```json
{
  "SubagentStop": [{
    "matcher": "worker",
    "hooks": [{
      "type": "command",
      "command": "gt watcher-teammate-stopped"
    }]
  }]
}
```

The `gt watcher-teammate-stopped` script:
1. Reads the stopped agent's transcript path (available in hook input)
2. Checks AT task status — was the task completed?
3. Checks beads — was `gt done` run?
4. If completed: no action (normal lifecycle)
5. If incomplete: outputs `{ "decision": "block", "reason": "Teammate <name> stopped before completing task <id>. Beads state: <status>. Respawn needed." }`

The "block" decision prevents the Watcher from going idle, injecting the
respawn instruction as context for the Watcher to act on.

### Respawn Prompt Template

```
Teammate <name> stopped before completing work.

Last known state:
- Issue: <bead-id> (<title>)
- Workflow step: <step-id> (in_progress)
- Branch: <branch-name>
- Worktree: <path>

Spawn a replacement worker with this context. The new teammate
should read beads state and continue from the last checkpoint.
```

### Crash Loop Prevention

Track respawn attempts per issue. If a teammate crashes 3 times on the
same issue:

1. Mark the AT task as blocked
2. File a bead: `bd create --title "Worker crash loop on <issue>" --type bug`
3. Mail the Watcher/Coordinator for escalation
4. Do NOT respawn — the issue has a structural problem

Tracking: Use AT task metadata `{ "respawn_count": N }` incremented on
each respawn. This is ephemeral (dies with the team) which is correct —
crash tracking only matters during the current team session.

---

## 5. Error Handling

### Error Categories and Responses

| Error | Detection | Response |
|-------|-----------|----------|
| Teammate crash | SubagentStop hook | Respawn or escalate (see above) |
| Teammate stuck (no progress) | TeammateIdle hook | Send message asking for status |
| Test failures | TaskCompleted hook (exit 2) | Block completion, teammate must fix |
| Merge conflict | Worker messages Watcher | Watcher advises or reassigns |
| Dolt write failure | bd command exit code | Retry with backoff (existing mechanism) |
| AT team crash | Watcher session dies | Daemon/Boot/Supervisor chain detects, restarts Watcher |
| Worktree scope violation | PreToolUse hook | Block the operation, warn worker |

### TeammateIdle Hook

```bash
#!/bin/bash
# gt watcher-teammate-idle
# Fires when a teammate is about to go idle

export PATH="$HOME/go/bin:$HOME/.local/bin:$PATH"

# Check if there's more work in beads
READY=$(bd ready --count 2>/dev/null)
if [ "$READY" -gt 0 ]; then
  echo "There is more work available. Run 'bd ready' to see unblocked tasks." >&2
  exit 2  # Block idle, send feedback
fi

# Check if gt done was run
if git log --oneline -1 | grep -q "gt done"; then
  exit 0  # Normal completion
fi

# Teammate seems genuinely idle without completing
echo "Your work doesn't appear complete. Run 'bd ready' to check remaining steps, or 'gt done' if finished." >&2
exit 2
```

### TaskCompleted Quality Gate

```bash
#!/bin/bash
# Fires on TaskCompleted hook
# Validates that work meets minimum quality before marking complete

export PATH="$HOME/go/bin:$HOME/.local/bin:$PATH"

# Check for uncommitted changes
if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
  echo "Uncommitted changes detected. Commit your work before marking complete." >&2
  exit 2
fi

# Check that the branch has been pushed
BRANCH=$(git branch --show-current 2>/dev/null)
if ! git log "origin/$BRANCH" --oneline -1 >/dev/null 2>&1; then
  echo "Branch not pushed to remote. Run 'git push' before completing." >&2
  exit 2
fi

exit 0
```

---

## 6. Batch Mapping to AT Teams

### The Natural Mapping

| Gas Town | AT Equivalent |
|----------|--------------|
| Batch | AT team lifecycle |
| Batch issues | AT tasks |
| War Project (per-project batch execution) | AT team instance |
| Ready front (unblocked issues) | Unblocked AT tasks |
| Dispatch | AT task creation + teammate spawn |
| Completion tracking | AT task list status |

### One Batch = One AT Team Session

A batch arrives at a project. The Watcher creates an AT team for that batch:

```
Batch hq-abc arrives at gastown
    │
    ├── Watcher creates team: "gastown-batch-abc"
    │
    ├── For each issue in batch:
    │   ├── Create AT task (with bead_id in metadata)
    │   └── Set dependencies (from beads dep graph)
    │
    ├── Spawn N worker teammates (N = min(issues, max_workers))
    │
    ├── Teammates self-claim tasks from ready front
    │
    ├── As tasks complete:
    │   ├── Dependencies unblock next tasks
    │   ├── Idle teammates auto-claim newly ready tasks
    │   └── Beads synced via TaskCompleted hook
    │
    └── All tasks done:
        ├── Watcher verifies beads sync
        ├── Watcher sends batch completion to Coordinator (gt mail)
        └── Team shutdown
```

### Multiple Batches

AT limitation: one team per session. If a second batch arrives while the
first is active:

**Option A: Sequential processing.** Finish batch 1, then start batch 2.
Simple, no concurrency issues. Acceptable if batch throughput is sufficient.

**Option B: Batch queue.** The Watcher queues incoming batches and processes
them in order. The queue lives in beads (mail inbox) — the Watcher checks for
new batches when the current team finishes.

**Option C: Multiple Watcher sessions.** The daemon spawns a second Watcher
session for the second batch. Each Watcher manages its own AT team. This
requires the daemon to support multiple Watcher instances per project.

**Recommendation:** Option A for Phase 1 (sequential). Option C for Phase 2+
if throughput demands it. The batch queue in Option B is implicit in beads
already (unprocessed batch mail = queued work).

### Steady-State Worker Pool

For large batches (20+ issues), the Watcher doesn't spawn 20 teammates at once.
Instead:

```
max_teammates = 5  # configurable per project

1. Spawn max_teammates workers
2. Create all AT tasks (with dependencies)
3. Teammates self-claim from ready front
4. As teammates complete tasks:
   - Auto-claim next unblocked task
   - No respawn needed (same teammate, new task)
5. When all tasks done: team shutdown
```

AT's self-claim mechanism is the key enabler. Teammates don't die after one
task — they pick up the next one. This eliminates the current spawn/nuke
overhead per issue.

**When a teammate needs to cycle** (compaction), the Watcher spawns a
replacement, not an additional teammate. The pool size stays at max_teammates.

---

## 7. Mail Bridge: gt mail ↔ AT Messages

### The Boundary

```
                    ┌─────────────────┐
                    │    Watcher       │
                    │  (AT Team Lead)  │
                    │                  │
    gt mail ←──────│── Bridge ──────→ AT messaging
    (cross-project,    │                  (intra-team,
     persistent)   │                   ephemeral)
                    └─────────────────┘
```

### Inbound: gt mail → AT message

When the Watcher receives gt mail relevant to an active teammate:

```
gt mail inbox
    │
    ├── From Coordinator: "Priority shift — issue X is now P0"
    │   └── Watcher sends AT message to relevant teammate:
    │       Teammate({ operation: "write", target_agent_id: "<worker>",
    │                  value: "Priority update: <issue> is now P0. Expedite." })
    │
    ├── From Merger: "Merge conflict on <branch>"
    │   └── Watcher sends AT message to the worker on that branch:
    │       Teammate({ operation: "write", target_agent_id: "<worker>",
    │                  value: "Merge conflict detected. Rebase on main." })
    │
    └── From another project's Watcher: "Dependency <issue> is done"
        └── Watcher creates/unblocks AT task for downstream work
```

### Outbound: AT event → gt mail

When AT events need to reach entities outside the team:

```
Teammate completes final task
    │
    └── Watcher detects all tasks done
        │
        ├── gt mail send gastown/merger -s "MERGE_READY: <branch>"
        │   └── Merger processes merge queue
        │
        ├── gt mail send coordinator/ -s "BATCH COMPLETE: hq-abc"
        │   └── Coordinator updates batch tracking
        │
        └── gt mail send gastown/watcher -s "worker_done: <name>"
            └── (Self-mail for beads record)
```

### What Goes Where

| Communication | Channel | Why |
|--------------|---------|-----|
| Watcher ↔ Worker | AT messaging | Same team, real-time, ephemeral |
| Worker ↔ Worker | AT messaging | Same team, coordination chatter |
| Watcher → Merger | gt mail | Different lifecycle, needs persistence |
| Watcher → Coordinator | gt mail | Cross-project, needs persistence |
| Coordinator → Watcher | gt mail | Cross-project, needs persistence |
| Worker escalation | AT message to Watcher, Watcher relays via gt mail | Bridge pattern |

### The Relay Pattern

Workers can't send gt mail directly to entities outside their team (AT
messaging is team-scoped). Instead:

```
Worker needs to escalate to Coordinator:
    │
    ├── Worker sends AT message to Watcher:
    │   "ESCALATE: Need Coordinator decision on auth approach"
    │
    └── Watcher relays via gt mail:
        gt mail send coordinator/ -s "ESCALATE from worker <name>" -m "..."
```

This is analogous to the current model where workers mail the Watcher and
the Watcher escalates. The difference: AT messaging is real-time (no Dolt
sync lag), and the Watcher can relay immediately.

---

## 8. Configuration

### `.claude/settings.json` (Project Level)

```json
{
  "env": {
    "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS": "1"
  },
  "hooks": {
    "TaskCompleted": [{
      "hooks": [{
        "type": "command",
        "command": ".claude/hooks/task-completed-sync.sh"
      }]
    }],
    "TeammateIdle": [{
      "hooks": [{
        "type": "command",
        "command": ".claude/hooks/teammate-idle-check.sh"
      }]
    }],
    "SubagentStop": [{
      "matcher": "worker",
      "hooks": [{
        "type": "command",
        "command": ".claude/hooks/teammate-stopped.sh"
      }]
    }]
  }
}
```

### `.claude/agents/watcher-lead.md`

```yaml
---
name: watcher-lead
description: Gas Town Watcher operating as AT team lead
model: opus
permissionMode: delegate
hooks:
  SessionStart:
    - hooks:
        - type: command
          command: "export PATH=\"$HOME/go/bin:$HOME/.local/bin:$PATH\" && gt prime --hook"
  PreToolUse:
    - matcher: "Bash"
      hooks:
        - type: command
          command: "gt watcher-bash-guard"
  Stop:
    - hooks:
        - type: command
          command: "gt signal stop"
---

You are the Gas Town Watcher for this project.

## Role
You coordinate worker workers. You NEVER implement code directly.
Delegate mode enforces this structurally — you cannot edit files.

## Startup
1. Check for incoming work: `gt mail inbox`, `bd ready`
2. Create AT team if work is available
3. Spawn worker teammates for each issue
4. Monitor progress via AT task list

## During Work
- Monitor teammate progress via TaskList
- Relay cross-project messages (gt mail ↔ AT messages)
- Handle teammate crashes (respawn or escalate)
- Enforce quality via plan approval

## Completion
- Verify all AT tasks completed
- Verify beads are synced (all issues closed)
- Send MERGE_READY to Merger via gt mail
- Send batch completion to Coordinator via gt mail
- Shutdown team
```

### `.claude/agents/worker.md`

See Section 2 above for the full definition.

---

## 9. What Gets Replaced

### Infrastructure Removed (Phase 1)

| Component | Replacement | Notes |
|-----------|-------------|-------|
| `gt dispatch` (worker spawn) | `Teammate({ operation: "spawn" })` | AT native |
| `gt worker nuke` | `Teammate({ operation: "requestShutdown" })` | AT native |
| tmux session management | AT manages teammate sessions | No more tmux for workers |
| `gt message` (tmux send-keys) | `Teammate({ operation: "write" })` | AT messaging |
| Zombie detection (tmux-based) | SubagentStop / TeammateIdle hooks | Structural |
| Watcher "are you stuck?" polling | TeammateIdle hook (automatic) | Event-driven |
| Worker-to-worker isolation | Prompt + PreToolUse hook | Behavioral → hook-enforced |

### Infrastructure Kept (Phase 1)

| Component | Why |
|-----------|-----|
| Beads (Dolt) | Durable ledger — AT tasks are ephemeral |
| gt mail | Cross-project communication — AT is team-scoped |
| Workflows/templates | Work templates — AT tasks created from these |
| `gt done` | Worker self-clean — unchanged lifecycle |
| Git worktrees | Filesystem isolation — AT doesn't provide this |
| Daemon/Boot/Supervisor | Health monitoring — AT has no crash recovery |
| Merger (separate) | Different lifecycle (Phase 2 brings it in-band) |
| Batch tracking | Cross-project work orders — above AT scope |

### Dolt Write Pressure Reduction

**Current:** Every `bd update`, `bd close`, `bd create` from every worker
= concurrent Dolt writes. 20 workers = 20+ concurrent commits.

**With AT:** Real-time task coordination happens in AT (file-locked, no Dolt).
Dolt writes only at boundaries:
- `bd close` when a workflow step completes (1 per task)
- `bd create` when workers discover new issues (rare)

**Estimated reduction: 80-90%.** The remaining writes are naturally staggered
across minutes (task completions), not milliseconds (concurrent status updates).

---

## 10. Watcher Startup Flow (Updated)

```
Watcher session starts (managed by daemon)
    │
    ├── SessionStart hook: gt prime --hook
    │   └── Loads role context, checks hook
    │
    ├── Check for work:
    │   ├── gt mail inbox (batch dispatch, priority changes)
    │   ├── bd ready (unblocked issues)
    │   └── gt assignment (assigned work)
    │
    ├── If work available:
    │   │
    │   ├── Create AT team:
    │   │   Teammate({ operation: "spawnTeam", team_name: "<project>-work" })
    │   │
    │   ├── Create AT tasks from beads issues:
    │   │   For each issue: TaskCreate({ subject, description, metadata: { bead_id } })
    │   │   Set dependencies: TaskUpdate({ addBlockedBy: [...] })
    │   │
    │   ├── Create worktrees for workers:
    │   │   For each worker: git worktree add ...
    │   │
    │   ├── Spawn worker teammates:
    │   │   For each (up to max_teammates):
    │   │     Task({ subagent_type: "worker", team_name: "...", name: "..." })
    │   │
    │   └── Enter monitoring loop:
    │       ├── Watch AT task list for completions
    │       ├── Handle teammate crashes (SubagentStop)
    │       ├── Relay gt mail ↔ AT messages
    │       ├── Check for new batch arrivals
    │       └── When all tasks done: cleanup and report
    │
    └── If no work:
        └── Stop hook checks for queued work periodically
            └── If work arrives: wake and create team
```

---

## 11. Phase 1 Scope and Validation Criteria

### In Scope

1. Watcher as AT team lead in delegate mode (with Bash for gt/bd)
2. Worker teammates with `.claude/agents/worker.md`
3. Bead sync via TaskCompleted hook
4. Session cycling via PreCompact transfer + respawn
5. Basic error handling (crash detection, respawn, crash loop prevention)
6. Mail bridge (gt mail ↔ AT messaging)
7. Single-batch sequential processing

### Out of Scope (Phase 2+)

1. Merger as AT teammate
2. Multiple concurrent batches
3. Cross-project AT coordination
4. Team squads / shadow workers
5. Advanced plan approval workflows
6. Performance optimization (token cost tuning)

### Validation Criteria

| Criterion | Test |
|-----------|------|
| Watcher stays in delegate mode | Verify Watcher cannot write/edit files |
| Workers complete work | End-to-end: spawn → implement → push → gt done |
| Beads sync correctly | AT task completion → bd close fires → bead is closed |
| Session cycling works | Force compaction → new teammate resumes from beads |
| Crash recovery works | Kill a teammate → Watcher detects → respawns |
| Mail bridge works | Coordinator sends mail → Watcher relays to worker |
| Dolt writes reduced | Measure bd command frequency: before vs after |
| Token cost acceptable | `/cost` shows < 3x overhead vs current model |
| Batch completes | Full batch lifecycle: dispatch → work → merge → done |

---

## 12. Migration Path

### Current Architecture → Phase 1

The transition is additive: AT runs alongside existing infrastructure during
validation. The Watcher can fall back to tmux-based management if AT fails.

```
Step 1: Enable AT feature flag in gastown .claude/settings.json
Step 2: Create .claude/agents/worker.md and .claude/agents/watcher-lead.md
Step 3: Implement hook scripts (task-completed-sync, teammate-idle, teammate-stopped)
Step 4: Implement gt watcher-bash-guard
Step 5: Implement gt validate-worktree-scope
Step 6: Implement gt watcher-teammate-stopped
Step 7: Update Watcher startup to create AT team instead of tmux worker sessions
Step 8: Test with 2 workers on a small batch
Step 9: Validate all criteria above
Step 10: If validated: expand to 3-5 workers, larger batches
```

### Rollback Plan

If Phase 1 fails:
1. Disable AT feature flag
2. Watcher reverts to tmux-based worker management
3. No beads data lost (beads sync is additive)
4. File lessons-learned bead for Phase 1 retry

---

*"The transport changes. The ledger endures."*
