# Batch Stability Roadmap

How to get from where we are to the target UX, while preserving existing
workflows and fixing the reliability problems people actually hit.

---

## Current state

Milestone 0 complete -- all foundation PRs merged.

---

## Workflows to preserve

### Workflow A: Manual bead creation + batch dispatch

The most common pattern today:

```
bd create --type=task "Fix auth timeout"       → sh-task-1
bd create --type=task "Add validation"         → sh-task-2
bd create --type=task "Integration tests"      → sh-task-3
bd dep add sh-task-2 sh-task-1 --type=blocks
gt dispatch sh-task-1 sh-task-2 sh-task-3 gastown
```

What happens today (with PR [#1759](https://github.com/steveyegge/gastown/pull/1759)):
- Batch dispatch creates **one batch** tracking all 3 tasks
- Project is auto-resolved from bead prefixes (explicit project is deprecated)
- Tasks dispatch sequentially with 2s delays, sharing 1 batch
- `blocks` deps are respected by the daemon feeder — sh-task-2 won't
  be fed by the daemon until sh-task-1 closes (but initial dispatch
  sends all tasks regardless of deps)

What people expect:
- Tasks dispatch in dependency order
- Tasks that are blocked don't get dispatched until their blockers close
- Completed tasks land on the target branch through the merger

### Workflow B: design-to-beads + manual dispatch

```
/design-to-beads PRD.md
→ creates: root epic, sub-epics, leaf tasks
→ adds: parent-child deps (organizational hierarchy)
→ adds: blocks deps (execution ordering between tasks)
gt dispatch <task1> <task2> <task3> gastown
```

Same outcome as Workflow A: one shared batch, blocks deps respected
by the daemon feeder. The epic and sub-epic structure exists in beads
and affects daemon-driven feeding (epics are filtered by `IsDispatchableType`,
blocked tasks wait for their blockers to close).

### Workflow C: Manual batch creation

```
gt batch create "Auth overhaul" sh-task-1 sh-task-2 sh-task-3
gt dispatch sh-task-1 gastown
→ watcher feeds sh-task-2 when sh-task-1 closes (serial)
→ watcher feeds sh-task-3 when sh-task-2 closes (serial)
→ batch auto-closes when all 3 are done
```

This works on upstream/main but is serial (one task at a time) and the
watcher feed ignores blocks deps, type filters, and project capacity.

---

## Target UX

The ideal experience, achievable at the end of this roadmap:

```
/design-to-beads PRD.md
→ creates: root epic → sub-epics → leaf tasks
→ adds: parent-child (hierarchy) + blocks (ordering) deps
→ sub-epics get integration branches

gt batch stage <epic-id>
→ walks DAG, validates structure, displays route plan (tree + waves)
→ creates staged batch tracking all beads

gt batch launch <batch-id>
→ activates batch, dispatches Wave 1 tasks
→ daemon feeds subsequent waves as tasks close
→ sub-epic status auto-managed (open → in_progress → closed)
→ when sub-epic closes: dispatch sub-epic with review template
→ review template examines accumulated changes on integration branch
→ on approval: integration branch lands to main/parent branch
→ batch closes when root epic closes
```

---

## What people actually report as broken

The most common complaint: **tasks don't make it through the merger and
land on the target branch.** This is NOT a batch problem — it's a
dispatch→done→merger pipeline reliability problem. The batch system layers
on top of this pipeline.

### Critical failure points (independent of batches)

| # | Failure | Where | Severity | Recovery |
|---|---------|-------|----------|----------|
| 1 | ~~Dolt branch merge fails~~ | ~~`done.go`~~ | Resolved | Eliminated by all-on-main architecture (no per-worker Dolt branches). |
| 2 | Push fails (all 3 tiers) | `done.go:531-572` | Critical | Commits local-only. Worktree preserved. Manual recovery required. |
| 3 | MR bead creation fails | `done.go:744-752` | High | Branch pushed but no MR. Watcher notified. No auto-recovery. |
| 4 | Merger never wakes (agent stall) | Agent-level | High | Heartbeat restarts, but gap can be minutes. |
| 5 | Merge conflict blocks MR indefinitely | `engineer.go:764-786` | Medium | Conflict task must be dispatched + resolved. Stalls if project at capacity. |
| 6 | Orphaned MR (branch deleted, MR still open) | `engineer.go:1086-1198` | Medium | Anomaly detection finds it. Agent must act. |

These failures affect ALL worker work, not just batch-tracked work.
Fixing them benefits the entire system.

### Batch-specific failure points

| # | Failure | Fixed by | Status |
|---|---------|----------|--------|
| 7 | Blocked tasks get dispatched (blocks deps ignored) | `isIssueBlocked` | PR [#1759](https://github.com/steveyegge/gastown/pull/1759) (open) |
| 8 | Epics get dispatched to workers (no type filter) | `IsDispatchableType` | PR [#1759](https://github.com/steveyegge/gastown/pull/1759) (open) |
| 9 | Cross-project close events invisible to daemon | Multi-project SDK polling | **Merged** |
| 10 | Daemon doesn't feed next task after close | Continuation feeding | **Merged** |
| 11 | Merger batch check passes wrong path (never works) | Call removed | **Merged** |
| 12 | First dispatch failure abandons entire batch | Dispatch failure iteration | PR [#1759](https://github.com/steveyegge/gastown/pull/1759) (open) |
| 13 | Stranded scan is reporting-only, doesn't auto-dispatch | `feedFirstReady` | **Merged** |

---

## Phased plan

### Milestone 0: Land the foundation

**Status: Complete.**

### Milestone 1: Pipeline reliability (independent of batches)

**Goal:** Fix the dispatch→done→merger pipeline failures that cause
"tasks don't land" complaints.

This is the highest-impact work for user-reported problems. Batches
can't deliver if the underlying pipeline drops tasks.

**Work items:**

| # | Problem | Proposed fix | Complexity |
|---|---------|-------------|------------|
| 1a | ~~Dolt branch merge fails~~ | Resolved — all-on-main eliminates per-worker Dolt branches. | N/A |
| 1b | ~~Stranded MR beads on Dolt branches~~ | Resolved — no per-worker Dolt branches to strand on. | N/A |
| 1c | Merger agent stall | Harden merger heartbeat. Add a daemon-level MR queue monitor that messages (or restarts) the merger when MRs sit unprocessed beyond a threshold. | Medium |
| 1d | Merge conflicts block indefinitely | Track conflict task age. If unresolved after N hours, escalate to Coordinator/owner with the specific conflict details. | Low |

**This milestone is independent of batch work.** It can be done in
parallel by a different contributor, or sequenced after Milestone 0.

### Milestone 2: Stage and launch (`gt batch stage`, `gt batch launch`)

**Goal:** Enable the `/design-to-beads → gt batch stage → gt batch
launch` workflow.

**Depends on:** Milestone 0 (the feeder must respect blocks deps and
filter types for staged batches to work correctly).

**What ships (from Phase 2 PRD):**
- `gt batch stage <bead-id>` — DAG walking, validation, wave computation,
  tree + wave route plan display
- `gt batch launch <batch-id>` — activates batch, dispatches Wave 1
- Epic status management (open → in_progress → closed)
- Integration branch awareness (warnings when missing)
- Staged status transitions (staged_ready ↔ staged_warnings → open)

**Key design decisions already made:**
- `parent-child` is organizational only, never blocking (aligned with
  `bd ready` and beads SDK)
- Execution ordering is via explicit `blocks` deps
- Wave computation is informational (display only), runtime dispatch uses
  per-cycle `isIssueBlocked` checks
- Integration branch creation and landing remain manual (or merger
  auto-land)

**What this enables for Workflow B:**
```
/design-to-beads PRD.md
gt batch stage <root-epic-id>
→ see tree view + wave view
→ see warnings (missing integration branch, parked projects, etc.)
gt batch launch <batch-id>
→ Wave 1 tasks dispatched automatically
→ subsequent waves fed by daemon as tasks close
→ epic statuses update as children progress
→ batch closes when root epic closes
```

**What it does NOT enable yet:**
- Sub-epic review template (see Milestone 3)
- Auto-template detection for epic dispatching (Phase 3)
- Coordinator worker (Phase 3)

### Milestone 3: Sub-epic review gate

**Goal:** When all tasks under a sub-epic complete and merge into the
sub-epic's integration branch, automatically trigger a comprehensive
review of the accumulated changes before landing.

This is the missing piece between "tasks merge to integration branch"
and "integration branch lands to main."

**Current state:** Integration branch landing is purely mechanical — all
children closed + all MRs merged = ready to land. There is no review
step that examines the combined diff.

**Proposed mechanism:**

1. **Sub-epic completion trigger**: When the batch's epic status
   management (Milestone 2 US-014) closes a sub-epic, instead of (or
   before) auto-landing, dispatch the sub-epic itself with a review template.

2. **Review template**: A new template (e.g., `wf-integration-review` or
   adapt `code-review.template.toml`) that:
   - Checks out the integration branch
   - Computes the full diff against the base branch
   - Reviews the accumulated changes for:
     - Cross-task consistency
     - API contract violations between tasks
     - Missing tests for combined functionality
     - Merge conflict residue
   - Produces a review report
   - If approved: runs `gt mq integration land <sub-epic-id>`
   - If rejected: creates a fix task, blocks the sub-epic on it

3. **Batch awareness**: The batch stays open while the review runs.
   The review worker's completion triggers the next sub-epic (if the
   root epic has `blocks` deps between sub-epics) or the root epic
   closure.

**Integration points:**
- `internal/batch/operations.go` — after closing an epic, check if it
  has an integration branch. If yes, dispatch with review template instead of
  calling `gt mq integration land`.
- `internal/daemon/Batch_manager.go` — the event poll detects the
  review worker's bead close, feeds the next sub-epic or closes the
  root epic.
- New template: `wf-integration-review.template.toml`

**design-to-beads changes needed:**
- Ensure sub-epics get integration branches (either design-to-beads
  creates them, or `gt batch stage` creates them at stage time)
- Ensure `blocks` deps exist between sub-epics if sequential ordering
  is desired

### Milestone 4: Advanced dispatch (Phase 3 PRD)

**Goal:** Pluggable dispatch strategies and coordinator workers.

**What ships:**
- `FeederStrategy` interface
- Hierarchy depth validation (opt-in)
- Auto-generate `blocks` deps from hierarchy (`--infer-blocks`)
- Auto-template detection in `gt dispatch` (epic → coordinator template)
- Coordinator worker strategy
- Dynamic DAG decomposition

This milestone is the furthest out and the least urgent. The default
dispatch strategy (Phase 1 feeder with blocks checking) covers the
common case. The coordinator worker is for complex epics where
AI-driven task selection outperforms static dependency ordering.

### Milestone 5: Launch-Eater (autonomous epic grinding)

**Goal:** Layer agent-driven judgment on top of the mechanical
BatchManager so that large epics grind to completion autonomously.

**Depends on:** Milestone 2 (stage-launch) for the `gt batch stage/launch`
pipeline that mountains build on.

**Design doc:** [launch-eater.md](launch-eater.md)

**What ships:**

| Component | Description |
|-----------|-------------|
| `gt launch <epic>` | CLI: validate + stage + label + launch |
| `gt launch status` | CLI: rich progress view (active, ready, blocked, skipped) |
| `gt launch pause/resume/cancel` | CLI: lifecycle management |
| Watcher failure tracking | Sweep step: count worker failures per batch issue, auto-skip after 3 |
| Supervisor launch-audit | Sweep step: periodic progress check, dispatch Helper on stall |
| `wf-launch-helper` template | Helper template: investigate stall, dispatch orphaned issues, escalate |
| BatchManager skip-after-N | Global: stranded scan stops re-dispatching repeatedly-failed issues |
| Enhanced batch status | Global: `gt batch status` shows active workers, ready front, blocked issues |

**Key insight:** No agent holds the thread. The `launch` label on a
batch triggers sweep behavior in Watcher (failure tracking) and Supervisor
(progress audit). Helpers bring fresh context to stall investigation. The
BatchManager's mechanical feeding handles the happy path; the judgment
layers handle the 20% that gets stuck.

**Global improvements (benefit all batches):**
- Worker failure tracking (Watcher)
- Skip-after-N-failures in stranded scan (BatchManager)
- Enhanced `gt batch status` output

---

## Dependency graph

```
Milestone 0: Foundation  ← MERGED
  │
  ├──────────────────────────┐
  │                          │
  v                          v
Milestone 1: Pipeline    Milestone 2: Stage/Launch
  (done/merger fixes)    (gt batch stage/launch)
  │                          │
  │                          ├───────────────────────┐
  │                          v                       v
  │                      Milestone 3: Review gate  Milestone 5: Launch-Eater
  │                          │                       │
  └──────────┬───────────────┘                       │
             │                                       │
             v                                       │
         Milestone 4: Advanced dispatch ◄────────────┘
```

Milestones 1 and 2 are independent and can run in parallel.
Milestone 3 depends on Milestone 2 (needs epic status management).
Milestone 4 depends on both 2 and 3 being stable.
Milestone 5 depends on Milestone 2 (uses stage-launch pipeline).
Milestones 3 and 5 are independent and can run in parallel.

---

## What design-to-beads needs to change

The current design-to-beads plugin creates the right structure (epics
with parent-child deps, tasks with blocks deps). For the staged batch
workflow, it needs:

| Change | When needed | Who |
|--------|------------|-----|
| Create `blocks` deps between sub-epics (not just between tasks) | Milestone 2 | design-to-beads plugin |
| Create integration branches for sub-epics | Milestone 3 | design-to-beads plugin or `gt batch stage` |
| Output the root epic ID for `gt batch stage` input | Milestone 2 | design-to-beads plugin |

The current plugin already creates blocks deps between tasks. The gap is
inter-sub-epic ordering: if Sub-Epic A should complete before Sub-Epic B
starts, a `blocks` dep between them (or between A's last task and B's
first task) must exist.

If design-to-beads doesn't create inter-sub-epic blocks deps, `gt batch
stage` will show them dispatching in parallel (Wave 1), which may or may
not be desired. The `--infer-blocks` flag (Milestone 4) can auto-generate
these from creation order, but explicit deps from the PRD structure are
more reliable.

---

## Summary: what to do next

1. **Now:** Get PR [#1759](https://github.com/steveyegge/gastown/pull/1759) (feeder safety guards) reviewed and merged to
   complete Milestone 0.

2. **Next:** Start Milestone 1 (pipeline reliability) and/or Milestone 2
   (stage/launch) depending on priorities. Milestone 1 has broader impact
   (fixes "tasks don't land" for everyone). Milestone 2 enables the
   staged batch UX. These can run in parallel.

3. **After M2:** Milestone 3 (sub-epic review gate) and Milestone 5
   (Launch-Eater) can run in parallel. Milestone 5 is the "go to lunch"
   autonomous grinding feature. Milestone 3 is the review quality gate.

4. **Later:** Milestone 4 (advanced dispatch) when the common case is
   stable.
