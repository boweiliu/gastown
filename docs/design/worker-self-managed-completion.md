# Worker Self-Managed Completion

> **Bead:** gt-0wkk
> **Date:** 2026-02-28
> **Author:** rictus (gastown worker)
> **Status:** Design proposal
> **Related:** gt-4ac (persistent worker model), gt-a6gp (message-over-mail),
> gt-6a9d (nuke safety), gt-w0br (bead-based discovery)

---

## 1. Problem Statement

Workers currently depend on the watcher to complete their lifecycle. When a
worker runs `gt done`, it performs most of the work (push branch, create MR
bead, write completion metadata, message watcher) but then **stops and waits for
the watcher** to:

1. Discover the completion (via sweep scan of agent beads)
2. Transition the worker from `agent_state=done` to `agent_state=idle`
3. Create a cleanup ephemeral to track the pending MR
4. Send `MERGE_READY` to the merger

The watcher is single-threaded (one sweep cycle at a time), so at high
throughput it becomes a bottleneck. Zombie workers accumulate in `done` state
waiting for watcher processing. This is a regression from the original model
where workers were fully self-contained.

### The Bottleneck in Numbers

With N workers completing simultaneously:
- Each watcher sweep cycle takes 30-90 seconds
- `survey-workers` step scans all agent beads sequentially
- Only one completion is processed per cycle (create ephemeral, message merger)
- N completions queue up, taking N * sweep-cycle-time to process

### How We Got Here

The watcher dependency crept in through two well-intentioned changes:

1. **Persistent worker model (gt-4ac):** Preserved sandboxes for reuse,
   requiring someone to manage the idle→reuse lifecycle. The watcher became
   that someone because it was already monitoring workers.

2. **Message-over-mail (gt-a6gp):** Moved completion discovery from
   worker-sent mail to watcher scanning agent beads. This reduced Dolt
   pressure (messages are free vs mail creating beads) but centralized
   discovery in the watcher sweep loop.

Neither change was wrong. But together they created a serial bottleneck where
the watcher became a mandatory checkpoint in every completion.

---

## 2. Current Flow (What Happens Today)

```
Worker runs gt done
    │
    ├── 1. Validate clean state (no uncommitted changes)
    ├── 2. Push branch to origin
    ├── 3. Create MR bead (type: merge-request, label: gt:merge-request)
    ├── 4. Write completion metadata to agent bead:
    │      exit_type, mr_id, branch, mr_failed, completion_time
    ├── 5. Set agent_state = "done" (NOT idle)
    ├── 6. Clear assignment_bead
    ├── 7. Message watcher via tmux
    ├── 8. Sync worktree to main, delete old branch
    └── 9. Session goes idle (sandbox preserved)
         │
         ▼
    ┌─── WAIT ──────────────────────────────────────────┐
    │ Worker is in "done" state.                       │
    │ Cannot accept new work until watcher processes.    │
    │ If watcher is busy: worker sits idle for minutes. │
    └───────────────────────────────────────────────────┘
         │
         ▼ (next watcher sweep cycle)
Watcher survey-workers step
    │
    ├── Scans all worker agent beads
    ├── Finds exit_type + completion_time set
    ├── If pending MR:
    │   ├── Create cleanup ephemeral (merge-requested state)
    │   ├── Send MERGE_READY to merger
    │   └── Clear completion metadata
    ├── Transition agent_state: done → idle
    └── Worker is now available for new work
```

**Time in "done" state:** 30s to several minutes, depending on watcher sweep
cycle timing and how many other workers completed simultaneously.

---

## 3. Proposed Flow (Self-Managed Completion)

```
Worker runs gt done
    │
    ├── 1. Validate clean state (no uncommitted changes)
    ├── 2. Push branch to origin
    ├── 3. Create MR bead (type: merge-request, label: gt:merge-request)
    ├── 4. Write completion metadata to agent bead (for audit)
    ├── 5. Message merger directly: "MERGE_READY <mr-id>"     ← NEW
    ├── 6. Set agent_state = "idle"                           ← CHANGED
    ├── 7. Clear assignment_bead
    ├── 8. Sync worktree to main, delete old branch
    └── 9. Session goes idle (sandbox preserved)
              │
              └── Worker is IMMEDIATELY available for new work
```

**Key changes:**
1. Worker sets `agent_state=idle` directly (not `done`)
2. Worker messages merger directly (not via watcher relay)
3. No cleanup ephemeral needed (see Section 5)
4. Watcher is NOT in the critical path

### What the Watcher Still Does

The watcher role **returns to being an observer** — it sweeps for anomalies
and intervenes only when something is wrong:

| Watcher Action | When | Why |
|---------------|------|-----|
| Zombie detection | Sweep scan | Session dead but agent_state=running |
| Stuck detection | Sweep scan | Hook set but no progress for 30+ min |
| Dirty state recovery | Sweep scan | Uncommitted changes in idle worker |
| MR failure recovery | Sweep scan | MR bead with error state, no retry |
| Escalation relay | On discovery | Problems beyond worker self-repair |

The watcher does NOT need to:
- Process every successful completion
- Relay MERGE_READY to merger
- Create cleanup ephemerals for routine completions
- Transition agent_state from done→idle

---

## 4. Detailed Design

### 4.1 Worker Self-Transitions

Currently, agent state transitions are split between worker and watcher:

| Transition | Current Owner | Proposed Owner |
|-----------|--------------|---------------|
| → working | Worker (gt dispatch) | Worker (no change) |
| → done | Worker (gt done) | **REMOVED** (skip to idle) |
| done → idle | Watcher (sweep) | Worker (gt done) |
| → stuck | Worker (gt done --status=ESCALATED) | Worker (no change) |
| → running | Watcher (restart) | Watcher (no change — safety net) |

**Elimination of "done" state:** The intermediate `done` state exists solely as
a transfer signal to the watcher. With self-managed completion, workers
transition directly from `working` to `idle`. The completion metadata (exit_type,
mr_id, etc.) remains on the agent bead for audit purposes.

### 4.2 Direct Merger Notification

Currently, the watcher creates a cleanup ephemeral and messages merger when it
discovers a completion. The worker can do this directly:

```go
// In gt done, after creating MR bead:
if mrID != "" {
    // Message merger directly (already implemented, but currently
    // only as fallback alongside watcher notification)
    nudgeMerger(rigName, fmt.Sprintf("MERGE_READY %s", mrID))
}
```

The merger already discovers MRs by **polling beads** for open merge-request
issues (`ListReadyMRs()`). The message is just a wake-up signal — even if it's
missed, the merger finds the MR on its next sweep cycle. This makes the
notification idempotent and loss-tolerant.

**The merger does NOT depend on the watcher for MR discovery.** From
`engineer.go:1194-1252`, `ListReadyMRs()` queries beads directly:
```go
issues, err := e.beads.List(beads.ListOptions{
    Status:   "open",
    Label:    "gt:merge-request",
    Priority: -1,
})
```

So the watcher relay was always redundant — the merger's own polling is the
true discovery mechanism. The watcher message just reduces latency.

### 4.3 Cleanup Ephemeral Elimination

Cleanup ephemerals (`merge-requested` state) were introduced so the watcher could
track pending MRs and detect failures. With self-managed completion, this
tracking is unnecessary because:

1. **MR beads are self-tracking.** The MR bead has status (open/closed),
   retry_count, error state. The merger updates these as it processes.

2. **Failure detection moves to merger.** If a merge fails, the merger
   already creates a conflict-resolution task. The watcher doesn't need a
   ephemeral to discover this.

3. **The watcher can still detect anomalies** by scanning for stale MR beads
   (open merge-request older than threshold with no merger assignee). This
   is discovery-based — no ephemeral required.

**Migration:** Existing cleanup ephemerals can be drained naturally. The watcher
sweep's `process-cleanups` step becomes a no-op and can be removed after
migration.

### 4.4 Completion Metadata Retention

The agent bead completion metadata (exit_type, mr_id, branch, completion_time)
is still written by the worker. This serves two purposes:

1. **Audit trail:** The ledger shows exactly what each worker did.
2. **Anomaly detection:** The watcher can scan for unusual patterns
   (repeated escalations, MR failures, etc.) during sweep.

The metadata is NOT used as a transfer signal anymore. The watcher reads it
during sweep for observability, not for action routing.

### 4.5 What Changes in `gt done`

```diff
 func runDone(ctx context.Context, exitType ExitType, ...) error {
     // ... validation, push, MR creation ...

     if mrID != "" {
-        // Message watcher (watcher relays to merger)
-        nudgeWatcher(rigName, fmt.Sprintf("worker_done %s exit=%s", name, exitType))
+        // Message merger directly (watcher not in critical path)
+        nudgeMerger(rigName, fmt.Sprintf("MERGE_READY %s", mrID))
     }

-    // Set agent_state to "done" (watcher will transition to idle)
-    setAgentState(agentBeadID, "done")
+    // Set agent_state to "idle" directly (self-managed)
+    setAgentState(agentBeadID, "idle")

     // ... clear hook, sync worktree ...
 }
```

### 4.6 What Changes in Watcher Sweep

The `survey-workers` step simplifies:

```diff
 func surveyWorkers() {
     for _, worker := range allWorkers {
-        // Check for completions (done state)
-        if worker.AgentState == "done" && worker.CompletionTime != "" {
-            handleDiscoveredCompletion(worker)
-        }

         // Check for zombies (dead session, agent says running)
         if worker.AgentState == "running" && !isSessionAlive(worker) {
             handleZombie(worker)
         }

+        // Check for stuck idle workers (idle but sandbox dirty)
+        if worker.AgentState == "idle" && hasDirtyState(worker) {
+            handleDirtyIdle(worker)
+        }
+
+        // Check for stale MRs (open MR bead with no merger claim)
+        if worker.MRID != "" && isMRStale(worker.MRID) {
+            handleStaleMR(worker)
+        }
     }
 }
```

The watcher sweep gains new anomaly-detection checks but loses the
completion-processing responsibility. Net effect: faster sweep cycles
(no ephemeral creation, no merger nudging) with better anomaly coverage.

---

## 5. Edge Cases and Failure Modes

### 5.1 Worker Crashes During `gt done`

**Current:** Watcher detects `done-intent` label + live session = stuck-in-done.
Watcher kills session and continues cleanup pipeline.

**Proposed:** Same mechanism. The `done-intent` label is set at the start of
`gt done` (before any state changes). If the worker crashes mid-done:
- Agent state is still `working` (not yet transitioned to idle)
- `done-intent` label is set
- Watcher zombie detection finds: dead session + done-intent = crashed in done
- Watcher restarts session (restart-first policy, gt-dsgp)
- New session discovers done-intent, resumes `gt done`

**No change needed.** The done-intent safety mechanism is independent of who
manages the idle transition.

### 5.2 Worker Sets Idle But Push Failed

**Current:** Not possible — push happens before watcher processing.

**Proposed:** Same. The push happens early in `gt done`, before the idle
transition. If push fails, `gt done` errors out and the worker remains in
`working` state. The watcher detects this as a zombie (dead session but
agent_state=working) and restarts.

### 5.3 Merger Misses the Message

**Current:** Merger polls for MRs independently. Message is latency optimization.

**Proposed:** Same. Whether the message comes from the watcher or the worker,
the merger's polling (`ListReadyMRs`) is the reliable discovery mechanism.
A missed message adds at most one sweep cycle of latency.

### 5.4 Two Workers Complete Simultaneously

**Current:** Watcher processes them sequentially (serial bottleneck).

**Proposed:** Each worker transitions itself to idle and messages merger
independently. No serialization. The merger processes MRs from its queue
(already serialized by merge slot). This is the primary throughput improvement.

### 5.5 Watcher is Down

**Current:** Completions queue up as `done` state workers. When watcher
returns, it drains the queue. Workers are unavailable during the outage.

**Proposed:** Workers self-transition to idle and message merger directly.
Watcher downtime has **zero impact on routine completions**. The watcher is
only needed for anomaly recovery (zombies, dirty state), which can wait.

---

## 6. Migration Strategy

### Phase 1: Dual-Signal (Low Risk)

Add direct merger message to `gt done` alongside existing watcher notification.
Worker still sets `agent_state=done` (watcher still processes).

```go
// gt done sends BOTH signals
nudgeWatcher(rigName, fmt.Sprintf("worker_done %s", name))
nudgeMerger(rigName, fmt.Sprintf("MERGE_READY %s", mrID))  // NEW
```

**Validation:** Verify merger processes MRs from both signal sources.
No behavior change — just redundancy.

### Phase 2: Self-Transition (Medium Risk)

Worker sets `agent_state=idle` directly. Watcher sweep skips completion
processing (no `done` state to discover). Watcher message becomes optional.

```go
// gt done: self-manage
setAgentState(agentBeadID, "idle")
nudgeMerger(rigName, fmt.Sprintf("MERGE_READY %s", mrID))
// Watcher message: optional, for observability only
```

**Validation:** Verify workers become immediately available for new work.
Verify watcher sweep doesn't break when no `done` state workers exist.

### Phase 3: Cleanup (Low Risk)

Remove watcher completion-processing code:
- Remove `DiscoverCompletions()` function
- Remove `handleDiscoveredCompletion()` function
- Remove cleanup ephemeral creation for routine completions
- Remove `process-cleanups` sweep step (or repurpose for anomaly ephemerals)
- Update `wf-watcher-sweep.template.toml` to remove completion references

**Validation:** Full sweep cycle test. Verify zombie detection still works.

### Rollback

At each phase, rollback is trivial:
- Phase 1: Remove the extra message line
- Phase 2: Revert to `agent_state=done` and re-enable watcher processing
- Phase 3: Re-add watcher completion code

---

## 7. Impact Assessment

### Throughput

| Metric | Current | Proposed |
|--------|---------|----------|
| Completion latency | 30s-3min (watcher cycle) | ~0s (immediate) |
| Concurrent completions | Serial (1 per cycle) | Parallel (unlimited) |
| Watcher sweep time | 30-90s (processing completions) | 10-30s (anomaly scan only) |
| Worker idle time | Minutes waiting | Zero waiting |

### Dolt Pressure

No change — both flows use messages (free) and direct bead writes.

### Robustness

**Improved:** Removes single point of failure (watcher) from the critical path.
Routine completions succeed even if watcher is down, restarting, or slow.

**Preserved:** Watcher still provides safety net for edge cases (zombies,
dirty state, stale MRs). The "discover, don't track" principle is maintained.

### Complexity

**Reduced:** Eliminates cleanup ephemerals, completion discovery code, and the
done→idle state machine in the watcher. The `gt done` command becomes the
single source of truth for completion lifecycle.

---

## 8. Alignment with Design Principles

| Principle | How This Design Aligns |
|-----------|----------------------|
| **Auto-Execute Rule** | Workers become available for new work faster → higher throughput |
| **ZFC** | Worker self-reports idle (already does cleanup_status). Watcher verifies by exception |
| **Discover Don't Track** | Watcher discovers anomalies by scanning state, not by processing events |
| **Self-recycling preferred** | From worker-lifecycle-sweep.md Q2: "Prefer explicit self-recycling. Use mechanical intervention only as a safety net." This design delivers on that stated preference |
| **Persistent worker model** | Fully compatible — sandbox preservation and identity persistence are unchanged |

### The Missed Implication of gt-4ac

The persistent worker model (gt-4ac) was designed so workers survive and
get reused. But the watcher was inserted as a gatekeeper for the idle
transition, defeating part of the benefit. A worker that completes work
but can't accept new work for 3 minutes because the watcher hasn't processed
it is effectively dead capacity.

This design completes the promise of gt-4ac: persistent workers that
self-manage their full lifecycle, with the watcher as a safety net rather
than a required checkpoint.

---

## 9. Summary

**The core insight:** The watcher relay for routine completions is redundant.
The merger already discovers MRs by polling beads. The worker already
writes all the metadata. The watcher is only needed for anomaly detection —
and it can do that by scanning state, not by processing every completion.

**Three changes:**
1. Worker sets `agent_state=idle` directly (skip the `done` intermediate)
2. Worker messages merger directly (skip the watcher relay)
3. Watcher removes completion-processing code (sweep focuses on anomalies)

**Result:** Completion latency drops from minutes to zero. The watcher returns
to its designed role as an observer. The system scales linearly with worker
count instead of being bottlenecked by a single-threaded sweep loop.

---

*"Self-recycling is preferred. Mechanical intervention is the safety net,
not the primary mechanism." — worker-lifecycle-sweep.md, Q2*
