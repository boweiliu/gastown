# Batch Manager Specification

> Daemon-resident event-driven completion and stranded batch recovery.

**Status**: Implementation complete (all stories DONE)
**Owner**: Daemon subsystem
**Related**: [batch-lifecycle.md](batch-lifecycle.md) | [batch-manager.md](../daemon/batch-manager.md)

---

## 1. Problem Statement

Batches group work but don't drive it. Completion depends on a single
poll-based Supervisor sweep cycle running `gt batch check`. When Supervisor is down
or slow, batches stall. Work finishes but the loop never lands:

```
Create -> Track -> Execute -> Issue closes -> ??? -> Batch closes
```

The gap needs three capabilities:
1. **Event-driven completion** -- react to issue closes, not poll for them.
2. **Stranded recovery** -- catch batches missed by event-driven path (crash, restart, stale state).
3. **Redundant observation** -- multiple agents detect completion so no single failure blocks the loop.

---

## 2. Architecture

### 2.1 BatchManager (daemon-resident)

Two goroutines inside `gt daemon`:

| Goroutine | Trigger | What it does |
|-----------|---------|--------------|
| **Event poll** | `GetAllEventsSince` every 5s, all project stores + hq | Detects `EventClosed` / `EventStatusChanged(closed)`, calls `CheckBatchsForIssue` |
| **Stranded scan** | `gt batch stranded --json` every 30s | Feeds first ready issue via `gt dispatch`, auto-closes empty batches via `gt batch check` |

Both goroutines are context-cancellable and coordinate shutdown via `sync.WaitGroup`.

The event poll opens beads stores for all known projects (via `routes.jsonl`) plus
the workspace-level hq store. Parked/docked projects are skipped during polling. Batch
lookups always use the hq store since batches are `hq-*` prefixed. Each store
has an independent high-water mark for event IDs.

### 2.2 Shared Observer (`batch.CheckBatchsForIssue`)

Shared function called by the daemon's event poll:

| Observer | When | Entry point |
|----------|------|-------------|
| **Daemon event poll** | Close event detected in any project store or hq | `batch.CheckBatchsForIssue` (hq store passed in) |

The shared function:
1. Finds batches tracking the closed issue (SDK `GetDependentsWithMetadata` on hq store, filtered by `tracks` type)
2. Skips already-closed batches
3. Runs `gt batch check <id>` for open batches
4. If batch remains open after check, feeds next ready issue via `gt dispatch`
5. Idempotent -- safe to call multiple times for the same event

### 2.3 Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| SDK polling (not CLI streaming) | Avoids subprocess lifecycle management, simpler restart semantics |
| High-water mark (atomic int64) | Monotonically advancing, no duplicate event processing |
| One issue fed per batch per scan | Prevents batch overflow; next issue fed on next close event |
| Stranded scan as safety net | Catches batches missed by event-driven path (crash recovery) |
| Nil store disables event poll only | Stranded scan still works without beads SDK (degraded mode) |
| Resolved binary paths (PATCH-006) | BatchManager resolves `gt`/`bd` at startup to avoid PATH issues |

---

## 3. Stories

### Legend

| Status | Meaning |
|--------|---------|
| DONE | Implemented, tested, integrated |
| DONE-PARTIAL | Implemented but has known gaps |
| TODO | Not yet implemented |

### Quality Gates (for all implementation stories)

These commands must pass for every implementation story in this spec:
- `go test ./...`
- `golangci-lint run`

---

### S-01: Event-driven batch completion detection [DONE]

**Description**: When an issue closes, the daemon detects the close event via
SDK polling and triggers batch completion checks.

**Implementation**: `BatchManager.runEventPoll` + `pollEvents` in `Batch_manager.go`

**Acceptance criteria**:
- [x] Polls `GetAllEventsSince` on a 5-second interval
- [x] Detects `EventClosed` events
- [x] Detects `EventStatusChanged` where `new_value == "closed"`
- [x] Skips non-close events (close path not triggered)
- [x] Skips events with empty `issue_id`
- [x] Calls `batch.CheckBatchsForIssue` for each detected close
- [x] High-water mark advances monotonically (no duplicate processing)
- [x] Error on `GetAllEventsSince` logs and retries next interval
- [x] Nil store disables event polling (returns immediately)
- [x] Context cancellation exits cleanly

**Tests**:
- [x] `TestEventPoll_DetectsCloseEvents` -- real beads store, creates+closes issue, verifies log
- [x] `TestEventPoll_SkipsNonCloseEvents` -- create-only, no close detection

**Corrective note**: "Zero side effects" negative assertions have been added via
`TestEventPoll_SkipsNonCloseEvents_NegativeAssertion` (verifies no subprocess
calls, no close detection, and no batch activity for non-close events). Originally
tracked in S-11; now resolved.

---

### S-02: Periodic stranded batch scan [DONE]

**Description**: Every 30 seconds, scan for stranded batches (unassigned work or
empty). Feed ready work or auto-close empties.

**Implementation**: `BatchManager.runStrandedScan` + `scan` + `findStranded` + `feedFirstReady` + `closeEmptyBatch` in `Batch_manager.go`

**Acceptance criteria**:
- [x] Runs immediately on start, then every `scanInterval`
- [x] Calls `gt batch stranded --json` and parses output
- [x] For batches with `ready_count > 0`: dispatches first ready issue via `gt dispatch <id> <project> --no-boot`
- [x] For batches with `ready_count == 0`: runs `gt batch check <id>` to auto-close
- [x] Resolves issue prefix to project name via `beads.ExtractPrefix` + `beads.GetRigNameForPrefix`
- [x] Skips issues with unknown prefix (logged)
- [x] Skips issues with unknown project (logged)
- [x] Continues to next batch after dispatch failure
- [x] Context cancellation exits mid-iteration
- [x] Scan interval defaults to 30s when 0 or negative

**Tests**:
- [x] `TestScanStranded_FeedsReadyIssues` -- mock gt, verify dispatch log file
- [x] `TestScanStranded_ClosesEmptyBatchs` -- mock gt, verify check log file
- [x] `TestScanStranded_NoStrandedBatchs` -- empty list: asserts dispatch log absent, check log absent, no batch activity in logs
- [x] `TestScanStranded_DispatchFailure` -- first dispatch fails, scan continues
- [x] `TestBatchManager_ScanInterval_Configurable` -- 0 -> default, custom preserved
- [x] `TestStrandedBatchInfo_JSONParsing` -- JSON round-trip

---

### S-03: Shared batch observer function [DONE]

**Description**: A shared function for checking batch completion and feeding
the next ready issue, callable from any observer.

**Implementation**: `CheckBatchsForIssue` + `feedNextReadyIssue` in `batch/operations.go`

**Acceptance criteria**:
- [x] Finds tracking batches via `GetDependentsWithMetadata` filtered by `tracks` type
- [x] Filters out `blocks` dependencies
- [x] Skips already-closed batches
- [x] Runs `gt batch check <id>` for open batches
- [x] After check, if still open: feeds next ready issue via `gt dispatch`
- [x] Ready = open status + no assignee
- [x] Feeds one issue at a time (first match)
- [x] Handles `external:prefix:id` wrapper format via `extractIssueID`
- [x] Refreshes issue status via `GetIssuesByIDs` for cross-project accuracy
- [x] Falls back to dependency metadata if fresh status unavailable
- [x] Nil store returns immediately
- [x] Nil logger replaced with no-op (no panic)
- [x] Idempotent (calling multiple times for same issue is safe)
- [x] Returns list of checked batch IDs

**Tests**:
- [x] `TestGetTrackingBatchs_FiltersByTracksType` -- real store, blocks filtered
- [x] `TestIsBatchClosed_ReturnsCorrectStatus` -- real store, open vs closed
- [x] `TestExtractIssueID` -- all wrapper variants
- [x] `TestFeedNextReadyIssue_SkipsNonOpenIssues` -- filtering logic
- [x] `TestFeedNextReadyIssue_FindsReadyIssue` -- first match
- [x] `TestCheckBatchsForIssue_NilStore` -- returns nil
- [x] `TestCheckBatchsForIssue_NilLogger` -- no panic
- [x] `TestCheckBatchsForIssueWithAutoStore_NoStore` -- non-existent path, nil

---

### S-04: Watcher integration [REMOVED]

**Description**: Watcher batch observer removed. The daemon's multi-project event
poll (watching all project databases + hq) provides event-driven coverage for close
events from any project. The stranded scan (30s) provides backup. The watcher's core
job is worker lifecycle management -- batch tracking is orthogonal.

**History**: Originally had 6 `CheckBatchsForIssueWithAutoStore` call sites in
`handlers.go` (1 post-merge, 5 zombie cleanup paths). All were pure side-effect
notification hooks. Removed when daemon gained multi-project event polling.

---

### S-05: Merger integration [REMOVED]

**Description**: Merger batch observer removed. The daemon event poll (5s)
and watcher observer provide sufficient coverage. The merger observer was
silently broken (S-17: wrong root path) for the entire feature lifetime with
no visible impact, confirming the other two observers are sufficient. Since
beads unavailability disables the entire workspace (not just batch checks), the
"degraded mode" justification for a third observer does not hold.

**History**: Originally called `CheckBatchsForIssueWithAutoStore` after merge.
S-17 found it passed project path instead of workspace root. S-18 fixed it. Subsequently
removed as unnecessary redundancy.

---

### S-06: Daemon lifecycle integration [DONE]

**Description**: BatchManager starts and stops cleanly with the daemon.

**Implementation**: Integrated in `daemon.go` `Run()` and `shutdown()` methods.

**Acceptance criteria**:
- [x] Opens beads store at daemon startup (nil if unavailable)
- [x] Passes resolved `gtPath`/`bdPath` to BatchManager
- [x] Passes `logger.Printf` for daemon log integration
- [x] Starts after feed curator
- [x] Stops before beads store is closed (correct shutdown order)
- [x] Stop completes within bounded time (no hang)

**Tests**:
- [x] `TestDaemon_StartsManagerAndScanner` -- start + stop with mock binaries
- [x] `TestDaemon_StopsManagerAndScanner` -- stop completes within 5s

---

### S-07: Batch fields in MR beads [DONE]

**Description**: Merge-request beads carry batch tracking fields for priority
scoring and starvation prevention.

**Implementation**: `BatchID` and `BatchCreatedAt` in `MRFields` struct in `beads/fields.go`

**Acceptance criteria**:
- [x] `batch_id` field parsed and formatted
- [x] `Batch_created_at` field parsed and formatted
- [x] Supports underscore, hyphen, and camelCase key variants
- [x] Used by merger for merge queue priority scoring

---

### S-08: BatchManager lifecycle safety [DONE]

**Description**: Start/Stop are safe under edge conditions.

**Acceptance criteria**:
- [x] `Stop()` is idempotent (double-call does not deadlock)
- [x] `Stop()` before `Start()` returns immediately
- [x] `Start()` is guarded against double-call (`atomic.Bool` with `CompareAndSwap` at `Batch_manager.go:50-51,80-83`)

**Tests**:
- [x] `TestManagerLifecycle_StartStop` -- basic start + stop
- [x] `TestBatchManager_DoubleStop_Idempotent` -- double stop
- [x] `TestStart_DoubleCall_Guarded` -- second Start() is no-op, warning logged

---

### S-09: Subprocess context cancellation [DONE]

**Description**: All subprocess calls in BatchManager and observer
propagate context cancellation so daemon shutdown is not blocked by hanging
subprocesses.

**Implementation**: All `exec.Command` calls replaced with `exec.CommandContext`.
Process group killing via `setProcessGroup` + `syscall.Kill(-pid, SIGKILL)` prevents
orphaned child processes.

**Acceptance criteria**:
- [x] All `exec.Command` calls in BatchManager use `exec.CommandContext(m.ctx, ...)` (`Batch_manager.go:200,241,257`)
- [x] All `exec.Command` calls in operations.go accept and use a context parameter
- [x] Daemon shutdown completes within bounded time even if `gt` subprocess hangs (`Batch_manager_integration_test.go:154-206`)
- [x] Killed subprocesses do not leave orphaned child processes (`Batch_manager.go`, `operations.go`)

---

### S-10: Resolved binary paths in operations.go [DONE]

**Description**: Observer subprocess calls use resolved binary paths instead
of bare `"gt"` to avoid PATH-dependent behavior drift.

**Implementation**: `CheckBatchsForIssue` resolves via `exec.LookPath("gt")`
with fallback to bare `"gt"`. Threads `gtPath` parameter to `runBatchCheck`
and `dispatchIssue` in `operations.go`.

**Acceptance criteria**:
- [x] `runBatchCheck` and `dispatchIssue` accept a `gtPath` parameter
- [x] `CheckBatchsForIssue` threads resolved path
- [x] All callers updated: daemon (resolved `m.gtPath`)
- [x] Fallback to bare `"gt"` if resolution fails

---

### S-11: Test gap -- priority 1 (high blast-radius invariants) [DONE]

**Description**: Filled testing gaps for core invariants identified in the test
plan analysis.

**Tests added**:

| Test | What it proves |
|------|---------------|
| `TestFeedFirstReady_MultipleReadyIssues_DispatchesOnlyFirst` | 3 ready issues -> dispatch log contains only first issue ID |
| `TestFeedFirstReady_UnknownPrefix_Skips` | Issue prefix not in routes.jsonl -> dispatch never called, error logged |
| `TestFeedFirstReady_UnknownRig_Skips` | Prefix resolves but project lookup fails -> dispatch never called |
| `TestFeedFirstReady_EmptyReadyIssues_NoOp` | `ReadyIssues=[]` despite `ReadyCount>0` -> no crash, no dispatch |
| `TestEventPoll_SkipsNonCloseEvents_NegativeAssertion` | Asserts zero side effects (no subprocess calls, no batch activity) |

**Acceptance criteria**:
- [x] All 5 tests passing
- [x] Each test has explicit assertions (no assertion-free "no panic" tests)

---

### S-12: Test gap -- priority 2 (error paths) [DONE]

**Description**: Covered error paths that previously had no test coverage.

**Tests added**:

| Test | What it proves |
|------|---------------|
| `TestFindStranded_GtFailure_ReturnsError` | `gt batch stranded` exits non-zero -> error returned |
| `TestFindStranded_InvalidJSON_ReturnsError` | `gt` returns non-JSON stdout -> parse error returned |
| `TestScan_FindStrandedError_LogsAndContinues` | `scan()` doesn't panic when `findStranded` fails |
| `TestPollEvents_GetAllEventsSinceError` | `GetAllEventsSince` returns error -> logged, retried next interval |

**Acceptance criteria**:
- [x] All 4 tests passing
- [x] Error messages are verified in log assertions

---

### S-13: Test gap -- priority 3 (lifecycle edge cases) [DONE]

**Description**: Covered lifecycle edge cases identified in the test plan.

**Tests added**:

| Test | What it proves |
|------|---------------|
| `TestScan_ContextCancelled_MidIteration` | Large stranded list + cancel mid-loop -> exits cleanly |
| `TestScanStranded_MixedReadyAndEmpty` | Heterogeneous stranded list routes ready->dispatch and empty->check correctly |
| `TestStart_DoubleCall_Guarded` | Second `Start()` is no-op, warning logged |

**Acceptance criteria**:
- [x] All 3 tests passing

---

### S-14: Test infrastructure improvements [DONE]

**Description**: Improved test harness quality and reduced duplication.

**Items**:

| Item | Impact |
|------|--------|
| Extract `mockGtForScanTest(t, opts)` helper | Used by 5+ scan tests (`Batch_manager_test.go:57-117`) |
| Add side-effect logger to all mock scripts | All mock scripts write call logs for positive/negative assertions |
| Fix `DispatchFailure` test logger to capture `fmt.Sprintf(format, args...)` | Assertions verify rendered messages with correct IDs |
| Convert `TestScanStranded_NoStrandedBatchs` to negative test | Asserts dispatch/check logs absent |

**Acceptance criteria**:
- [x] Shared mock builder exists and is used by >= 3 scan tests (5 tests use it)
- [x] All mock scripts write to call log files (negative tests can assert empty)
- [x] No assertion-free tests remain in Batch_manager_test.go

---

### S-15: Documentation update [DONE]

**Description**: Update stale documentation to reflect current implementation.

**Items**:

| Document | Issue |
|----------|-------|
| `docs/design/daemon/batch-manager.md` | Mermaid diagram shows `bd activity --follow` but implementation uses SDK `GetAllEventsSince` polling |
| `docs/design/daemon/batch-manager.md` | Text says "Restarts with 5s backoff on stream error" -- no stream, no backoff; it's a poll-retry loop |
| `docs/design/batch/testing.md` | Row "Stream failure triggers backoff + retry loop" is stale (no stream) |
| `docs/design/batch/testing.md` | `TestDoubleStop_Idempotent` listed as gap but now exists |
| `docs/design/batch/batch-lifecycle.md` | Observer table lists Supervisor as primary third observer; implementation uses Merger |
| `docs/design/batch/batch-lifecycle.md` | "No manual close" claim is stale; `gt batch close --force` exists |
| `docs/design/batch/batch-lifecycle.md` | Relative link to batch concepts doc is broken (`../concepts/...`) |
| `docs/design/batch/spec.md` | File map test counts drifted from current suite |

**Acceptance criteria**:
- [x] Mermaid diagram shows SDK polling architecture
- [x] Text accurately describes poll-retry semantics
- [x] Testing.md reflects current test inventory
- [x] Lifecycle observer and manual-close sections match implementation
- [x] Broken links in lifecycle doc are fixed
- [x] Spec file-map counts and command list match current source

**Completion note**: Completed in this review pass; remaining ambiguity about
merger root-path semantics is tracked separately in S-17.

---

### S-16: Corrective follow-up for DONE stories [DONE]

**Description**: Add explicit corrective tasks for inaccuracies discovered in
stories marked DONE, without changing the implementation status itself.

**Rationale**: DONE stories can still contain stale supporting narrative or
inventory details after nearby refactors. Corrections are tracked explicitly
to avoid silently editing historical delivery claims.

**Scope**:
- S-01: clarify that non-close event "zero side effects" is currently partial
  until negative subprocess assertions are added (see S-11)
- S-04: replace brittle line-number call-site references with symbol/section
  anchors in `handlers.go`
- S-05: validate/clarify merger `townRoot` vs project-path argument assumptions
  for `CheckBatchsForIssueWithAutoStore`

**Acceptance criteria**:
- [x] Corrective notes are added to affected DONE stories without downgrading status
- [x] S-04 call-site references no longer depend on fixed line numbers
- [x] S-05 includes an explicit note on root-path assumptions and validation status

**Status note**: All corrective notes updated. S-01 negative assertion test now
exists (resolved). S-04 call sites already use semantic descriptions. S-05 note
updated to reflect S-17 verification findings (incorrect path, fix in S-18).

---

### S-17: Merger observer root-path verification [DONE]

**Description**: Verify whether merger passing `e.project.Path` into
`CheckBatchsForIssueWithAutoStore` is correct for batch visibility.

**Context**:
- Observer helper opens beads store under `<townRoot>/.beads/dolt`
- Merger currently passes project path, not explicitly workspace root

**Findings**:

The current behavior is **incorrect**. `e.project.Path` is a project-level path
(`<townRoot>/<rigName>`), set in `project/manager.go` as `filepath.Join(m.townRoot, name)`.
`OpenStoreForTown` constructs `<path>/.beads/dolt`, so the merger opens
`<townRoot>/<rigName>/.beads/dolt` instead of `<townRoot>/.beads/dolt`.

The project-level `.beads/` directory typically contains either a redirect file
(pointing to `coordinator/project/.beads`) or project-scoped metadata -- not the workspace-level
Dolt database that holds batch data. As a result, `beadsdk.Open` either fails
(no `dolt/` directory) or opens a project-scoped store that does not contain batch
tracking dependencies. In both cases `CheckBatchsForIssueWithAutoStore` silently
returns nil, effectively **disabling batch checks from the merger observer**.

Other observers handle this correctly:
- **Watcher**: resolves workspace root via `workspace.Find(workDir)` before calling
- **Daemon**: passes `d.config.TownRoot` directly

**Fix required**: Resolve workspace root from `e.project.Path` using `workspace.Find`
before passing to `CheckBatchsForIssueWithAutoStore`, matching the watcher pattern.
See S-18 for implementation.

**Acceptance criteria**:
- [x] Behavioral expectation is documented (workspace root vs project root)
- [x] If current behavior is correct, add code comment/spec note explaining why
- [x] If incorrect, create implementation follow-up story and cross-link here -> S-18

---

### S-18: Fix merger batch observer workspace-root path [DONE]

**Description**: Fixed the merger's `CheckBatchsForIssueWithAutoStore` call to
pass the workspace root instead of the project path, so batch checks actually open the
correct beads store.

**Context**: Identified by S-17 verification. The merger was passing `e.project.Path`
(`<townRoot>/<rigName>`) but the function expects the workspace root. This silently
disabled batch observation from the merger.

**Implementation**: `engineer.go` now resolves workspace root via `workspace.Find(e.project.Path)`
before calling `CheckBatchsForIssueWithAutoStore`, matching the watcher pattern.

**Acceptance criteria**:
- [x] Merger resolves workspace root via `workspace.Find(e.project.Path)` before calling `CheckBatchsForIssueWithAutoStore`
- [x] Pattern matches watcher implementation (graceful fallback if workspace root not found)
- [x] Import `workspace` package added to `engineer.go`
- [x] BUG(S-17) comment in `engineer.go` removed after fix

---

## 4. Critical Invariants

| # | Invariant | Category | Blast Radius | Story | Tested? |
|---|-----------|----------|-------------|-------|---------|
| I-1 | Issue close triggers `CheckBatchsForIssue` | Data | High | S-01 | Yes |
| I-2 | Non-close events produce zero side effects | Safety | Low | S-01 | Yes (`TestEventPoll_SkipsNonCloseEvents_NegativeAssertion`) |
| I-3 | High-water mark advances monotonically | Data | High | S-01 | Implicit |
| I-4 | Batch check is idempotent | Data | Low | S-03 | Yes |
| I-5 | Stranded batches with ready work get fed | Liveness | High | S-02 | Yes |
| I-6 | Empty stranded batches get auto-closed | Data | Medium | S-02 | Yes |
| I-7 | Scan continues after dispatch failure | Liveness | Medium | S-02 | Yes |
| I-8 | Context cancellation stops both goroutines | Liveness | High | S-06 | Yes |
| I-9 | One issue fed per batch per scan | Safety | Medium | S-02 | Implicit |
| I-10 | Unknown prefix/project skips issue (no crash) | Safety | Medium | S-02 | Yes (`TestFeedFirstReady_UnknownPrefix_Skips`, `_UnknownRig_Skips`) |
| I-11 | `Stop()` is idempotent | Safety | Low | S-08 | Yes |
| I-12 | Subprocess cancellation on shutdown | Liveness | High | S-09 | Yes (`TestBatchManager_ShutdownKillsHangingSubprocess`) |

---

## 5. Failure Modes

### Event Poll

| Failure | Likelihood | Recovery | Tested? |
|---------|------------|----------|---------|
| `GetAllEventsSince` error | Low | Retry next 5s interval | Yes (`TestPollEvents_GetAllEventsSinceError`) |
| Beads store nil | Medium | Event poll disabled, stranded scan continues | Yes |
| Close event with empty `issue_id` | Low | Skipped | No |
| `CheckBatchsForIssue` panics | Low | Daemon process crash -> restart | No |

### Stranded Scan

| Failure | Likelihood | Recovery | Tested? |
|---------|------------|----------|---------|
| `gt batch stranded` error | Low | Logged, skip cycle | Yes (`TestFindStranded_GtFailure_ReturnsError`) |
| Invalid JSON from `gt` | Low | Logged, skip cycle | Yes (`TestFindStranded_InvalidJSON_ReturnsError`) |
| `gt dispatch` dispatch fails | Medium | Logged, continue to next batch | Yes |
| `gt batch check` fails | Low | Logged, continue to next batch | No |
| Unknown prefix for issue | Low | Logged, skip issue | Yes (`TestFeedFirstReady_UnknownPrefix_Skips`) |
| Unknown project for prefix | Low | Logged, skip issue | Yes (`TestFeedFirstReady_UnknownRig_Skips`) |
| `gt` subprocess hangs | Low | Context cancellation kills process group | Yes (`TestBatchManager_ShutdownKillsHangingSubprocess`) |

### Lifecycle

| Failure | Likelihood | Recovery | Tested? |
|---------|------------|----------|---------|
| `Stop()` before `Start()` | Low | `wg.Wait()` returns immediately | No |
| Double `Stop()` | Low | Idempotent | Yes |
| Double `Start()` | Low | Guarded (`atomic.Bool`, no-op) | Yes (`TestStart_DoubleCall_Guarded`) |
| Subprocess blocks shutdown | Low | Context cancellation kills process group | Yes (`TestBatchManager_ShutdownKillsHangingSubprocess`) |

---

## 6. File Map

### Core Implementation

| File | Contents |
|------|----------|
| `internal/daemon/Batch_manager.go` | BatchManager: event poll + stranded scan goroutines |
| `internal/batch/operations.go` | Shared `CheckBatchsForIssue`, `feedNextReadyIssue`, `getTrackingBatchs`, `IsDispatchableType`, `isIssueBlocked` |
| `internal/beads/routes.go` | `ExtractPrefix`, `GetRigNameForPrefix` (prefix -> project resolution) |
| `internal/beads/fields.go` | `MRFields.BatchID`, `MRFields.BatchCreatedAt` (batch tracking in MR beads) |

### Integration Points

| File | How it uses batch |
|------|-------------------|
| `internal/daemon/daemon.go` | Opens multi-project beads stores, creates BatchManager in `Run()`, stops in `shutdown()` |
| `internal/watcher/handlers.go` | Batch observer removed (S-04 REMOVED) |
| `internal/merger/engineer.go` | Batch observer removed (S-05 REMOVED) |
| `internal/cmd/batch.go` | CLI: `gt batch create/status/list/add/check/stranded/close/land` |
| `internal/cmd/dispatch_batch.go` | Auto-batch creation during `gt dispatch` |
| `internal/cmd/template.go` | `executeBatchFormula` for batch-type templates |

### Tests

| File | What it tests |
|------|--------------|
| `internal/daemon/Batch_manager_test.go` | BatchManager unit tests (22 tests) |
| `internal/daemon/Batch_manager_integration_test.go` | BatchManager integration tests (2 tests, `//go:build integration`) |
| `internal/batch/store_test.go` | Observer store helpers (3 tests) |
| `internal/batch/operations_test.go` | Operations function edge cases + safety guard tests |
| `internal/daemon/daemon_test.go` | Daemon-level manager lifecycle (2 batch tests) |

### Design Documents

| File | Contents |
|------|----------|
| `docs/design/batch/batch-lifecycle.md` | Problem statement, design principles, flow diagram |
| `docs/design/batch/spec.md` | This document (includes test harness scorecard and remaining gaps) |
| `docs/design/daemon/batch-manager.md` | BatchManager architecture diagram (SDK polling + stranded scan) |

---

## 7. Review Findings -> Story Mapping

| Finding | Story |
|---------|-------|
| Stream-based batch-manager doc was stale | S-15 |
| Testing doc had stale stream/backoff and duplicate gap entries | S-15 |
| Lifecycle observer/manual-close claims were stale | S-15 |
| Spec file-map command/test counts drifted | S-15 |
| DONE stories needed explicit corrective handling | S-16 |
| Merger observer root-path ambiguity remains | S-17 (verified) |
| Merger root-path fix required | S-18 |

---

## 8. Non-Goals (This Spec)

These are documented in batch-lifecycle.md as future work but are **not** in
scope for this spec:

- Batch owner/requester field and targeted notifications (P2 in lifecycle doc)
- Batch timeout/SLA (`due_at` field, overdue surfacing) (P3 in lifecycle doc)
- Batch reopen command (implicit via add, explicit command deferred)
- Test clock injection for BatchManager (P3 -- useful but not blocking)

---

## Test Harness & Remaining Gaps

### Harness Scorecard

| Dimension | Score (1-5) | Key Gap |
|-----------|-------------|---------|
| Fixtures & Setup | 4 | `mockGtForScanTest` shared builder covers scan tests; processLine path has own setup |
| Isolation | 4 | Temp dirs + `t.Setenv(PATH)` is solid; Windows correctly skipped; no shared state |
| Observability | 4 | All mock scripts emit call logs; negative tests assert log files absent/empty |
| Speed | 4 | All batch-manager tests run quickly; no long-running interval waits in current suite |
| Determinism | 4 | No real timing dependencies; ticker tests use long intervals to avoid races |

### Test Clock Injection (P3)

**Problem**: BatchManager uses `time.Ticker` with 30s default. Testing "runs at interval" requires waiting or injecting a clock.

**Proposal**: Add `clock` field to BatchManager (interface with `NewTicker(d)`) defaulting to real time. Tests inject fake clock with immediate tick.

**Compound Value**: All periodic daemon components benefit.

**Status**: Not implemented. Tests use long intervals (10min) to prevent ticker firing during test.

### Remaining Test Gaps

- Add `TestProcessLine_EmptyIssueID` (close event with empty issue_id)
- Expand integration test coverage for multi-project event polling
