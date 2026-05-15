---
name: batch
description: The definitive guide for working with gastown's batch system -- batch work tracking, event-driven feeding, stage-launch workflow, and dispatch safety guards. Use when writing batch code, debugging batch behavior, adding batch features, testing batch changes, or answering questions about how batches work. Triggers on batch, batch manager, batch feeding, dispatch, stranded batch, feedFirstReady, feedNextReadyIssue, IsDispatchableType, isIssueBlocked, CheckBatchsForIssue, gt batch, gt dispatch, stage, launch, staged, wave.
---

# gastown Batch System

The batch system tracks batches of work across projects. A batch is a bead that `tracks` other beads via dependencies. The daemon monitors close events and feeds the next ready issue when one completes.

## Architecture

```
+================================ CREATION =================================+
|                                                                            |
|   gt dispatch <beads>      gt batch create ...     gt batch stage <epic>    |
|        |  (auto-batch)       |  (explicit)            |  (validated)     |
|        v                      v                        v                  |
|   +-----------+          +-----------+         +----------------+         |
|   |  status:  |          |  status:  |         |    status:     |         |
|   |   open    |          |   open    |         | staged:ready   |         |
|   +-----------+          +-----------+         | staged:warnings|         |
|                                                +----------------+         |
|                                                        |                  |
|                                              gt batch launch             |
|                                                        |                  |
|                                                        v                  |
|                                                +----------------+         |
|                                                |    status:     |         |
|                                                |     open       |         |
|                                                | (Wave 1 dispatched) |         |
|                                                +----------------+         |
|                                                                            |
|   All paths produce: BATCH (hq-cv-*)                                      |
|                      tracks: issue1, issue2, ...                           |
+============================================================================+
              |                              |
              v                              v
+= EVENT-DRIVEN FEEDER (5s) =+   +=== STRANDED SCAN (30s) ===+
|                              |   |                            |
|   GetAllEventsSince (SDK)    |   |   findStranded             |
|     |                        |   |     |                      |
|     v                        |   |     v                      |
|   close event detected       |   |   batch has ready issues  |
|     |                        |   |   but no active workers    |
|     v                        |   |     |                      |
|   CheckBatchsForIssue       |   |     v                      |
|     |                        |   |   feedFirstReady           |
|     v                        |   |   (iterates all ready)     |
|   feedNextReadyIssue         |   |     |                      |
|   (iterates all ready)       |   |     v                      |
|     |                        |   |   gt dispatch <next-bead>     |
|     v                        |   |   or closeEmptyBatch     |
|   gt dispatch <next-bead>       |   |                            |
|                              |   +============================+
+==============================+
```

Three creation paths (dispatch, create, stage), two feed paths, same safety guards:
- **Event-driven** (`operations.go`): Polls beads stores every ~5s for close events. Calls `feedNextReadyIssue` which checks `IsDispatchableType` + `isIssueBlocked` before dispatch. **Skips staged batches** (`isBatchStaged` check).
- **Stranded scan** (`Batch_manager.go`): Runs every 30s. `feedFirstReady` iterates all ready issues. The ready list is pre-filtered by `IsDispatchableType` in `findStrandedBatchs` (cmd/batch.go). **Only sees open batches** — staged batches never appear.

## Safety guards (the three rules)

These prevent the event-driven feeder from dispatching work it shouldn't:

### 1. Type filtering (`IsDispatchableType`)

Only leaf work items dispatch. Defined in `operations.go`:

```go
var dispatchableTypes = map[string]bool{
    "task": true, "bug": true, "feature": true, "chore": true,
    "": true, // empty defaults to task
}
```

Epics, sub-epics, batches, decisions -- all skip. Applied in both `feedNextReadyIssue` (event path) and `findStrandedBatchs` (stranded path).

### 2. Blocks dep checking (`isIssueBlocked`)

Issues with unclosed `blocks`, `conditional-blocks`, or `waits-for` dependencies skip. `parent-child` is **not** blocking -- a child task dispatches even if its parent epic is open. This is consistent with `bd ready` and workflow step behavior.

Fail-open on store errors (assumes not blocked) to avoid stalling batches on transient Dolt issues.

### 3. Dispatch failure iteration

Both feed paths iterate past failures instead of giving up:
- `feedNextReadyIssue`: `continue` on dispatch failure, try next ready issue
- `feedFirstReady`: `for range ReadyIssues` with `continue` on skip/failure, `return` on first success

## CLI commands

### Stage and launch (validated creation)

```bash
gt batch stage <epic-id>            # analyze deps, build DAG, compute waves, create staged batch
gt batch stage gt-task1 gt-task2    # stage from explicit task list
gt batch stage hq-cv-abc            # re-stage existing staged batch
gt batch stage <epic-id> --json     # machine-readable output
gt batch stage <epic-id> --launch   # stage + immediately launch if no errors
gt batch launch hq-cv-abc           # transition staged → open, dispatch Wave 1
gt batch launch <epic-id>           # stage + launch in one step (delegates to stage --launch)
```

### Create and manage

```bash
gt batch create "Auth overhaul" gt-task1 gt-task2 gt-task3
gt batch add hq-cv-abc gt-task4
```

### Check and monitor

```bash
gt batch check hq-cv-abc       # auto-closes if all tracked issues done
gt batch check                  # check all open batches
gt batch status hq-cv-abc       # single batch detail
gt batch list                   # all batches
gt batch list --all             # include closed
```

### Find stranded work

```bash
gt batch stranded               # ready work with no active workers
gt batch stranded --json        # machine-readable
```

### Close and land

```bash
gt batch close hq-cv-abc --reason "done"
gt batch land hq-cv-abc         # cleanup worktrees + close
```

### Interactive TUI

```bash
gt batch -i                     # opens interactive batch browser
gt batch --interactive          # long form
```

## Batch dispatch behavior

`gt dispatch <bead1> <bead2> <bead3>` creates **one batch** tracking all beads. The project is auto-resolved from the beads' prefixes (via `routes.jsonl`). The batch title is `"Batch: N beads to <project>"`. Each bead gets its own worker, but they share a single batch for tracking.

The batch ID and merge strategy are stored on each bead, so `gt done` can find the batch via the fast path (`getBatchInfoFromIssue`).

### Project resolution

- **Auto-resolve (preferred):** `gt dispatch gt-task1 gt-task2 gt-task3` -- resolves project from the `gt-` prefix. All beads must resolve to the same project.
- **Explicit project (deprecated):** `gt dispatch gt-task1 gt-task2 gt-task3 myrig` -- still works, prints a deprecation warning. If any bead's prefix doesn't match the explicit project, errors with suggested actions.
- **Mixed prefixes:** If beads resolve to different projects, errors listing each bead's resolved project and suggested actions (dispatch separately, or `--force`).
- **Unmapped prefix:** If a prefix has no route, errors with diagnostic info (`cat .beads/routes.jsonl | grep <prefix>`).

### Conflict handling

If any bead is already tracked by another batch, batch dispatch **errors** with detailed conflict info (which batch, all beads in it with statuses, and 4 recommended actions). This prevents accidental double-tracking.

```bash
# Auto-resolve: one batch, three workers (preferred)
gt dispatch gt-task1 gt-task2 gt-task3
# -> Created batch hq-cv-xxxxx tracking 3 beads

# Explicit project still works but prints deprecation warning
gt dispatch gt-task1 gt-task2 gt-task3 gastown
# -> Deprecation: gt dispatch now auto-resolves the project from bead prefixes.
# -> Created batch hq-cv-xxxxx tracking 3 beads
```

## Stage-launch workflow

> Implemented in [PR #1820](https://github.com/steveyegge/gastown/pull/1820). Depends on the feeder safety guards from [PR #1759](https://github.com/steveyegge/gastown/pull/1759). Design docs: `docs/design/batch/stage-launch/prd.md`, `docs/design/batch/stage-launch/testing.md`.

The stage-launch workflow is a two-phase batch creation path that validates dependencies and computes wave dispatch order **before** any work is dispatched. This is the preferred path for epic delivery.

### Input types

`gt batch stage` accepts three mutually exclusive input types:

| Input | Example | Behavior |
|-------|---------|----------|
| Epic ID | `gt batch stage bcc-nxk2o` | BFS walks entire parent-child tree, collects all descendants |
| Task list | `gt batch stage gt-t1 gt-t2 gt-t3` | Analyzes exactly those tasks |
| Batch ID | `gt batch stage hq-cv-abc` | Re-reads tracked beads from existing staged batch (re-stage) |

Mixed types (e.g., epic + task together) error. Multiple epics or multiple batches error.

### Processing pipeline

```
1. validateStageArgs     — reject empty/flag-like args
2. bdShow each arg       — resolve bead types
3. resolveInputKind      — classify Epic / Tasks / Batch
4. collectBeads          — gather BeadInfo + DepInfo (BFS for epic, direct for tasks)
5. buildBatchDAG        — construct in-memory DAG (nodes + edges)
6. detectErrors          — cycle detection + missing project checks
7. detectWarnings        — orphans, parked projects, cross-project, capacity, missing branches
8. categorizeFindings    — split into errors / warnings
9. chooseStatus          — staged:ready, staged:warnings, or abort on errors
10. computeWaves         — Kahn's algorithm (only when no errors)
11. renderDAGTree        — print ASCII dependency tree
12. renderWaveTable      — print wave dispatch plan
13. createStagedBatch   — bd create --type=batch --status=<staged-status>
```

### Wave computation (Kahn's algorithm)

Only dispatchable types participate in waves: `task`, `bug`, `feature`, `chore`. Epics are excluded.

Execution edges (create wave ordering):
- `blocks`
- `conditional-blocks`
- `waits-for`

Non-execution edges (ignored for wave ordering):
- `parent-child` — hierarchy only
- `related`, `tracks`, `discovered-from`

**Algorithm:**
1. Filter to dispatchable nodes only
2. Calculate in-degree for each node (count BlockedBy edges to other dispatchable nodes)
3. Peel loop: collect all nodes with in-degree 0 → Wave N; remove them; decrement neighbors; repeat
4. Sort within each wave alphabetically for determinism

Output example:
```
  Wave   ID              Title                     Project       Blocked By
  ──────────────────────────────────────────────────────────────────────
  1      bcc-nxk2o.1.1   Init scaffolding          bcc       —
  2      bcc-nxk2o.1.2   Shared types              bcc       bcc-nxk2o.1.1
  3      bcc-nxk2o.1.3   CLI wrapper               bcc       bcc-nxk2o.1.2

  3 tasks across 3 waves (max parallelism: 1 in wave 1)
```

### Batch status model

Four statuses with defined transitions:

| Status | Meaning |
|--------|---------|
| `staged:ready` | Validated, no errors or warnings, ready to launch |
| `staged:warnings` | Validated, no errors but has warnings. Fix and re-stage, or launch anyway. |
| `open` | Active — daemon feeds work as beads close |
| `closed` | Complete or cancelled |

Valid transitions:

| From → To | Allowed? |
|-----------|----------|
| `staged:ready` → `open` | Yes (launch) |
| `staged:warnings` → `open` | Yes (launch) |
| `staged:*` → `closed` | Yes (cancel) |
| `staged:ready` ↔ `staged:warnings` | Yes (re-stage) |
| `open` → `closed` | Yes |
| `closed` → `open` | Yes (reopen) |
| `open` → `staged:*` | **No** |
| `closed` → `staged:*` | **No** |

### Error vs warning classification

**Errors** (fatal — prevent batch creation):

| Category | Trigger | Fix |
|----------|---------|-----|
| `cycle` | Cycle detected in execution edges | Remove one blocking dep in the cycle |
| `no-project` | dispatchable bead has no project (prefix not in routes.jsonl) | Add routes.jsonl entry |

**Warnings** (non-fatal — batch created as `staged:warnings`):

| Category | Trigger |
|----------|---------|
| `orphan` | dispatchable task with no blocking deps in either direction (epic input only) |
| `blocked-project` | Bead targets a parked or docked project |
| `cross-project` | Bead on a different project than the majority |
| `capacity` | A wave has more than 5 tasks |
| `missing-branch` | Sub-epic with children but no integration branch |

### Launch behavior

`gt batch launch <batch-id>` transitions a staged batch to open and dispatches Wave 1:

1. Validate batch exists and is staged
2. Transition status to `open`
3. Re-read tracked beads, rebuild DAG, recompute waves
5. Dispatch every task in Wave 1 via `gt dispatch <beadID> <project>`
6. Individual dispatch failures do NOT abort remaining dispatches
7. Print dispatch results (checkmark/X per task)
8. Subsequent waves handled automatically by the daemon

If `gt batch launch` receives an epic or task list (not a staged batch), it delegates to `gt batch stage --launch` to stage-then-launch in one step.

### Staged batch daemon safety

**Staged batches are completely inert to the daemon.** Neither feed path processes them:

- **Event-driven feeder:** `isBatchStaged` check in `CheckBatchsForIssue` skips any batch with `staged:*` status. Fail-open on read errors (assumes not staged → processes, which is safe since a read error on a non-existent batch does nothing).
- **Stranded scan:** `gt batch stranded` only returns open batches. Staged batches never appear.

This means you can stage a batch, review the wave plan, and launch when ready — no risk of premature dispatch.

### Re-staging

Running `gt batch stage <batch-id>` on an existing staged batch re-analyzes and updates:
- Re-reads tracked beads from the batch's `tracks` deps
- Rebuilds DAG, re-detects errors/warnings, recomputes waves
- Updates status via `bd update` (e.g., `staged:warnings` → `staged:ready` if warnings resolved)
- Does NOT create a new batch or re-add track dependencies

## Testing batch changes

### Running tests

```bash
# Full batch suite (all packages)
go test ./internal/batch/... ./internal/daemon/... ./internal/cmd/... -count=1

# By area:
go test ./internal/batch/... -v -count=1                       # feeding logic
go test ./internal/daemon/... -v -count=1 -run TestBatch       # BatchManager
go test ./internal/daemon/... -v -count=1 -run TestFeedFirstReady
go test ./internal/cmd/... -v -count=1 -run TestCreateBatchBatch  # batch dispatch
go test ./internal/cmd/... -v -count=1 -run TestBatchDispatch
go test ./internal/cmd/... -v -count=1 -run TestResolveRig      # project resolution
go test ./internal/daemon/... -v -count=1 -run Integration      # real beads stores

# Stage-launch:
go test ./internal/cmd/... -v -count=1 -run TestBatchStage     # staging logic
go test ./internal/cmd/... -v -count=1 -run TestBatchLaunch    # launch + Wave 1 dispatch
go test ./internal/cmd/... -v -count=1 -run TestDetectCycles    # cycle detection
go test ./internal/cmd/... -v -count=1 -run TestComputeWaves    # wave computation
go test ./internal/cmd/... -v -count=1 -run TestBuildBatchDAG  # DAG construction
```

### Key test invariants

- `feedFirstReady` dispatches exactly 1 issue per call (first success wins)
- `feedFirstReady` iterates past failures (dispatch exit 1 -> try next)
- Parked projects are skipped in both event poll and feedFirstReady
- hq store is never skipped even if `isRigParked` returns true for everything
- High-water marks prevent event reprocessing across poll cycles
- First poll cycle is warm-up only (seeds marks, no processing)
- `IsDispatchableType("epic") == false`, `IsDispatchableType("task") == true`, `IsDispatchableType("") == true`
- `isIssueBlocked` is fail-open (store error -> not blocked)
- `parent-child` deps are NOT blocking
- Batch dispatch creates exactly 1 batch for N beads (not N batches)
- `resolveRigFromBeadIDs` errors on mixed prefixes, unmapped prefixes, workspace-level prefixes
- Cycles in blocking deps prevent staged batch creation (exit non-zero, no side effects)
- Wave 1 contains ONLY tasks with zero unsatisfied blocking deps among dispatchable nodes
- Epics and non-dispatchable types are NEVER placed in waves
- Daemon does NOT feed issues from `staged:*` batches (both feed paths skip)
- `staged:warnings` batches can still be launched (warnings are informational)
- Re-staging a batch does NOT create duplicates (updates in place)
- Launch dispatches ONLY Wave 1, not subsequent waves
- Wave computation is deterministic (same input → same output, alphabetical sort within waves)

### Deeper test engineering

See `docs/design/batch/stage-launch/testing.md` for the full stage-launch test plan (105 tests across unit, integration, snapshot, and property tiers).

See `docs/design/batch/testing.md` for the general batch test plan covering failure modes, coverage gaps, harness scorecard, test matrix, and recommended test strategy.

## Common pitfalls

- **`parent-child` is never blocking.** This is a deliberate design choice, not a bug. Consistent with `bd ready`, beads SDK, and workflow step behavior.
- **Batch dispatch errors on already-tracked beads.** If any bead is already in a batch, the entire batch dispatch fails with conflict details. The user must resolve the conflict before proceeding.
- **The stranded scan has its own blocked check.** `isReadyIssue` in cmd/batch.go reads `t.Blocked` from issue details. `isIssueBlocked` in operations.go covers the event-driven path. Don't consolidate them without understanding both paths.
- **Empty IssueType is dispatchable.** Beads default to type "task" when IssueType is unset. Treating empty as non-dispatchable would break all legacy beads.
- **`isIssueBlocked` is fail-open.** Store errors assume not blocked. A transient Dolt error should not permanently stall a batch -- the next feed cycle retries with fresh state.
- **Explicit project in batch dispatch is deprecated.** `gt dispatch beads... project` still works but prints a warning. Prefer `gt dispatch beads...` with auto-resolution.
- **Staged batches are inert.** The daemon ignores them completely. Don't expect auto-feeding until you `gt batch launch`.
- **Review `staged:warnings` before launching.** Warnings are informational — fix and re-stage if possible, or launch anyway if they're acceptable.
- **`gt batch launch` on a non-staged input delegates to stage.** If you pass an epic or task list to `launch`, it runs `stage --launch` internally. Only an already-staged batch gets the fast path.
- **Wave computation is informational.** Waves are computed at stage time for display. Runtime dispatch uses the daemon's per-cycle `isIssueBlocked` checks, which are more dynamic.
- **You cannot un-stage an open batch.** Once launched, a batch cannot return to staged status. The `open → staged:*` transition is rejected.

## Key source files

| File | What it does |
|------|-------------|
| `internal/batch/operations.go` | Core feeding: `CheckBatchsForIssue`, `feedNextReadyIssue`, `IsDispatchableType`, `isIssueBlocked` |
| `internal/daemon/Batch_manager.go` | `BatchManager` goroutines: `runEventPoll` (5s), `runStrandedScan` (30s), `feedFirstReady` |
| `internal/cmd/batch.go` | All `gt batch` subcommands + `findStrandedBatchs` type filter |
| `internal/cmd/dispatch.go` | Batch detection at ~242, auto-project-resolution, deprecation warning |
| `internal/cmd/dispatch_batch.go` | `runBatchDispatch`, `resolveRigFromBeadIDs`, `allBeadIDs`, cross-project guard |
| `internal/cmd/dispatch_batch.go` | `createAutoBatch`, `createBatchBatch`, `printBatchConflict` |
| `internal/cmd/Batch_stage.go` | `gt batch stage`: DAG walking, wave computation, error/warning detection, staged batch creation |
| `internal/cmd/Batch_launch.go` | `gt batch launch`: status transition, Wave 1 dispatch via `dispatchWave1` |
| `internal/daemon/daemon.go` | Daemon startup -- creates `BatchManager` at ~237 |
