# The Launch-Eater: Autonomous Epic Grinding

> Judgment layer for batch-driven epic execution.
>
> **Status**: Design
> **Depends on**: Batch Milestones 0-2 (BatchManager, stage-launch)
> **Related**: [roadmap.md](roadmap.md) | [spec.md](spec.md) | [swarm-architecture.md](../../../docs/swarm-architecture.md)

---

## 1. Problem Statement

Gas Town has all the pieces for autonomous epic execution:
- BatchManager feeds ready issues as blocking deps close (event-driven, 5s)
- Stranded scan catches missed feeding (periodic, 30s)
- Stage-launch validates DAGs and computes waves (Kahn's algorithm)
- Workers execute individual issues
- Watchers monitor workers, Mergers merge

Yet users report that large epics "get stuck." They create a launch of
beads, launch a batch, go away for a few hours, and come back to find the
batch stalled at 40% with no indication of why.

**Root cause**: The BatchManager is mechanical. It feeds the next ready
issue when one closes, but it cannot reason about failure patterns, make
skip decisions, or escalate intelligently. When a worker fails repeatedly
on the same issue, the mechanical system re-dispatches it endlessly. When a
subtle blocking condition exists outside the dep graph, nothing notices.

The Launch-Eater adds a judgment layer — agent-driven stall detection,
skip-after-N-failures, intelligent escalation, and completion notification
— on top of the existing mechanical feeding.

---

## 2. Design Principle: No Agent Holds the Thread

The reason single-coordinator approaches fail is **hysteresis**. Any agent
maintaining an "I'm driving this epic" loop will lose that thread at
compaction. Even with the epic assigned, the re-primed agent doesn't remember
the coordination context.

The Launch-Eater sidesteps this entirely:

- **The epic IS the thread.** The beads ARE the state.
- **No agent needs to remember anything.** Each check discovers state fresh.
- **Helpers bring fresh context every time.** Zero hysteresis by construction.
- **The label triggers sweep behavior.** No persistent coordinator needed.

This aligns with core Gas Town principles:
- **ZFC**: Agents decide, Go transports. BatchManager is transport; Helpers make judgment calls.
- **Eventual Completion**: Any Helper can check any launch. Different agents, same outcome.
- **Discover, Don't Track**: `bd ready --epic=X` and batch status derive state.
- **Float over Integer**: A stuck issue doesn't halt the launch — work flows around it.

---

## 3. Architecture: Four-Layer Grinding

```
Layer 0: BATCH MANAGER (mechanical, Go daemon — already built)
    Event-driven feeding + stranded scan
    Handles the happy path: issue closes → feed next ready

Layer 1: WATCHER (reactive, per-project — enhancement)
    Worker failure tracking for launch batch issues
    Same issue failed 3+ times → mark blocked, skip, feed next

Layer 2: SUPERVISOR DOG (periodic, cross-project — new)
    "Has this launch progressed since last check?"
    Fresh Helper investigates stalls with full context
    Makes judgment calls: skip, restructure, escalate
    Notifies Coordinator on stalls and completion

Layer 3: COORDINATOR (strategic, user-facing — enhancement)
    Receives stall escalations from Layer 2
    Cross-project judgment calls
    Notifies user on completion or unrecoverable stalls
```

**Layer 0** already exists and handles ~80% of batch execution.
**Layers 1-2** are the Launch-Eater — they handle the 20% that gets stuck.
**Layer 3** is the escalation path for the ~2% that requires human judgment.

### Why Four Layers?

Redundant monitoring is resilience. If the Watcher misses a completion
(crash, compaction), the BatchManager catches it (5s event poll). If the
BatchManager feeds a bad issue repeatedly, the Watcher catches the failure
pattern. If both miss a stall, the Supervisor Helper catches it on the next sweep
cycle. Each layer operates independently and discovers state from beads.

---

## 4. The `launch` Label

A launch is a batch with the `launch` label. No new entity types,
no new database schema. The label IS the opt-in for Layers 1-2.

```bash
# Activate the Launch-Eater on an epic
gt launch <epic-id>

# Internally:
#   1. gt batch stage <epic-id>          ← validate DAG, compute waves
#   2. bd update <batch> --add-label launch  ← trigger judgment layers
#   3. gt batch launch <batch-id>       ← dispatch wave 1, BatchManager takes over

# Check progress
gt launch status [epic-id|batch-id]

# Pause/resume (keeps label, stops/starts dispatch)
gt launch pause <epic-id|batch-id>
gt launch resume <epic-id|batch-id>

# Cancel (removes label, leaves batch for manual management)
gt launch cancel <epic-id|batch-id>
```

Regular batches (no `launch` label) continue working exactly as today.
The `launch` label opts a batch into enhanced stall detection,
skip-after-N-failures, and active progress monitoring.

### When to Use Mountains vs Regular Batches

| Scenario | Use |
|----------|-----|
| Batch dispatch of 3-5 tasks | Regular batch (BatchManager is sufficient) |
| Large epic with 10+ tasks and DAG deps | Launch |
| Cross-project epic | Launch (needs the Helper's cross-project visibility) |
| "Go to lunch and come back to it done" | Launch |
| Quick parallel tasks, no deps | Regular batch |

---

## 5. Layer 1: Watcher Failure Tracking

### Problem

When a worker fails on a launch issue, the BatchManager's stranded
scan re-dispatches it. If the issue has a fundamental problem (bad description,
impossible task, missing context), this creates an infinite dispatch-fail loop.

### Enhancement

The Watcher already monitors worker completions. Add failure tracking
for issues belonging to launch batches:

```
WATCHER SWEEP — launch failure tracking:

For each worker that exited without completing its issue:
  issue = worker's assigned bead
  batch = tracking batch for this issue (if any)
  if batch has "launch" label:
    increment failure count for this issue (stored as issue note or label)
    if failure_count >= 3:
      bd update <issue> --status=blocked --add-label launch:skipped
      bd update <issue> --notes "Skipped by Launch-Eater after 3 worker failures"
      log: "Launch: skipped <issue> after 3 failures"
      # BatchManager's next feed will skip this issue (blocked status)
      # and feed the next ready issue instead
```

**Failure count storage**: Use a label like `launch:failures:3` on the
issue. Labels are cheap, queryable, and visible in `bd show`. No new
schema needed.

**Why the Watcher and not the BatchManager?** The Watcher already observes
worker lifecycle. It knows whether a worker completed successfully or
crashed. The BatchManager only sees issue status changes — it can't
distinguish "worker failed" from "worker is still working."

### Skip Semantics

A skipped issue (`launch:skipped` label, `blocked` status) is:
- Excluded from the ready front (blocked status)
- Visible in `gt launch status` output
- Escalated to Coordinator by Layer 2 (Supervisor Helper)
- Recoverable: `bd update <issue> --status=open --remove-label launch:skipped`

The launch continues grinding around the skipped issue. If the skipped
issue was blocking other work in the DAG, those dependents remain blocked.
The Helper reports this in its stall diagnosis.

---

## 6. Layer 2: Supervisor Helper Launch Audit

### The Core Loop

The Supervisor's sweep template gains a `launch-audit` step:

```
SUPERVISOR SWEEP — launch-audit step:

mountains = bd list --label launch --status=open --type=batch
for each launch:
  helper_needed = false

  # Progress check (compare against last audit)
  current_closed = count of closed issues in this batch
  last_closed = read from launch:audit:<batch-id> label on supervisor bead

  if current_closed > last_closed:
    # Making progress — update audit mark, continue
    update launch:audit:<batch-id> = current_closed

  else if current_closed == total_issues:
    # Complete — dispatch Helper for cleanup + notification
    helper_needed = true
    helper_task = "complete"

  else:
    # No progress since last check — dispatch Helper to investigate
    helper_needed = true
    helper_task = "stall"

  if helper_needed:
    dispatch launch-helper template to a Helper with batch-id and task type
```

### The Launch Helper Template

`wf-launch-helper.template.toml` — a short-lived Helper template for
investigating launch progress:

```toml
[template]
name = "launch-helper"
description = "Investigate launch batch progress"
type = "worker"

[template.variables]
batch_id = { required = true }
task = { required = true }  # "stall" or "complete"

[[template.steps]]
name = "investigate"
description = """
You are a Launch Helper investigating a launch batch.

Batch: {{batch_id}}
Task: {{task}}

If task is "stall":
  1. Run: gt batch status {{batch_id}}
  2. Identify why no progress:
     - Are there skipped issues (launch:skipped label)?
     - Are all remaining issues blocked? By what?
     - Are workers active but slow?
     - Is the merger backed up?
  3. If there are ready issues with no workers: dispatch them
  4. If all remaining issues are skipped/blocked:
     Mail Coordinator: "Launch {{batch_id}} stalled: N skipped, M blocked.
     Remaining DAG cannot progress without intervention."
  5. If workers are active: this is fine, no action needed

If task is "complete":
  1. Run: gt batch status {{batch_id}}
  2. Verify all tracked issues are closed
  3. If any skipped issues remain:
     Mail Coordinator: "Launch {{batch_id}} finished with N skipped issues.
     Review skipped work: [list issue IDs]"
  4. If all clean:
     Mail Coordinator: "Launch {{batch_id}} complete. N issues closed in Xh Ym."
  5. Run: gt batch close {{batch_id}}
"""
```

### Helper Properties That Make This Work

- **Fresh context**: The Helper starts with zero state. It reads the batch
  and beads from scratch. No hysteresis from prior sessions.
- **Narrow scope**: One batch, one question ("stalled?" or "complete?").
  Fits easily in a single context window.
- **Ephemeral**: Does its job, reports, dies. No long-running coordination.
- **Cross-project visibility**: Helpers have worktrees into multiple projects. They can
  check beads status across projects for cross-project batches.

### Audit Frequency

The Supervisor sweep cycle determines how often mountains are audited. Current
Supervisor sweep runs on a feed-driven + heartbeat model. For mountains, the
relevant question is: "How long can a launch be stalled before someone
notices?"

- **Target**: Stall detected within 10-15 minutes
- **Mechanism**: Supervisor's heartbeat interval (daemon pokes Supervisor every
  5-10 minutes depending on activity). Each heartbeat runs the sweep
  template including the launch-audit step.
- **Cost**: One `bd list --label launch` query per sweep cycle (cheap),
  plus one Helper spawn per stalled launch (only when needed).

---

## 7. Layer 3: Coordinator Notification

The Coordinator receives two types of launch mail from Helpers:

### Stall Notification

```
Subject: Launch stalled: <batch-title>
Body:
  Batch: hq-cv-abc "Rebuild auth system"
  Progress: 23/35 closed (65%)
  Stalled for: 15 minutes

  Skipped issues (worker failure):
    gt-xyz "Migrate session store" (failed 3 times)
    gt-abc "Update JWT validation" (failed 3 times)

  Blocked issues (DAG):
    gt-def "Integration tests" (blocked by gt-xyz)
    gt-ghi "E2E tests" (blocked by gt-def)

  Active workers: 0
  Ready issues: 0

  Action needed: Review skipped issues. Possible fixes:
    bd update gt-xyz --status=open --remove-label launch:skipped  (retry)
    bd close gt-xyz --reason="Descoped"  (skip permanently, unblocks dependents)
```

### Completion Notification

```
Subject: Launch complete: <batch-title>
Body:
  Batch: hq-cv-abc "Rebuild auth system"
  Result: 33/35 closed, 2 skipped
  Elapsed: 3h 42m

  Skipped issues:
    gt-xyz "Migrate session store" (failed 3 times — needs manual review)
    gt-abc "Update JWT validation" (failed 3 times — needs manual review)
```

### Coordinator's Role

The Coordinator is NOT part of the grinding loop. It receives notifications and
can take action, but the launch grinds autonomously without Coordinator
involvement. The Coordinator's actions are:

- **Retry a skipped issue**: `bd update <id> --status=open --remove-label launch:skipped`
- **Permanently skip**: `bd close <id> --reason="Descoped"` (unblocks dependents)
- **Notify user**: Forward the stall/completion notification
- **Restructure DAG**: Remove or add dependencies to work around a blocker

---

## 8. User Experience

### Starting a Launch

```bash
$ gt launch gt-epic-auth-rebuild

Validating epic structure...
  Epic: gt-epic-auth-rebuild "Rebuild auth system"
  Tasks: 35 (31 dispatchable, 4 epics)
  Waves: 6 (computed from blocking deps)
  Max parallelism: 4

  Warnings:
    gt-migrate-sessions has no description (may cause worker confusion)

  Errors: none

Creating batch...
  Batch: hq-cv-m7x "Launch: Rebuild auth system"
  Label: launch

Launching Wave 1 (4 tasks)...
  dispatched gt-foundation-types → gastown
  dispatched gt-config-schema → gastown
  dispatched gt-test-fixtures → gastown
  dispatched gt-error-types → gastown

Launch active. BatchManager will feed subsequent waves.
Supervisor will audit progress every ~10 minutes.
Check status: gt launch status hq-cv-m7x
```

### Checking Status

```bash
$ gt launch status

Active Mountains:
  hq-cv-m7x "Rebuild auth system"
    Progress: ████████████░░░░░░░░ 23/35 (65%)
    Active: 3 workers working
    Ready: 1 issue waiting for worker
    Blocked: 6 issues (DAG deps)
    Skipped: 2 issues (worker failures)
    Elapsed: 1h 47m

  hq-cv-n9y "Migrate database layer"
    Progress: ██████████████████░░ 18/20 (90%)
    Active: 2 workers working
    Elapsed: 52m
```

### Detailed Status

```bash
$ gt launch status hq-cv-m7x

Launch: hq-cv-m7x "Rebuild auth system"
Epic: gt-epic-auth-rebuild

Progress: 23/35 closed (65%)
Elapsed: 1h 47m
Wave: 4 of 6

Completed (23):
  ✓ gt-foundation-types, gt-config-schema, gt-test-fixtures, ...

Active (3):
  ⟳ gt-session-handler (worker: gastown/nux, 12m)
  ⟳ gt-middleware-chain (worker: gastown/furiosa, 8m)
  ⟳ gt-rate-limiter (worker: gastown/max, 3m)

Ready (1):
  ○ gt-cache-layer (unblocked, waiting for worker)

Skipped (2):
  ⊘ gt-migrate-sessions (failed 3 times — no description)
  ⊘ gt-jwt-validation (failed 3 times — test dependency missing)

Blocked (6):
  ◌ gt-auth-integration (needs: gt-session-handler, gt-jwt-validation⊘)
  ◌ gt-e2e-auth-tests (needs: gt-auth-integration)
  ...

Stall risk: gt-jwt-validation⊘ blocks 4 downstream issues.
  Fix: bd update gt-jwt-validation --status=open --remove-label launch:skipped
  Or:  bd close gt-jwt-validation --reason="Descoped"
```

---

## 9. Global Improvements (All Batches)

The Launch-Eater design reveals improvements that benefit ALL batches,
not just mountains. These should be applied globally:

### 9.1 Worker Failure Tracking

Even non-launch batches benefit from knowing "this issue has failed 3
times." The Watcher should track failure counts for all batch-tracked
issues, not just launch ones. The difference: mountains auto-skip after
3 failures; regular batches just log a warning.

### 9.2 Stall Detection in Stranded Scan

The BatchManager's stranded scan currently feeds the first ready issue.
Add: if the same issue has been dispatched 3+ times and keeps appearing as
stranded, stop re-dispatching it and log a warning. This prevents the
infinite dispatch-fail loop for all batches.

### 9.3 Progress Visibility

`gt batch status` should show the same rich information as
`gt launch status` — active workers, ready front, blocked issues,
skipped issues. This is useful for all batches, not just mountains.

---

## 10. Relationship to Swarm Architecture

The [swarm architecture doc](../../../docs/swarm-architecture.md) describes
a design where swarms are persistent workflows coordinated by a dedicated
agent. The Launch-Eater achieves the same outcome through a different
mechanism:

| Swarm Architecture | Launch-Eater |
|--------------------|----------------|
| Dedicated coordinator agent | No coordinator — sweep steps + Helpers |
| Swarm workflow tracks state | Label triggers sweep behavior |
| Coordinator survives via workflow | Helpers bring fresh context (no survival needed) |
| Ready Front computed by coordinator | Ready Front computed by BatchManager + Helpers |
| Recovery via workflow resume | Recovery via beads state discovery |

The Launch-Eater is the implementation path for the swarm architecture's
goals. The swarm doc's "ready front" model, "gate issues," and "batch
management" concepts apply directly. The difference is mechanism:
sweep-driven grinding instead of coordinator-driven grinding.

The swarm architecture doc should be updated to reference the Launch-Eater
as the concrete implementation.

---

## 11. Implementation Plan

See [roadmap.md](roadmap.md) Milestone 5 for the phased implementation.

### Summary of Changes

| Component | Change | Scope |
|-----------|--------|-------|
| `gt launch` CLI | New command (stage + label + launch) | ~200 lines |
| `gt launch status` | New command (query + format) | ~300 lines |
| `gt launch pause/resume/cancel` | Label management | ~100 lines |
| Watcher sweep template | Failure tracking for batch issues | Template step |
| Supervisor sweep template | Launch audit step | Template step |
| `wf-launch-helper.template.toml` | Helper template for stall investigation | New template |
| BatchManager stranded scan | Skip after N failures (global) | ~30 lines |
| `gt batch status` | Enhanced output (active, ready, blocked) | ~100 lines |

### What Does NOT Change

- Batch data model (still `hq-cv-*` beads with `tracks` deps)
- BatchManager event poll (still 5s, still feeds on close)
- BatchManager stranded scan (still 30s, enhanced with skip logic)
- Stage-launch workflow (launch uses it directly)
- Worker lifecycle (unchanged)
- Merger (unchanged)

---

## 12. Open Questions

1. **Should `gt launch` auto-undock a docked project?** If the epic's issues
   route to a docked project, should the launch automatically undock it?
   Current thinking: no — require the project to be active. Mountains only
   grind active projects.

2. **Max concurrent workers per launch.** Should mountains have a
   configurable concurrency limit? The BatchManager feeds one issue per
   close event. For mountains, we might want to dispatch multiple ready
   issues when a wave transition happens (e.g., wave 1 completes, wave 2
   has 8 ready issues — dispatch all 8, not one-at-a-time).

3. **Launch-to-launch dependencies.** Can one launch depend on
   another? Probably not needed initially — cross-launch deps are just
   cross-issue deps in the DAG.

4. **Notification channel.** Coordinator mail is the current notification path.
   Should mountains also support webhook/Slack notification for the user?
   Defer to future work.
