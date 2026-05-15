# Persistent Worker Pool

**Issue:** gt-lpop
**Status:** Design
**Author:** Coordinator

## Problem

Three concepts are conflated in the worker lifecycle:

| Concept | Lifecycle | Current behavior |
|---------|-----------|-----------------|
| **Identity** | Long-lived (name, CV, ledger) | Destroyed on nuke |
| **Sandbox** | Per-assignment (worktree, branch) | Destroyed on nuke |
| **Session** | Ephemeral (Claude context window) | = worker lifetime |

Consequences:
- Work is lost when workers are nuked before pushing
- 219 stale remote branches from destroyed worktrees
- Slow dispatch (~5s worktree creation per assignment)
- Lost capability record (CV, completion history)
- Idle workers were treated as waste and nuked

## Design

### Lifecycle Separation

```
IDENTITY (persistent)
  Name: "furiosa"
  Agent bead: gt-gastown-worker-furiosa
  CV: work history, languages, completion rate
  Lifecycle: created once, never destroyed (unless explicitly retired)

SANDBOX (per-assignment, reusable)
  Worktree: workers/furiosa/gastown/
  Branch: worker/furiosa/<issue>@<timestamp>
  Lifecycle: synced to main between assignments, not destroyed

SESSION (ephemeral)
  Tmux: gt-gastown-furiosa
  Claude context: cycles on compaction/transfer
  Lifecycle: independent of identity and sandbox
```

### Pool States

```
         ┌──────────┐
    ┌───►│  IDLE    │◄──── sync sandbox to main
    │    └────┬─────┘      clear hook
    │         │ gt dispatch
    │         ▼
    │    ┌──────────┐
    │    │ WORKING  │◄──── session active, hook set
    │    └────┬─────┘
    │         │ work complete
    │         ▼
    │    ┌──────────┐
    └────┤  DONE    │──── push branch, submit MR
         └──────────┘
```

No `nuke` in the happy path. Workers cycle: IDLE → WORKING → DONE → IDLE.

### Pool Management

**Pool size:** Fixed per project. Configured in `project.config.json`:
```json
{
  "worker_pool_size": 4,
  "worker_names": ["furiosa", "nux", "toast", "slit"]
}
```

**Initialization:** `gt project add` or `gt worker pool init <project>` creates N workers
with identities and worktrees. They start in IDLE state.

**Dispatch:** `gt dispatch <bead> <project>` finds an IDLE worker (already does this via
`FindIdleWorker()`), attaches work, starts session. No worktree creation needed.

**Completion:** When a worker finishes work:
1. Push branch to origin
2. Submit MR (if code changes)
3. Clear assignment_bead
4. Sync worktree: `git checkout main && git pull`
5. Set state to IDLE
6. Session stays alive or cycles — doesn't matter, identity persists

### Sandbox Sync (DONE → IDLE transition)

When work completes and MR is merged (or no code changes):

```bash
# In the worker's worktree
git checkout main
git pull origin main
git branch -D worker/furiosa/<old-issue>@<timestamp>
# Worktree is now clean, on main, ready for next assignment
```

When new work is dispatched:
```bash
# Create fresh branch from current main
git checkout -b worker/furiosa/<new-issue>@<timestamp>
# Start working
```

No worktree add/remove. Just branch operations on an existing worktree.

### Merger Integration

No changes to merger. Merger still:
1. Sees MR from worker branch
2. Reviews and merges to main
3. Deletes remote worker branch (NEW: add this step)

The worker doesn't care — it already moved to main locally during DONE → IDLE.

### Watcher Integration

Watcher sweep behavior (shipped):
- Sees idle worker → healthy state, skip
- **Stuck detection:** Worker in WORKING state for too long → escalate (don't nuke)
- **Dead session detection:** Session died but state=WORKING → restart session (not nuke worker)

### What Nuke Becomes

`gt worker nuke` is reserved for exceptional cases:
- Worker worktree is irrecoverably broken
- Need to reclaim disk space
- Decommissioning a project

It should be rare and manual, not part of normal workflow.

### Branch Pollution Solution

With persistent workers, branches have clear owners:
- Active branches: worker is WORKING on them
- Merged branches: merger deletes after merge
- Abandoned branches: worker syncs to main on DONE → IDLE, old branch deleted locally

The 219 stale branches came from nuked workers that never cleaned up. With persistent
workers, branch lifecycle is managed by the worker itself.

### One-time Cleanup

For the existing 219 stale branches:
```bash
# Delete all remote worker branches that don't belong to active workers
git branch -r | grep 'origin/worker/' | grep -v 'furiosa/gt-ziiu' | grep -v 'nux/gt-uj16' \
  | sed 's/origin\///' | xargs -I{} git push origin --delete {}
```

## Implementation Phases

### Phase 1: Stop the bleeding — SHIPPED
- Watcher no longer nukes idle workers
- `gt worker done` transitions to IDLE instead of triggering nuke
- Merger deletes remote branch after merge

### Phase 2: Pool initialization — DEFERRED
- `gt worker pool init <project>` creates N persistent workers
- Pool size configured in project.config.json
- Worktrees created once, reused across assignments

**Status:** Workers are allocated on-demand by `gt dispatch` via `FindIdleWorker()`
and `AllocateAndAdd()`. Pre-allocation is unnecessary because idle workers are
reused automatically. Pool size enforcement is a future optimization, not a blocker.

### Phase 3: Sandbox sync — SHIPPED
- DONE → IDLE transition syncs worktree to main (`done.go`)
- IDLE → WORKING creates fresh branch (no worktree add) via `ReuseIdleWorker()`
- `gt dispatch` prefers idle workers via `FindIdleWorker()`
- Branch-only reuse eliminates ~5s worktree creation overhead

### Phase 4: Session independence — SHIPPED
- Session cycling doesn't affect worker state
- Dead sessions restarted by watcher (restart-first policy, no auto-nuke)
- Transfer preserves worker identity across session boundaries
- `gt transfer` works for all roles (Coordinator, Team, Watcher, Merger, Workers)

### Phase 5: One-time cleanup — PARTIALLY SHIPPED
- Worker branch cleanup after merge: SHIPPED (landed to main; PRs #2436/#2437 closed)
- Merger notifies coordinator after merge: not yet shipped
- Pool reconciliation (`ReconcilePool`): not yet implemented

### Implementation Status Summary

| Component | Status | Key Files |
|-----------|--------|-----------|
| `gt done` (push, MR, idle, sandbox sync) | SHIPPED | `internal/cmd/done.go` |
| `gt dispatch` (idle reuse, branch-only repair) | SHIPPED | `internal/cmd/dispatch.go`, `worker_spawn.go` |
| `gt transfer` (session cycle, all roles) | SHIPPED | `internal/cmd/transfer.go` |
| Watcher sweep (zombie, stale, orphan detection) | SHIPPED | `internal/watcher/handlers.go`, `internal/worker/manager.go` |
| Cleanup pipeline (worker_done → MERGE_READY → MERGED) | SHIPPED | `internal/watcher/handlers.go`, `internal/merger/engineer.go` |
| Idle worker heresy fix (skip healthy idle) | SHIPPED | `internal/watcher/handlers.go` |
| Restart-first policy (no auto-nuke) | SHIPPED | `internal/worker/manager.go` |
| Worker branch always deleted after merge | SHIPPED | `internal/merger/engineer.go` |
| Merger notifies coordinator after merge | NOT SHIPPED | — |
| Pool size enforcement | DEFERRED | — |
| `ReconcilePool()` | DEFERRED | — |
| `gt worker pool init` command | DEFERRED | — |
