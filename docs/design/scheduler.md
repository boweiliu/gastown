# Scheduler Architecture

> Config-driven capacity-controlled worker dispatch.

## Quick Start

Enable deferred dispatch and schedule some work:

```bash
# 1. Enable deferred dispatch (config-driven, no per-command flag)
gt config set scheduler.max_workers 5

# 2. Schedule work via gt dispatch (auto-defers when max_workers > 0)
gt dispatch gt-abc gastown              # Single task bead
gt dispatch gt-abc gt-def gt-ghi gastown  # Batch task beads
gt dispatch hq-cv-abc                   # Batch (schedules all tracked issues)
gt dispatch gt-epic-123                 # Epic (schedules all children)

# 3. Check what's scheduled
gt scheduler status
gt scheduler list

# 4. Dispatch manually (or let the daemon do it)
gt scheduler run
gt scheduler run --dry-run    # Preview first
```

### Dispatch Modes

The `scheduler.max_workers` config value controls dispatch behavior:

| Value | Mode | Behavior |
|-------|------|----------|
| `-1` (default) | Direct dispatch | `gt dispatch` dispatches immediately, near-zero overhead |
| `0` | Direct dispatch | Same as `-1` — `gt dispatch` dispatches immediately |
| `N > 0` | Deferred dispatch | `gt dispatch` creates dispatch context bead, daemon dispatches |

No per-invocation flag needed. The same `gt dispatch` command adapts automatically.

### Common CLI

| Command | Description |
|---------|-------------|
| `gt dispatch <bead> <project>` | Dispatch bead (direct or deferred, per config) |
| `gt dispatch <bead>... <project>` | Batch dispatch/schedule multiple beads |
| `gt dispatch <batch-id>` | Dispatch/schedule all tracked issues in batch |
| `gt dispatch <epic-id>` | Dispatch/schedule all children of epic |
| `gt scheduler status` | Show scheduler state and capacity |
| `gt scheduler list` | List all scheduled beads by project |
| `gt scheduler run` | Trigger dispatch manually |
| `gt scheduler pause` | Pause all dispatch workspace-wide |
| `gt scheduler resume` | Resume dispatch |
| `gt scheduler clear` | Remove beads from scheduler |

### Minimal Example

```bash
gt config set scheduler.max_workers 5
gt dispatch gt-abc gastown              # Defers: creates dispatch context bead
gt scheduler status                  # "Queued: 1 total, 1 ready"
gt scheduler run                     # Dispatches -> spawns worker -> closes context
```

---

## Overview

The scheduler solves **back-pressure** and **capacity control** for batched worker dispatch.

Without the scheduler, dispatching N beads spawns N workers simultaneously, exhausting API rate limits, memory, and CPU. The scheduler introduces a governor: beads enter a waiting state and the daemon dispatches them incrementally, respecting a configurable concurrency cap.

The scheduler integrates into the daemon heartbeat as **step 14** — after all agent health checks, lifecycle processing, and branch pruning. This ensures the system is healthy before spawning new work.

```
Daemon heartbeat (every 3 min)
    |
    +- Steps 0-13: Health checks, agent recovery, cleanup
    |
    +- Step 14: gt scheduler run (capacity-controlled dispatch)
         |
         +- flock (exclusive)
         +- Check paused state
         +- Load config (max_workers, batch_size)
         +- Count active workers (tmux)
         +- Query dispatch contexts (bd list --label=gt:dispatch-context)
         +- Join with bd ready to determine unblocked beads
         +- DispatchCycle.Run() — plan + execute + report
         |    +- PlanDispatch(availableCapacity, batchSize, ready)
         |    +- For each planned bead: Execute → OnSuccess/OnFailure
         +- Wake project agents (watcher, merger)
         +- Save dispatch state
```

---

## Dispatch Context Beads

Scheduling state is stored on **separate ephemeral beads** called dispatch contexts. The work bead is never modified by the scheduler.

Each dispatch context bead:
- Is created via `bd create --ephemeral` with label `gt:dispatch-context`
- Has a `tracks` dependency pointing to the work bead
- Stores all scheduling parameters as JSON in its description
- Is closed when dispatch succeeds, the bead is cleared, or the circuit breaker trips

### Why Separate Beads?

The previous approach stored scheduling metadata on the work bead's description (delimited block) and used labels (`gt:queued`) as state signals. This required:
- Two-step writes with rollback (metadata then label)
- Description sanitization to avoid delimiter collision
- Three-step dispatch cleanup (strip metadata + swap labels + retry)
- Custom key-value format/parse/strip functions (~250 lines)

Dispatch context beads eliminate all of this:
- **Single atomic create** — `bd create --ephemeral` is one operation
- **JSON format** — `json.Marshal`/`json.Unmarshal` replaces custom parsers
- **Work bead pristine** — no description mutation, no label manipulation
- **Clean lifecycle** — open context = scheduled, closed context = done

### Context Fields (JSON)

| Field | Type | Description |
|-------|------|-------------|
| `version` | int | Schema version (currently 1) |
| `work_bead_id` | string | The actual work bead being scheduled |
| `target_rig` | string | Destination project name |
| `template` | string | Template to apply at dispatch (e.g., `wf-worker-work`) |
| `args` | string | Natural language instructions for executor |
| `vars` | string | Newline-separated template variables (`key=value`) |
| `enqueued_at` | RFC3339 | Timestamp of schedule |
| `merge` | string | Merge strategy: `direct`, `mr`, `local` |
| `batch` | string | Batch bead ID (set after auto-batch creation) |
| `base_branch` | string | Override base branch for worker worktree |
| `no_merge` | bool | Skip merge queue on completion |
| `account` | string | Claude Code account handle |
| `agent` | string | Agent/runtime override |
| `hook_raw_bead` | bool | Hook without default template |
| `owned` | bool | Caller-managed batch lifecycle |
| `mode` | string | Execution mode: `ralph` (fresh context per step) |
| `dispatch_failures` | int | Consecutive failure count (circuit breaker) |
| `last_failure` | string | Most recent dispatch error message |

---

## Bead State Machine

A dispatch context transitions through these states:

```
                                  +------------------+
                                  |                  |
                                  v                  |
          +----------+    dispatch ok     +--------+ |
 schedule |  CONTEXT  | ----------------> | CLOSED | |
--------> |   OPEN    |                   | (done) | |
          +----------+                    +--------+ |
                |                                    |
                +-- 3 failures --> CLOSED (circuit-broken)
                |
                +-- gt scheduler clear --> CLOSED (cleared)
```

| State | Representation | Trigger |
|-------|---------------|---------|
| **SCHEDULED** | Open dispatch context bead | `scheduleBead()` |
| **DISPATCHED** | Closed dispatch context (reason: "dispatched") | `dispatchSingleBead()` success |
| **CIRCUIT-BROKEN** | Closed dispatch context (reason: "circuit-broken") | `dispatch_failures >= 3` |
| **CLEARED** | Closed dispatch context (reason: "cleared") | `gt scheduler clear` |

Key invariant: the work bead is **never modified** by the scheduler. All state lives on the dispatch context bead.

---

## Entry Points

### CLI Entry Points

`gt dispatch` auto-detects the dispatch mode from config and the ID type:

| Command | Direct Mode (max_workers=-1) | Deferred Mode (max_workers>0) |
|---------|-------------------------------|-------------------------------|
| `gt dispatch <bead> <project>` | Immediate dispatch | Schedule for later dispatch |
| `gt dispatch <bead>... <project>` | Batch immediate dispatch | Batch schedule |
| `gt dispatch <epic-id>` | `runEpicDispatchByID()` — dispatch all children | `runEpicScheduleByID()` — schedule all children |
| `gt dispatch <batch-id>` | `runBatchDispatchByID()` — dispatch all tracked | `runBatchScheduleByID()` — schedule all tracked |

**Detection chain** in `runDispatch`:
1. `shouldDeferDispatch()` — check `scheduler.max_workers` config
2. Batch (3+ args, last is project) — `runBatchSchedule()` or `runBatchDispatch()`
3. `--on` flag set — template-on-bead mode
4. 2 args + last is project — `scheduleBead()` or inline dispatch
5. 1 arg, auto-detect type: epic/batch/task

All schedule paths go through `scheduleBead()` in `internal/cmd/dispatch_schedule.go`.
All dispatch goes through `dispatchScheduledWork()` in `internal/cmd/capacity_dispatch.go`.

### Daemon Entry Point

The daemon calls `gt scheduler run` as a subprocess on each heartbeat (step 14):

```go
// internal/daemon/daemon.go
func (d *Daemon) dispatchScheduledWork() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()
    cmd := exec.CommandContext(ctx, "gt", "scheduler", "run")
    cmd.Env = append(os.Environ(), "GT_DAEMON=1", "BD_DOLT_AUTO_COMMIT=off")
    // ...
}
```

| Property | Value |
|----------|-------|
| Timeout | 5 minutes |
| Environment | `GT_DAEMON=1` (identifies daemon dispatch) |
| Gating | `scheduler.max_workers > 0` (deferred mode) |

---

## Schedule Path

`scheduleBead()` performs these steps in order:

1. **Validate** bead exists, project exists
2. **Cross-project guard** — reject if bead prefix doesn't match target project (unless `--force`)
3. **Idempotency** — skip if an open dispatch context already exists for this work bead
4. **Status guard** — reject if bead is assigned/in_progress (unless `--force`)
5. **Validate template** — verify template exists (lightweight, no side effects)
6. **Cook template** — `bd cook` to catch bad protos before daemon dispatch
7. **Build context fields** — `DispatchContextFields` struct with all dispatch params
8. **Create dispatch context** — `bd create --ephemeral` + `bd dep add --type=tracks` (atomic)
9. **Auto-batch** — create batch if not already tracked, store batch ID in context fields
10. **Log event** — feed event for dashboard visibility

The create is a **single atomic operation** — no two-step write, no rollback needed.

---

## Dispatch Engine

### DispatchCycle

The dispatch loop is a generic orchestrator with injected callbacks:

```go
type DispatchCycle struct {
    AvailableCapacity func() (int, error)        // Free dispatch slots (0=unlimited)
    QueryPending      func() ([]PendingBead, error) // Work items eligible for dispatch
    Execute           func(PendingBead) error     // Dispatch a single item
    OnSuccess         func(PendingBead) error     // Post-dispatch cleanup
    OnFailure         func(PendingBead, error)    // Failure handling
    BatchSize         int
    SpawnDelay        time.Duration
}
```

`Run()` internally calls `PlanDispatch(availableCapacity, batchSize, ready)` to determine what to dispatch, then executes each planned item with callbacks.

### Dispatch Flow

```
DispatchCycle.Run()
    |
    +- AvailableCapacity() → capacity = maxWorkers - activeWorkers
    |
    +- QueryPending() → getReadyDispatchContexts():
    |    +- bd list --label=gt:dispatch-context --status=open (all project DBs)
    |    +- Parse DispatchContextFields from each context bead description
    |    +- bd ready --json --limit=0 (all project DBs) → readyWorkIDs set
    |    +- Filter: context beads whose WorkBeadID is in readyWorkIDs
    |    +- Skip circuit-broken (dispatch_failures >= threshold)
    |
    +- PlanDispatch(capacity, batchSize, ready)
    |    +- Returns DispatchPlan{ToDispatch, Skipped, Reason}
    |
    +- For each planned bead:
         +- Execute: ReconstructFromContext(fields) → executeDispatch(params)
         +- OnSuccess: CloseDispatchContext(contextID, "dispatched")
         +- OnFailure: increment dispatch_failures, update context, maybe close
         +- sleep(SpawnDelay)
```

### dispatchSingleBead

Dramatically simplified — context fields are already parsed:

1. `ReconstructFromContext(b.Context)` → `DispatchParams` with `BeadID = b.WorkBeadID`
2. Call `executeDispatch(params)` — that's it

Post-dispatch cleanup is handled by callbacks:
- **OnSuccess**: `CloseDispatchContext(b.ID, "dispatched")`
- **OnFailure**: increment `dispatch_failures`, update context bead, close if circuit-broken

---

## Capacity Management

### Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `scheduler.max_workers` | *int | `-1` | Max concurrent workers (-1=direct, 0=disabled, N=deferred) |
| `scheduler.batch_size` | *int | `1` | Beads dispatched per heartbeat tick |
| `scheduler.spawn_delay` | string | `"0s"` | Delay between spawns (Dolt lock contention) |

Set via `gt config set`:

```bash
gt config set scheduler.max_workers 5    # Enable deferred dispatch
gt config set scheduler.max_workers -1   # Direct dispatch (default)
gt config set scheduler.batch_size 2
gt config set scheduler.spawn_delay 3s
```

### Dispatch Count Template

```
toDispatch = min(capacity, batchSize, readyCount)

where:
  capacity   = maxWorkers - activeWorkers (positive = that many slots, 0 or negative = no capacity)
  batchSize  = scheduler.batch_size (default 1)
  readyCount = dispatch contexts whose work bead appears in bd ready
```

### Active Worker Counting

Active workers are counted by scanning tmux sessions and matching role via `session.ParseSessionName()`. This counts **all** workers (both scheduler-dispatched and directly-dispatched) because API rate limits, memory, and CPU are shared resources.

---

## Circuit Breaker

The circuit breaker prevents permanently-failing beads from causing infinite retry loops.

| Property | Value |
|----------|-------|
| Threshold | `maxDispatchFailures = 3` |
| Counter | `dispatch_failures` field in dispatch context JSON |
| Break action | Close dispatch context (reason: "circuit-broken") |
| Reset | No automatic reset (manual intervention required) |

### Flow

```
Dispatch attempt fails
    |
    +- Increment dispatch_failures in context bead
    +- Store last_failure error message
    |
    +- dispatch_failures >= 3?
         +- Yes -> CloseDispatchContext(contextID, "circuit-broken")
         |         (context bead closed, work bead untouched)
         +- No  -> bead stays scheduled, retried next cycle
```

---

## Scheduler Control

### Pause / Resume

Pausing stops all dispatch workspace-wide. The state is stored in `.runtime/scheduler-state.json`.

```bash
gt scheduler pause    # Sets paused=true, records actor and timestamp
gt scheduler resume   # Clears paused state
```

Write is atomic (temp file + rename) to prevent corruption from concurrent writers.

### Clear

Closes dispatch context beads, removing beads from the scheduler:

```bash
gt scheduler clear              # Close ALL dispatch contexts
gt scheduler clear --bead gt-abc  # Close context for specific bead
```

### Status / List

```bash
gt scheduler status         # Summary: paused, queued count, active workers
gt scheduler status --json  # JSON output

gt scheduler list           # Beads grouped by target project, with blocked indicator
gt scheduler list --json    # JSON output
```

`list` reconciles dispatch contexts (all scheduled) with `bd ready` (unblocked work beads) to mark blocked beads.

---

## Scheduler and Batch Integration

Batches and the scheduler are complementary but distinct mechanisms. Batches track completion of related beads; the scheduler controls dispatch capacity. Two paths exist for dispatching batch work:

### Dispatch Paths

| Path | Trigger | Capacity Control | Use Case |
|------|---------|-----------------|----------|
| **Direct dispatch** | `gt dispatch <batch-id>` (max_workers=-1) | None (fires immediately) | Default mode — all issues dispatch at once |
| **Deferred dispatch** | `gt dispatch <batch-id>` (max_workers>0) | Yes (daemon heartbeat, max_workers, batch_size) | Capacity-controlled — batched with back-pressure |

**Direct dispatch** (max_workers=-1): `gt dispatch <batch-id>` calls `runBatchDispatchByID()` which dispatches all open tracked issues immediately via `executeDispatch()`. Each issue's project is auto-resolved from its bead ID prefix. No capacity control — all issues dispatch at once.

**Deferred dispatch** (max_workers>0): `gt dispatch <batch-id>` calls `runBatchScheduleByID()` which schedules all open tracked issues (creating dispatch context beads). The daemon dispatches incrementally via `gt scheduler run`, respecting `max_workers` and `batch_size`. Use this for large batches where simultaneous dispatch would exhaust resources.

### When to Use Which

- **Small batches (< 5 issues)**: Direct dispatch (default, max_workers=-1)
- **Large batches (5+ issues)**: Set `scheduler.max_workers` for capacity-controlled dispatch
- **Epics**: Same logic — `gt dispatch <epic-id>` auto-resolves mode from config

### Project Resolution

`gt dispatch <batch-id>` and `gt dispatch <epic-id>` auto-resolve the target project per-bead from its ID prefix using `beads.ExtractPrefix()` + `beads.GetRigNameForPrefix()`. Workspace-root beads (`hq-*`) are skipped with a warning since they are coordination artifacts, not dispatchable work.

---

## Safety Properties

| Property | Mechanism |
|----------|-----------|
| **Schedule idempotency** | Skip if open dispatch context already exists for work bead |
| **Work bead pristine** | Scheduler never modifies work bead description or labels |
| **Cross-project guard** | Reject if bead prefix doesn't match target project (unless `--force`) |
| **Dispatch serialization** | `flock(scheduler-dispatch.lock)` prevents double-dispatch |
| **Atomic scheduling** | Single `bd create --ephemeral` — no two-step write, no rollback |
| **Template pre-cooking** | `bd cook` at schedule time catches bad protos before daemon dispatch loop |
| **Fresh state on save** | Dispatch re-reads state before saving to avoid clobbering concurrent pause |

---

## Code Layout

| Path | Purpose |
|------|---------|
| `internal/scheduler/capacity/config.go` | `SchedulerConfig` type, defaults, `IsDeferred()` |
| `internal/scheduler/capacity/pipeline.go` | `PendingBead`, `DispatchContextFields`, `PlanDispatch()`, `ReconstructFromContext()` |
| `internal/scheduler/capacity/dispatch.go` | `DispatchCycle` type — generic dispatch orchestrator |
| `internal/scheduler/capacity/state.go` | `SchedulerState` persistence |
| `internal/beads/beads_dispatch_context.go` | Dispatch context CRUD (create, find, list, close, update) |
| `internal/cmd/dispatch.go` | CLI entry, config-driven routing |
| `internal/cmd/dispatch_schedule.go` | `scheduleBead()`, `shouldDeferDispatch()`, `isScheduled()` |
| `internal/cmd/scheduler.go` | `gt scheduler` command tree |
| `internal/cmd/scheduler_epic.go` | Epic schedule/dispatch handlers |
| `internal/cmd/scheduler_batch.go` | Batch schedule/dispatch handlers |
| `internal/cmd/capacity_dispatch.go` | `dispatchScheduledWork()`, dispatch callback wiring |
| `internal/daemon/daemon.go` | Heartbeat integration (`gt scheduler run`) |

---

## See Also

- [Watchdog Chain](watchdog-chain.md) — Daemon heartbeat, where scheduler dispatch runs as step 14
- [Batches](../concepts/batch.md) — Batch tracking, auto-batch on schedule
- [Property Layers](property-layers.md) — Labels-as-state pattern used by scheduler labels (see Operational State Events section)
