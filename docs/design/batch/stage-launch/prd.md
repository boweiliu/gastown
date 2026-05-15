# PRD: Batch Stage & Launch (`gt batch stage`, `gt batch launch`)

## Problem

**1. No pre-flight validation before dispatching work.** `gt dispatch` dispatches tasks immediately with no structural analysis. A user who runs `gt dispatch task1 task2 task3` has no way to know beforehand that task2 has a circular dependency, task3's project doesn't exist, or that all three tasks will try to run in parallel when they should be serialized. Problems surface only after workers are spawned and work is underway — at which point cleanup is manual and error-prone.

**2. No visibility into execution order.** The daemon's feeder respects `blocks` dependencies at runtime, but the user has no way to preview the dispatch plan. When `/design-to-beads` creates an epic with 15 tasks across 3 sub-epics, the user cannot see which tasks will run in Wave 1, which are blocked until Wave 2, or whether the dependency graph even makes sense. The execution order is a black box until tasks start (or fail to start).

**3. Batch dispatch dispatches without dependency-aware ordering.** `gt dispatch task1 task2 task3` iterates through tasks sequentially and spawns a worker for each, regardless of dependency ordering. The daemon's `isIssueBlocked` check prevents *re-feeding* blocked tasks after a close event, but the initial batch dispatch does not check blocking deps before spawning. This means tasks that should wait for blockers to complete get workers spawned and may execute prematurely.

**4. No staged state for batches.** Batches go directly from creation to `open`, which immediately makes them eligible for daemon feeding. There is no way to create a batch, inspect it, validate it, and *then* activate it. Users who want a "dry run" have no mechanism — the act of creating a batch is the act of launching it.

**5. No single command bridges design-to-beads output to dispatch.** The workflow from `/design-to-beads` (which creates epics, tasks, and dependencies) to actual worker dispatch requires the user to manually identify leaf tasks, determine the correct project, and run `gt dispatch`. For a complex epic with sub-epics and cross-task dependencies, this manual step is tedious and error-prone — exactly the kind of work the system should automate.

## Overview

Add `gt batch stage` and `gt batch launch` commands that enable a structured workflow for dispatching work: analyze bead dependencies, compute execution waves, surface problems, create a staged batch, and dispatch Wave 1 tasks. This bridges the gap between `/design-to-beads` output (or manually created beads) and reliable multi-task batch execution.

The core insight: staging is a pre-flight check that catches dependency cycles, routing problems, orphaned tasks, and capacity issues **before** any workers are spawned. Launching is the act of activating a validated batch.

## Goals

- Enable `gt batch stage <epic-id | task1 task2 ... | batch-id>` as a pre-flight analysis and staging command
- Enable `gt batch launch <batch-id>` as an alias for `gt batch stage <batch-id> --launch`
- Compute execution waves from blocks deps and display them alongside the DAG tree
- Surface errors (cycles, invalid projects) and warnings (parked projects, missing branches, capacity) with clear categorization
- Create staged batches with `staged_ready` or `staged_warnings` status based on analysis results
- On launch, dispatch only Wave 1 (unblocked) tasks; daemon feeds subsequent waves
- Support `--json` for programmatic consumption (design-to-beads pipeline)

## Quality Gates

These commands must pass for every user story:

```bash
go vet ./... && go build ./... && go test ./internal/cmd/... ./internal/batch/... ./internal/daemon/... -count=1
```

## User Stories

### US-001: Bead validation and DAG construction

**Description:** As a user, I want `gt batch stage` to validate that all specified beads exist and construct the dependency graph, so that I catch typos and missing beads before any work is dispatched.

**Acceptance Criteria:**
- [ ] `gt batch stage <bead-id>` runs `bd show` on each bead and errors if any don't exist
- [ ] For epic input: walks the full parent-child tree (sub-epics, their children, recursively) to collect all descendant beads
- [ ] For task list input: analyzes exactly the given tasks (no auto-expansion to parent epic)
- [ ] For batch input: reads the batch's tracked beads via `bd dep list --type=tracks`
- [ ] Constructs an in-memory DAG from `blocks`, `conditional-blocks`, and `waits-for` dependencies between the collected beads
- [ ] `parent-child` deps are recorded for hierarchy display but do NOT create execution edges

### US-002: Error detection (blocks staging)

**Description:** As a user, I want staging to detect fatal structural problems and refuse to create a batch, so that I never launch a fundamentally broken work plan.

**Acceptance Criteria:**
- [ ] Dependency cycles (circular `blocks`/`conditional-blocks`/`waits-for` chains) are detected and reported with the cycle path
- [ ] Beads with no valid project (prefix not mapped in `routes.jsonl`, or resolves to empty) are detected and reported
- [ ] When errors are found: no batch is created, no bead statuses are modified
- [ ] Error output lists each error with the affected bead ID(s) and a suggested fix
- [ ] Exit code is non-zero when errors are found

### US-003: Warning detection (allows staging with acknowledgment)

**Description:** As a user, I want staging to surface non-fatal issues as warnings so that I can decide whether to fix them or proceed to launch with `--force`. (Note: `--force` is a launch-time flag, not a stage flag. Staging with warnings always creates a `staged_warnings` batch; the decision point is at launch.)

**Acceptance Criteria:**
- [ ] Orphan detection (epic input only): tasks not reachable from the epic's descendant tree, or tasks with no blocking deps from any other staged task (isolated in the wave graph). For task-list input, isolation is expected (all tasks are explicitly selected) — no orphan warning.
- [ ] Missing integration branches on sub-epics (warn, don't block)
- [ ] Parked or unavailable target projects
- [ ] Cross-project routing: beads that resolve to different projects than expected
- [ ] Capacity estimation: number of workers needed per wave vs available capacity (informational)
- [ ] Warnings are clearly distinguished from errors in output
- [ ] When only warnings (no errors): batch is created with `staged_warnings` status

### US-004: Wave computation

**Description:** As a user, I want to see which tasks can run in parallel (waves) based on their dependency ordering, so that I understand the execution plan before launching.

**Acceptance Criteria:**
- [ ] Wave 1 = all tasks with no unsatisfied `blocks`/`conditional-blocks`/`waits-for` deps within the staged set
- [ ] Wave N+1 = tasks whose blockers are all in Wave N or earlier
- [ ] Tasks not reachable by any blocking dep chain are placed in Wave 1 (maximum parallelism)
- [ ] Epics and non-dispatchable types are excluded from waves (they're containers, not dispatchable work)
- [ ] Wave computation handles the full descendant set (for epic input) or just the given tasks (for task list input)

### US-005: DAG tree display

**Description:** As a user, I want to see the full epic hierarchy as an ASCII tree, so that I understand the structural organization of the work.

**Acceptance Criteria:**
- [ ] For epic input: display the full parent-child tree with indentation showing nesting
- [ ] Each node shows: bead ID, title, type, status, and target project
- [ ] Sub-epics are visually distinct from leaf tasks
- [ ] Blocked tasks show their blocker(s) inline
- [ ] For task list input: show a flat list (no tree, since there's no epic hierarchy)
- [ ] Tree is displayed before the wave table

### US-006: Wave dispatch plan display

**Description:** As a user, I want to see a wave-by-wave dispatch plan table showing what will be dispatched and when, so that I can validate the execution order.

**Acceptance Criteria:**
- [ ] Table shows Wave number, task IDs, task titles, target project, and blockers (if any)
- [ ] Displayed after the DAG tree
- [ ] Summary line: total waves, total tasks, estimated parallelism per wave
- [ ] For `staged_warnings` batches: warnings are printed after the wave table

### US-007: Batch creation with staged status

**Description:** As a user, I want `gt batch stage` to create a batch with `staged_ready` or `staged_warnings` status, so that the batch exists in beads and can be launched later.

**Acceptance Criteria:**
- [ ] `staged_ready`: batch created when analysis finds no errors and no warnings
- [ ] `staged_warnings`: batch created when analysis finds warnings but no errors
- [ ] No batch created when analysis finds errors
- [ ] Batch tracks all dispatchable beads in the analyzed set via `tracks` deps
- [ ] Batch description includes wave count, task count, and staging timestamp
- [ ] Batch ID is printed to console for use with `gt batch launch`
- [ ] Re-staging an existing batch-id re-analyzes and updates the status (may change from ready to warnings or vice versa)

### US-008: Launch — dispatch Wave 1

**Description:** As a user, I want `gt batch stage --launch` (or `gt batch launch`) to activate the batch and dispatch all Wave 1 tasks, so that work begins immediately after validation.

**Acceptance Criteria:**
- [ ] Transitions batch status from `staged_ready` to `open`
- [ ] For `staged_warnings` batches: requires `--force` flag to launch (otherwise errors with warning summary)
- [ ] Dispatches all Wave 1 tasks via internal Go dispatch functions (one worker per task, no auto-batch creation — the staged batch already tracks these tasks)
- [ ] Subsequent waves are NOT dispatched — the daemon feeds them as Wave 1 tasks close
- [ ] If a Wave 1 dispatch fails, continues to next task (same as batch dispatch iteration behavior)

### US-009: Launch console output

**Description:** As a user, I want rich console output after launching, so that I know how to monitor progress.

**Acceptance Criteria:**
- [ ] Prints batch ID and `gt batch status <batch-id>` command
- [ ] Prints wave summary (how many waves, how many tasks per wave)
- [ ] Lists each dispatched Wave 1 task with its assigned worker
- [ ] Prints hint: `gt batch -i` for interactive TUI monitoring
- [ ] Explains that subsequent waves are fed automatically by the daemon as tasks complete

### US-010: `gt batch launch` as alias

**Description:** As a user, I want `gt batch launch <bead-id>` to work as an alias for `gt batch stage <bead-id> --launch`, so that I have a clean two-step workflow (stage then launch) or a one-step workflow (launch directly).

**Acceptance Criteria:**
- [ ] `gt batch launch <epic-id>` = `gt batch stage <epic-id> --launch`
- [ ] `gt batch launch <task1> <task2>` = `gt batch stage <task1> <task2> --launch`
- [ ] `gt batch launch <batch-id>` activates an already-staged batch (no re-analysis needed if status is `staged_ready`)
- [ ] `gt batch launch <batch-id>` on a `staged_warnings` batch requires `--force`
- [ ] `gt batch launch <batch-id>` on an already-`open` batch errors: "batch is already launched"

### US-011: JSON output mode

**Description:** As a user (or automation tool like design-to-beads), I want `gt batch stage --json` to output machine-readable analysis results, so that I can programmatically consume the staging output.

**Acceptance Criteria:**
- [ ] `--json` flag outputs structured JSON to stdout
- [ ] JSON includes: `errors` array, `warnings` array, `waves` array (each with task list), `tree` (nested structure), `batch_id` (if created), `status` (staged_ready | staged_warnings | error)
- [ ] Human-readable output is suppressed when `--json` is used
- [ ] Non-zero exit code on errors (same as non-JSON mode)

## Functional Requirements

- FR-1: `gt batch stage` must accept three input forms: epic ID, space-separated task IDs, or batch ID
- FR-2: Epic DAG walking must use the Go SDK (`beads.List(ListOptions{Parent: rootID})`) recursively, consistent with `workflow_dag.go:buildDAG`. The SDK approach avoids subprocess overhead per tree level and is faster for deep hierarchies. Integration tests stub the beads store, not the `bd` CLI.
- FR-3: Wave computation must use topological sort on the blocks/conditional-blocks/waits-for subgraph
- FR-4: Cycle detection must use standard graph cycle detection (DFS with back-edge detection or Kahn's algorithm failure)
- FR-5: Project resolution must use existing `beads.ExtractPrefix` + `beads.GetRigNameForPrefix` infrastructure
- FR-6: Staged batch must use `bd create --type=batch --status=staged_ready` (or staged_warnings)
- FR-7: Launch must dispatch Wave 1 tasks by calling internal Go dispatch functions directly (not `gt dispatch` CLI). Using `gt dispatch` would create a separate auto-batch per task via `createAutoBatch`, duplicating the staged batch. The internal dispatch path reuses the dispatch command's core logic (project resolution, worker spawning) without batch creation overhead.
- FR-8: Re-staging an existing batch must update its status and re-compute waves without creating a duplicate
- FR-9: `gt batch launch` must be registered as a subcommand of `gt batch` in `internal/cmd/batch.go`
- FR-10: Mixed input types (e.g., epic ID + task IDs in the same invocation) must be detected and rejected with a clear error message suggesting separate invocations

## Non-Goals (Out of Scope)

- Sub-epic status management (open → in_progress → closed) — deferred to later milestone
- Integration branch creation — manual or design-to-beads responsibility
- Auto-template detection for epic dispatching — Milestone 4
- Coordinator worker — Milestone 4
- `--infer-blocks` flag to auto-generate deps from hierarchy — Milestone 4
- Capacity plumbing (`isRigAtCapacity` callback) — not yet wired into feeder
- The design-to-beads plugin itself — reference only, not part of this PR

## Technical Considerations

- Wave computation is informational (display only at stage time). Runtime dispatch uses the daemon's per-cycle `isIssueBlocked` checks, which is more dynamic and handles external status changes.
- The `staged_ready` and `staged_warnings` statuses are new beads statuses that need to be recognized by the batch system. The daemon should NOT feed issues from staged batches (only `open` batches get fed).
- Epic DAG walking may involve cross-project beads. Use the existing `routes.jsonl` infrastructure for project resolution but be prepared for external references.
- The `--json` output should be stable enough for design-to-beads to depend on, but mark it as experimental in v1.

### Staged status: codebase impact (blocked on Q1)

The current codebase assumes batches are always `open` or `closed`. Adding staged statuses requires changes in multiple places:

- **`ensureKnownBatchStatus` (`batch.go:96-108`)** — Currently only accepts `"open"` and `"closed"`. Must be extended to recognize staged statuses. Called from 14+ callsites (add, close, land, check, isDispatchableBead).
- **`validateBatchStatusTransition` (`batch.go:110-131`)** — Must define valid transitions: `staged_ready→open`, `staged_warnings→open` (with `--force`). Currently only knows `open↔closed`.
- **Daemon feeding has TWO paths that need guards:**
  - **Event-driven path** (`operations.go:57,70`): `isBatchClosed` checks only `status == "closed"`. A staged batch passes this check (not closed) and would be fed. This path needs an explicit `isStagedBatch(status)` guard in `CheckBatchsForIssue`.
  - **Stranded scan path** (`batch.go:1231`): uses `bd list --status=open` which excludes non-open statuses. This path is safe by accident — staged batches won't appear in the query results.
- **`bd doctor` regex** (`^[a-z][a-z0-9_]*$`) rejects colons in custom statuses. The status format decision (Q1) must account for this.

### Key files expected to be modified:
- `internal/cmd/batch.go` — new `stage` and `launch` subcommands; update `ensureKnownBatchStatus` and `validateBatchStatusTransition` for staged statuses
- `internal/cmd/Batch_stage.go` — new file: staging logic, DAG walking, wave computation, display
- `internal/cmd/Batch_launch.go` — new file: launch logic, Wave 1 dispatch via internal Go functions (not `gt dispatch` CLI, to avoid auto-batch creation)
- `internal/batch/operations.go` — add staged-batch guard in `CheckBatchsForIssue` (event-driven path); `isBatchClosed` is insufficient
- `internal/daemon/Batch_manager.go` — ensure staged batches are not fed (stranded scan path is already safe via `--status=open` query)

### Existing code to reuse:
- `beads.ExtractPrefix`, `beads.GetRigNameForPrefix` (`internal/beads/routes.go`) — project resolution from bead prefix
- `IsDispatchableType` (`internal/batch/operations.go`) — filter non-dispatchable types from waves
- `isIssueBlocked` (`internal/batch/operations.go`) — already handles blocks dep checking (fail-open on store errors)
- `createBatchBatch` pattern (`internal/cmd/dispatch_batch.go:309`) — the *pattern* of creating a batch + adding `tracks` deps is reusable, but the function itself requires a `rigName` parameter (used in title "Batch: N beads to \<project\>"). Stage/launch batches may span multiple projects, so a new batch creation function is needed with a different title format and staged status.
- `workspace.FindFromCwdOrError` (`internal/workspace/find.go:98`) — workspace resolution (used by existing batch commands via `getTownBeadsDir`)
- `workflow_dag.go:computeTiers` (`internal/cmd/workflow_dag.go:213-262`) — prior art for Kahn's algorithm (topological sort). **Note:** this function silently `break`s on cycle detection (`line 237`) rather than returning an error with the cycle path. New `detectCycles` must be a separate function that returns the cycle path, as required by US-002 AC-1.

## Success Metrics

- `gt batch stage <epic-id>` correctly identifies all descendant tasks and computes waves
- Cycle detection catches all circular dependency chains
- Wave 1 dispatch only sends unblocked tasks
- Daemon correctly ignores staged batches (only feeds open ones)
- `--json` output is parseable and includes all analysis data
- Round-trip works: `/design-to-beads` → `gt batch stage` → `gt batch launch` → tasks dispatched in correct order

## Open Questions

1. **OPEN:** Should `staged_ready` and `staged_warnings` be proper beads statuses (requiring SDK changes), or simulated via labels/description fields? The beads SDK's `bd doctor` regex (`^[a-z][a-z0-9_]*$`) rejects colons, so `staged_ready` triggers warnings. Options under consideration: colons (canonical but triggers doctor warnings), underscores (`staged_ready`), or hyphens (`staged-ready`). This is architecturally blocking — the daemon guard (I-7), FR-6, and most integration tests depend on the answer.

2. **RESOLVED: Informational only.** Wave computation does not consider project capacity when splitting waves. Capacity is surfaced as an informational warning (US-003 AC-5) but does not affect wave assignment. Capacity plumbing (`isRigAtCapacity`) is deferred (see Non-Goals).

3. **RESOLVED: Preserve and update in place.** Re-staging an existing batch preserves the original batch ID and updates its status and wave data. No new batch is created. This is consistent with FR-8 and I-9 (no duplicates).
