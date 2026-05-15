# Understanding Gas Town

This document provides a conceptual overview of Gas Town's architecture, focusing on
the role taxonomy and how different agents interact.

## Why Gas Town Exists

As AI agents become central to engineering workflows, teams face new challenges:

- **Accountability:** Who did what? Which agent introduced this bug?
- **Quality:** Which agents are reliable? Which need tuning?
- **Efficiency:** How do you route work to the right agent?
- **Scale:** How do you coordinate agents across repos and teams?

Gas Town is an orchestration layer that treats AI agent work as structured data.
Every action is attributed. Every agent has a track record. Every piece of work
has provenance. See [Why These Features](why-these-features.md) for the full rationale,
and [Glossary](glossary.md) for terminology.

## Role Taxonomy

Gas Town has several agent types, each with distinct responsibilities and lifecycles.

### Infrastructure Roles

These roles manage the Gas Town system itself:

| Role | Description | Lifecycle |
|------|-------------|-----------|
| **Coordinator** | Global coordinator at coordinator/ | Singleton, persistent |
| **Supervisor** | Background supervisor daemon ([watchdog chain](design/watchdog-chain.md)) | Singleton, persistent |
| **Watcher** | Per-project worker lifecycle manager | One per project, persistent |
| **Merger** | Per-project merge queue processor | One per project, persistent |

### Worker Roles

These roles do actual project work:

| Role | Description | Lifecycle |
|------|-------------|-----------|
| **Worker** | Worker with persistent identity, ephemeral sessions | Watcher-managed ([details](concepts/worker-lifecycle.md)) |
| **Team** | Persistent worker with own clone | Long-lived, user-managed |
| **Helper** | Supervisor helper for infrastructure tasks | Persistent identity, Supervisor-managed |

## Batches: Tracking Work

A **batch** (🚚) is how you track batched work in Gas Town. When you kick off work -
even a single issue - create a batch to track it.

```bash
# Create a batch tracking some issues
gt batch create "Feature X" gt-abc gt-def --notify overseer

# Check progress
gt batch status hq-cv-abc

# Dashboard of active batches
gt batch list
```

**Why batches matter:**
- Single view of "what's in flight"
- Cross-project tracking (batch in hq-*, issues in gt-*, bd-*)
- Auto-notification when work lands
- Historical record of completed work (`gt batch list --all`)

The "swarm" is the set of workers currently assigned to a batch's issues.
When issues close, the batch lands. See [Batches](concepts/batch.md) for details.

## Team vs Workers

Both do project work, but with key differences:

| Aspect | Team | Worker |
|--------|------|---------|
| **Lifecycle** | Persistent (user controls) | Transient (Watcher controls) |
| **Monitoring** | None | Watcher watches, messages, recycles |
| **Work assignment** | Human-directed or self-assigned | dispatched via `gt dispatch` |
| **Git state** | Pushes to main directly | Works on branch, Merger merges |
| **Cleanup** | Manual | Automatic on completion |
| **Identity** | `<project>/team/<name>` | `<project>/workers/<name>` |

**When to use Team**:
- Exploratory work
- Long-running projects
- Work requiring human judgment
- Tasks where you want direct control

**When to use Workers**:
- Discrete, well-defined tasks
- Batch work (tracked via batches)
- Parallelizable work
- Work that benefits from supervision

## Helpers vs Team

**Helpers are NOT workers**. This is a common misconception.

| Aspect | Helpers | Team |
|--------|------|------|
| **Owner** | Supervisor | Human |
| **Purpose** | Infrastructure tasks | Project work |
| **Scope** | Narrow, focused utilities | General purpose |
| **Lifecycle** | Very short (single task) | Long-lived |
| **Example** | Boot (triages Supervisor health) | Joe (fixes bugs, adds features) |

Helpers are the Supervisor's helpers for system-level tasks:
- **Boot**: Triages Supervisor health on daemon tick
- Future helpers might handle: log rotation, health checks, etc.

If you need to do work in another project, use **worktrees**, not helpers.

## Cross-Project Work Patterns

When a team member needs to work on another project:

### Option 1: Worktrees (Preferred)

Create a worktree in the target project:

```bash
# gastown/team/joe needs to fix a beads bug
gt worktree beads
# Creates ~/gt/beads/team/gastown-joe/
# Identity preserved: BD_ACTOR = gastown/team/joe
```

Directory structure:
```
~/gt/beads/team/gastown-joe/     # joe from gastown working on beads
~/gt/gastown/team/beads-wolf/    # wolf from beads working on gastown
```

### Option 2: Dispatch to Local Workers

For work that should be owned by the target project:

```bash
# Create issue in target project
bd create --prefix beads "Fix authentication bug"

# Create batch and dispatch to target project
gt batch create "Auth fix" bd-xyz
gt dispatch bd-xyz beads
```

### When to Use Which

| Scenario | Approach |
|----------|----------|
| You need to fix something quick | Worktree |
| Work should appear in your CV | Worktree |
| Work should be done by target project team | Dispatch |
| Infrastructure/system task | Let Supervisor handle it |

## Directory Structure

The workspace root (`~/gt/`) contains infrastructure directories (`coordinator/`, `supervisor/`)
and per-project projects. Each project holds a bare repo (`.repo.git/`), a canonical beads
database (`coordinator/project/.beads/`), and agent directories (`watcher/`, `merger/`,
`team/`, `workers/`).

> For the full directory tree, see [architecture.md](design/architecture.md).

## Identity and Attribution

All work is attributed to the actor who performed it:

```
Git commits:      Author: gastown/team/joe <owner@example.com>
Beads issues:     created_by: gastown/team/joe
Events:           actor: gastown/team/joe
```

Identity is preserved even when working cross-project:
- `gastown/team/joe` working in `~/gt/beads/team/gastown-joe/`
- Commits still attributed to `gastown/team/joe`
- Work appears on joe's CV, not beads project's workers

## The Propulsion Principle

All Gas Town agents follow the same core principle:

> **If you find something on your hook, YOU RUN IT.**

This applies regardless of role. The hook is your assignment. Execute it immediately
without waiting for confirmation. Gas Town is a steam engine - agents are pistons.

## Model Evaluation and A/B Testing

Gas Town's attribution system enables objective model comparison by tracking
completion time, quality signals, and revision count per agent. Deploy different
models on similar tasks and compare outcomes with `bd stats`.

See [Why These Features](why-these-features.md) for details on work history and
capability-based routing.

## Common Mistakes

1. **Using helpers for user work**: Helpers are Supervisor infrastructure. Use team or workers.
2. **Confusing team with workers**: Team is persistent and human-managed. Workers are transient and Watcher-managed.
3. **Working in wrong directory**: Gas Town uses cwd for identity detection. Stay in your home directory.
4. **Waiting for confirmation when work is assigned**: The hook IS your assignment. Execute immediately.
5. **Creating worktrees when dispatch is better**: If work should be owned by the target project, dispatch it instead.
