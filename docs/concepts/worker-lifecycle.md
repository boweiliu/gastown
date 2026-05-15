# Worker Lifecycle

> Understanding the three-layer architecture of worker workers

## Overview

Workers have three distinct lifecycle layers that operate independently. The
key design principle: **workers are persistent**. They survive work completion
and can be reused across assignments.

## The Four Operating States

Workers have four operating states:

| State | Description | How it happens |
|-------|-------------|----------------|
| **Working** | Actively doing assigned work | Normal operation after `gt dispatch` |
| **Idle** | Work completed, sandbox preserved for reuse | After `gt done` completes successfully |
| **Stalled** | Session stopped mid-work | Interrupted, crashed, or timed out without being nudged |
| **Zombie** | Completed work but failed to exit | `gt done` failed during cleanup |

**State cycle (happy path):**

```
         ┌──────────┐
    ┌───>│  IDLE    │<──── sync sandbox to main, clear hook
    │    └────┬─────┘
    │         │ gt dispatch
    │         v
    │    ┌──────────┐
    │    │ WORKING  │<──── session active, hook set
    │    └────┬─────┘
    │         │ gt done
    │         v
    │    ┌──────────┐
    └────┤  IDLE    │──── push branch, submit MR, go idle
         └──────────┘
```

No `nuke` in the happy path. Workers cycle: IDLE -> WORKING -> IDLE.

**Key distinctions:**

- **Working** = actively executing. Session alive, hook set, doing work.
- **Idle** = work done, session killed, sandbox preserved. Ready for next `gt dispatch`.
- **Stalled** = supposed to be working, but stopped. Needs Watcher intervention.
- **Zombie** = finished work, tried to exit, but cleanup failed. Stuck in limbo.

## The Persistent Worker Model (gt-4ac)

**Workers persist after completing work.** When a worker finishes its assignment:

1. Signals completion via `gt done`
2. Pushes branch, submits MR to merge queue
3. Clears its hook (work is done)
4. Sets agent state to "idle"
5. Kills its own session
6. **Sandbox (worktree) is preserved for reuse**

The next `gt dispatch` reuses idle workers before allocating new ones, avoiding
the overhead of creating fresh worktrees.

### Why Persistent?

- **Faster turnaround** — Reusing an existing worktree is faster than creating one
- **Preserved identity** — The worker's agent bead, CV chain, and work history persist
- **Simpler lifecycle** — No nuke/respawn cycle between assignments
- **Done means idle** — Session dies, sandbox lives, worker awaits next assignment

### What About Pending Merges?

The Merger owns the merge queue. Once `gt done` submits work:
- The branch is pushed to origin
- Work exists in the MQ, not in the worker
- If rebase fails, Merger creates a conflict-resolution task
- The idle worker can be reused for the conflict resolution work

## The Three Layers

### The Problem: Three Concepts Were Conflated

Early designs treated workers as monolithic. This caused recurring issues:

| Concept | Lifecycle | Old behavior |
|---------|-----------|-----------------|
| **Identity** | Long-lived (name, CV, ledger) | Destroyed on nuke |
| **Sandbox** | Per-assignment (worktree, branch) | Destroyed on nuke |
| **Session** | Ephemeral (Claude context window) | = worker lifetime |

Separating these three layers means idle workers are a healthy state (not waste),
eliminates unnecessary worktree creation overhead, and preserves capability
records (CV, completion history) across assignments.

### Layer Summary

| Layer | Component | Lifecycle | Persistence |
|-------|-----------|-----------|-------------|
| **Identity** | Agent bead, CV chain, work history | Permanent | Never dies |
| **Sandbox** | Git worktree, branch | Persistent across assignments | Created on first dispatch, reused thereafter |
| **Session** | Claude (tmux pane), context window | Ephemeral per step | Cycles per step/transfer |

### Identity Layer

The worker's **identity is permanent**. It includes:

- Agent bead (created once, never deleted)
- CV chain (work history accumulates across all assignments)
- Mailbox and attribution record

Identity survives all session cycles and sandbox resets. In the HOP model, this IS
the worker — everything else is infrastructure that comes and goes. See
[Worker Identity](#worker-identity) below for details.

### Session Layer

The Claude session is **ephemeral**. It cycles frequently:

- After each workflow step (via `gt transfer`)
- On context compaction
- On crash/timeout
- After extended work periods

**Key insight:** Session cycling is **normal operation**, not failure. The worker
continues working—only the Claude context refreshes.

```
Session 1: Steps 1-2 → transfer
Session 2: Steps 3-4 → transfer
Session 3: Step 5 → gt done
```

All three sessions are the **same worker**. The sandbox persists throughout.

### Sandbox Layer

The sandbox is the **git worktree**—the worker's working directory:

```
~/gt/gastown/workers/Toast/
```

This worktree:
- Exists from first `gt dispatch` and persists across assignments
- Survives all session cycles
- Is repaired (reset to fresh branch from main) when reused by `gt dispatch`
- Contains uncommitted work, staged changes, branch state during active work

The Watcher never destroys sandboxes. Only explicit `gt worker nuke` removes them.

#### Sandbox Sync (Between Assignments)

When work completes and the worker goes idle, the sandbox is synced to main:

```bash
# In the worker's worktree (done automatically by gt done / gt dispatch)
git checkout main
git pull origin main
git branch -D worker/<name>/<old-issue>@<timestamp>
# Worktree is now clean, on main, ready for next assignment
```

When new work is dispatched:
```bash
# Create fresh branch from current main
git checkout -b worker/<name>/<new-issue>@<timestamp>
# Start working
```

No worktree add/remove between assignments. Just branch operations on an
existing worktree. This avoids the ~5s overhead of creating fresh worktrees.

### Slot Layer

The slot is the **name allocation** from the worker pool:

```bash
# Pool: [Toast, Shadow, Copper, Ash, Storm...]
# Toast is allocated to work gt-abc
```

The slot:
- Determines the sandbox path (`workers/Toast/`)
- Maps to a tmux session (`gt-gastown-Toast`)
- Appears in attribution (`gastown/workers/Toast`)
- Persists until explicit nuke

## Correct Lifecycle

```
┌─────────────────────────────────────────────────────────────┐
│                        gt dispatch                             │
│  → Find idle worker OR allocate slot from pool (Toast)    │
│  → Create/repair sandbox (worktree on new branch)          │
│  → Start session (Claude in tmux)                          │
│  → Hook workflow to worker                                │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     Work Happens                            │
│                                                             │
│  Session cycles happen here:                               │
│  - gt transfer between steps                                │
│  - Compaction triggers respawn                             │
│  - Crash → Watcher respawns                                │
│                                                             │
│  Sandbox persists through ALL session cycles               │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                  gt done (persistent model)                  │
│  → Push branch to origin                                   │
│  → Submit work to merge queue (MR bead)                    │
│  → Set agent state to "idle"                               │
│  → Kill session                                            │
│                                                             │
│  Work now lives in MQ. Worker is IDLE, not gone.          │
│  Sandbox preserved for reuse by next gt dispatch.             │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                   Merger: merge queue                     │
│  → Rebase and merge to target branch                       │
│    (main or integration branch — see below)                │
│  → Close the issue                                         │
│  → If conflict: create task for available worker          │
│                                                             │
│  Integration branch path:                                  │
│  → MRs from epic children merge to integration/<epic>      │
│  → When all children closed: land to main as one commit    │
└─────────────────────────────────────────────────────────────┘
```

## What "Recycle" Means

**Session cycling**: Normal. Claude restarts, sandbox stays, slot stays.

```bash
gt transfer  # Session cycles, worker continues
```

**Sandbox repair**: On reuse. `gt dispatch` resets the worktree to a fresh branch.

```bash
gt dispatch gt-xyz gastown  # Reuses idle Toast, repairs worktree
```

Session cycling happens constantly. Sandbox repair happens between assignments.

## Anti-Patterns

### Manual State Transitions

**Anti-pattern:**
```bash
gt worker done Toast    # DON'T: external state manipulation
gt worker reset Toast   # DON'T: manual lifecycle control
```

**Correct:**
```bash
# Worker signals its own completion:
gt done  # (from inside the worker session)

# Only explicit nuke destroys workers:
gt worker nuke Toast  # (destroys sandbox, identity persists)
```

Workers manage their own session lifecycle. External manipulation bypasses verification.

### Sandboxes Without Work (Idle vs Stalled)

An idle worker has no hook and no session — this is **normal**. It completed
its work and is waiting for the next `gt dispatch`.

A **stalled** worker has a hook but no session — this is a **failure**:
- The session crashed and wasn't nudged back to life
- The hook was lost during a crash
- State corruption occurred

**Recovery for stalled:**
```bash
# Watcher respawns the session in the existing sandbox
# Or, if unrecoverable:
gt worker nuke Toast        # Clean up the stalled worker
gt dispatch gt-abc gastown      # Respawn with fresh worker
```

### Confusing Session with Sandbox

**Anti-pattern:** Thinking session restart = losing work.

```bash
# Session ends (transfer, crash, compaction)
# Work is NOT lost because:
# - Git commits persist in sandbox
# - Staged changes persist in sandbox
# - Workflow state persists in beads
# - Hook persists across sessions
```

The new session picks up where the old one left off via `gt prime`.

## Session Lifecycle Details

Sessions cycle for these reasons:

| Trigger | Action | Result |
|---------|--------|--------|
| `gt transfer` | Voluntary | Clean cycle to fresh context |
| Context compaction | Automatic | Forced by Claude Code |
| Crash/timeout | Failure | Watcher respawns |
| `gt done` | Completion | Session exits, worker goes idle |

All except `gt done` result in continued work. Only `gt done` signals completion
and transitions the worker to idle.

## Watcher Responsibilities

The Watcher monitors workers but does NOT:
- Force session cycles (workers self-manage via transfer)
- Interrupt mid-step (unless truly stuck)
- Nuke workers after completion (persistent model)

The Watcher DOES:
- Detect and message stalled workers (sessions that stopped unexpectedly)
- Clean up zombie workers (sessions where `gt done` failed)
- Respawn crashed sessions
- Handle escalations from stuck workers (workers that explicitly asked for help)

## Worker Identity

**Key insight:** Worker *identity* is permanent; sessions are ephemeral, sandboxes are persistent.

In the HOP model, every entity has a chain (CV) that tracks:
- What work they've done
- Success/failure rates
- Skills demonstrated
- Quality metrics

The worker *name* (Toast, Shadow, etc.) is a slot from a pool — persistent until
explicit nuke. The *agent identity* that executes as that worker accumulates a
work history across all assignments.

```
WORKER IDENTITY (permanent)      SESSION (ephemeral)     SANDBOX (persistent)
├── CV chain                      ├── Claude instance     ├── Git worktree
├── Work history                  ├── Context window      ├── Branch
├── Skills demonstrated           └── Dies on transfer     └── Repaired on reuse
└── Credit for work                   or gt done              by gt dispatch
```

This distinction matters for:
- **Attribution** - Who gets credit for the work?
- **Skill routing** - Which agent is best for this task?
- **Cost accounting** - Who pays for inference?
- **Federation** - Agents having their own chains in a distributed world

## Implementation Status

As of 2026-03-07 (gt-o8g8 audit), all core lifecycle operations are **shipped and
running in production**. See [design/worker-lifecycle-sweep.md § 10](../design/worker-lifecycle-sweep.md#10-implementation-status-gt-o8g8-audit-2026-03-07)
for the full implementation matrix and [design/persistent-worker-pool.md](../design/persistent-worker-pool.md)
for phase-by-phase shipping status.

Key files:
- `internal/cmd/done.go` — work submission, sandbox sync, idle transition
- `internal/cmd/dispatch.go` + `worker_spawn.go` — idle reuse, branch-only repair
- `internal/cmd/transfer.go` — session cycling for all roles
- `internal/watcher/handlers.go` — cleanup pipeline, worker_done routing, zombie/orphan detection
- `internal/worker/manager.go` — stale detection, idle reuse (`FindIdleWorker`, `ReuseIdleWorker`), pool management

## Related Documentation

- [Overview](../overview.md) - Role taxonomy and architecture
- [Workflows](workflows.md) - Workflow execution and worker workflow
- [Propulsion Principle](propulsion-principle.md) - Why work triggers immediate execution
- [Worker Lifecycle Sweep](../design/worker-lifecycle-sweep.md) - Implementation details, cleanup stages, sweep coordination
- [Persistent Worker Pool](../design/persistent-worker-pool.md) - Pool management design and shipping status
