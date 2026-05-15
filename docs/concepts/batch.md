# Batches

Batches are the primary unit for tracking batched work across projects.

## Quick Start

```bash
# Create a batch tracking some issues
gt batch create "Feature X" gt-abc gt-def --notify overseer

# Check progress
gt batch status hq-cv-abc

# List active batches (the dashboard)
gt batch list

# See all batches including landed ones
gt batch list --all
```

## Concept

A **batch** is a persistent tracking unit that monitors related issues across
multiple projects. When you kick off work - even a single issue - a batch tracks it
so you can see when it lands and what was included.

```
                 🚚 Batch (hq-cv-abc)
                         │
            ┌────────────┼────────────┐
            │            │            │
            ▼            ▼            ▼
       ┌─────────┐  ┌─────────┐  ┌─────────┐
       │ gt-xyz  │  │ gt-def  │  │ bd-abc  │
       │ gastown │  │ gastown │  │  beads  │
       └────┬────┘  └────┬────┘  └────┬────┘
            │            │            │
            ▼            ▼            ▼
       ┌─────────┐  ┌─────────┐  ┌─────────┐
       │  nux    │  │ furiosa │  │  amber  │
       │(worker)│  │(worker)│  │(worker)│
       └─────────┘  └─────────┘  └─────────┘
                         │
                    "the swarm"
                    (ephemeral)
```

## Batch vs Swarm

| Concept | Persistent? | ID | Description |
|---------|-------------|-----|-------------|
| **Batch** | Yes | hq-cv-* | Tracking unit. What you create, track, get notified about. |
| **Swarm** | No | None | Ephemeral. "The workers currently on this batch's issues." |
| **Stranded Batch** | Yes | hq-cv-* | A batch with ready work but no workers assigned. Needs attention. |

When you "kick off a swarm", you're really:
1. Creating a batch (the tracking unit)
2. Assigning workers to the tracked issues
3. The "swarm" is just those workers while they're working

When issues close, the batch lands and notifies you. The swarm dissolves.

## Batch Lifecycle

```
OPEN ──(all issues close)──► LANDED/CLOSED
  ↑                              │
  └──(add more issues)───────────┘
       (auto-reopens)
```

| State | Description |
|-------|-------------|
| `open` | Active tracking, work in progress |
| `closed` | All tracked issues closed, notification sent |

Adding issues to a closed batch reopens it automatically.

## Commands

### Create a Batch

```bash
# Track multiple issues across projects
gt batch create "Deploy v2.0" gt-abc bd-xyz --notify gastown/joe

# Track a single issue (still creates batch for dashboard visibility)
gt batch create "Fix auth bug" gt-auth-fix

# With default notification (from config)
gt batch create "Feature X" gt-a gt-b gt-c
```

### Add Issues

```bash
# Add issues to existing batch
gt batch add hq-cv-abc gt-new-issue
gt batch add hq-cv-abc gt-issue1 gt-issue2 gt-issue3

# Adding to closed batch requires reopening first
bd update hq-cv-abc --status=open
gt batch add hq-cv-abc gt-followup-fix
```

### Check Status

```bash
# Show issues and active workers (the swarm)
gt batch status hq-abc

# All active batches (the dashboard)
gt batch status
```

Example output:
```
🚚 hq-cv-abc: Deploy v2.0

  Status:    ●
  Progress:  2/4 completed
  Created:   2025-12-30T10:15:00-08:00

  Tracked Issues:
    ✓ gt-xyz: Update API endpoint [task]
    ✓ bd-abc: Fix validation [bug]
    ○ bd-ghi: Update docs [task]
    ○ gt-jkl: Deploy to prod [task]
```

### List Batches (Dashboard)

```bash
# Active batches (default) - the primary attention view
gt batch list

# All batches including landed
gt batch list --all

# Only landed batches
gt batch list --status=closed

# JSON output
gt batch list --json
```

Example output:
```
Batches

  🚚 hq-cv-w3nm6: Feature X ●
  🚚 hq-cv-abc12: Bug fixes ●

Use 'gt batch status <id>' for detailed view.
```

## Notifications

When a batch lands (all tracked issues closed), subscribers are notified:

```bash
# Explicit subscriber
gt batch create "Feature X" gt-abc --notify gastown/joe

# Multiple subscribers
gt batch create "Feature X" gt-abc --notify coordinator/ --notify --human
```

Notification content:
```
🚚 Batch Landed: Deploy v2.0 (hq-cv-abc)

Issues (3):
  ✓ gt-xyz: Update API endpoint
  ✓ gt-def: Add validation
  ✓ bd-abc: Update docs

Duration: 2h 15m
```

## Create from Epic

Auto-discover tracked issues from an existing epic's children. Useful when
a planning/decomposition tool has already structured work as an epic with
child implementation beads.

```bash
# Auto-discover children from epic
gt batch create --from-epic gt-epic-abc

# Override the batch name (defaults to epic title)
gt batch create --from-epic gt-epic-abc "Custom batch name"

# Combine with other flags
gt batch create --from-epic gt-epic-abc --owned --merge=direct
```

**How it works:**
1. Verifies the given bead is an epic
2. BFS-walks the parent-child hierarchy to find dispatchable descendants
3. Creates a standard batch (`hq-cv-*`) tracking all dispatchable children (task, bug, feature, chore)

Non-dispatchable types (sub-epics, decisions) are recursed into but never
tracked directly. Only leaf work items appear in the batch.

## Auto-Batch on Dispatch

When you dispatch a single issue without an existing batch:

```bash
gt dispatch bd-xyz beads/amber
```

This auto-creates a batch so all work appears in the dashboard:
1. Creates batch: "Work: bd-xyz"
2. Tracks the issue
3. Assigns the worker

Even "swarm of one" gets batch visibility.

## Cross-Project Tracking

Batches live in workspace-level beads (`hq-cv-*` prefix) and can track issues from any project:

```bash
# Track issues from multiple projects
gt batch create "Full-stack feature" \
  gt-frontend-abc \
  gt-backend-def \
  bd-docs-xyz
```

The `tracks` relation is:
- **Non-blocking**: doesn't affect issue workflow
- **Additive**: can add issues anytime
- **Cross-project**: batch in hq-*, issues in gt-*, bd-*, etc.

## Batch vs Project Status

| View | Scope | Shows |
|------|-------|-------|
| `gt batch status [id]` | Cross-project | Issues tracked by batch + workers |
| `gt project status <project>` | Single project | All workers in project + their batch membership |

Use batches for "what's the status of this batch of work?"
Use project status for "what's everyone in this project working on?"

## See Also

- [Propulsion Principle](propulsion-principle.md) - Worker execution model
- [Mail Protocol](../design/mail-protocol.md) - Notification delivery
