# Agent Identity and Attribution

> Canonical format for agent identity in Gas Town

## Why Identity Matters

When you deploy AI agents at scale, anonymous work creates real problems:

- **Debugging:** "The AI broke it" isn't actionable. *Which* AI?
- **Quality tracking:** You can't improve what you can't measure.
- **Compliance:** Auditors ask "who approved this code?" - you need an answer.
- **Performance management:** Some agents are better than others at certain tasks.

Gas Town solves this with **universal attribution**: every action, every commit,
every bead update is linked to a specific agent identity. This enables work
history tracking, capability-based routing, and objective quality measurement.

## BD_ACTOR Format Convention

The `BD_ACTOR` environment variable identifies agents in slash-separated path format.
This is set automatically when agents are spawned and used for all attribution.

### Format by Role Type

| Role Type | Format | Example |
|-----------|--------|---------|
| **Coordinator** | `coordinator` | `coordinator` |
| **Supervisor** | `supervisor` | `supervisor` |
| **Watcher** | `{project}/watcher` | `gastown/watcher` |
| **Merger** | `{project}/merger` | `gastown/merger` |
| **Team** | `{project}/team/{name}` | `gastown/team/joe` |
| **Worker** | `{project}/workers/{name}` | `gastown/workers/toast` |

### Why Slashes?

The slash format mirrors filesystem paths and enables:
- Hierarchical parsing (extract project, role, name)
- Consistent mail addressing (`gt mail send gastown/watcher`)
- Path-like routing in beads operations
- Visual clarity about agent location

## Attribution Model

Gas Town uses three fields for complete provenance:

### Git Commits

```bash
GIT_AUTHOR_NAME="gastown/team/joe"      # Who did the work (agent)
GIT_AUTHOR_EMAIL="steve@example.com"    # Who owns the work (overseer)
```

Result in git log:
```
abc123 Fix bug (gastown/team/joe <steve@example.com>)
```

**Interpretation**:
- The agent `gastown/team/joe` authored the change
- The work belongs to the workspace owner (`steve@example.com`)
- Both are preserved in git history forever

### Beads Records

```json
{
  "id": "gt-xyz",
  "created_by": "gastown/team/joe",
  "updated_by": "gastown/watcher"
}
```

The `created_by` field is populated from `BD_ACTOR` when creating beads.
The `updated_by` field tracks who last modified the record.

### Event Logging

All events include actor attribution:

```json
{
  "ts": "2025-01-15T10:30:00Z",
  "type": "dispatch",
  "actor": "gastown/team/joe",
  "payload": { "bead": "gt-xyz", "target": "gastown/workers/toast" }
}
```

## Environment Setup

Gas Town uses a centralized `config.AgentEnv()` function to set environment
variables consistently across all agent spawn paths (managers, daemon, boot).

### Example: Worker Environment

```bash
# Set automatically for worker 'toast' in project 'gastown'
export GT_ROLE="worker"
export GT_RIG="gastown"
export GT_worker="toast"
export BD_ACTOR="gastown/workers/toast"
export GIT_AUTHOR_NAME="gastown/workers/toast"
export GT_ROOT="/home/user/gt"
export BEADS_DIR="/home/user/gt/gastown/.beads"
export BEADS_AGENT_NAME="gastown/toast"
```

### Example: Team Environment

```bash
# Set automatically for team member 'joe' in project 'gastown'
export GT_ROLE="team"
export GT_RIG="gastown"
export GT_team="joe"
export BD_ACTOR="gastown/team/joe"
export GIT_AUTHOR_NAME="gastown/team/joe"
export GT_ROOT="/home/user/gt"
export BEADS_DIR="/home/user/gt/gastown/.beads"
export BEADS_AGENT_NAME="gastown/joe"
```

### Manual Override

For local testing or debugging:

```bash
export BD_ACTOR="gastown/team/debug"
bd create --title="Test issue"  # Will show created_by: gastown/team/debug
```

See [reference.md](reference.md#environment-variables) for the complete
environment variable reference.

## Identity Parsing

The format supports programmatic parsing:

```go
// identityToBDActor converts daemon identity to BD_ACTOR format
// Workspace level: coordinator, supervisor
// Project level: {project}/watcher, {project}/merger
// Workers: {project}/team/{name}, {project}/workers/{name}
```

| Input | Parsed Components |
|-------|-------------------|
| `coordinator` | role=coordinator |
| `supervisor` | role=supervisor |
| `gastown/watcher` | project=gastown, role=watcher |
| `gastown/merger` | project=gastown, role=merger |
| `gastown/team/joe` | project=gastown, role=team, name=joe |
| `gastown/workers/toast` | project=gastown, role=worker, name=toast |

## Audit Queries

Attribution enables powerful audit queries:

```bash
# All work by an agent
bd audit --actor=gastown/team/joe

# All work in a project
bd audit --actor=gastown/*

# All worker work
bd audit --actor=*/workers/*

# Git history by agent
git log --author="gastown/team/joe"
```

## Design Principles

1. **Agents are not anonymous** - Every action is attributed
2. **Work is owned, not authored** - Agent creates, overseer owns
3. **Attribution is permanent** - Git commits preserve history
4. **Format is parseable** - Enables programmatic analysis
5. **Consistent across systems** - Same format in git, beads, events

## CV and Skill Accumulation

### Human Identity is Global

The global identifier is your **email** - it's already in every git commit. No separate "entity bead" needed.

```
steve@example.com                ← global identity (from git author)
├── Workspace A (home)                ← workspace
│   ├── gastown/team/joe         ← agent executor
│   └── gastown/workers/toast   ← agent executor
└── Workspace B (work)                ← workspace
    └── acme/workers/nux        ← agent executor
```

### Agent vs Owner

| Field | Scope | Purpose |
|-------|-------|---------|
| `BD_ACTOR` | Local (workspace) | Agent attribution for debugging |
| `GIT_AUTHOR_EMAIL` | Global | Human identity for CV |
| `created_by` | Local | Who created the bead |
| `owner` | Global | Who owns the work |

**Agents execute. Humans own.** The worker name in `completed-by: gastown/workers/toast` is executor attribution. The CV credits the human owner (`steve@example.com`).

### Workers Have Persistent Identities

Workers have **persistent identities but ephemeral sessions**. Like employees who
clock in/out: each work session is fresh (new tmux, new worktree), but the identity
persists across sessions.

- **Identity (persistent)**: Agent bead, CV chain, work history
- **Session (ephemeral)**: Claude instance, context window
- **Sandbox (ephemeral)**: Git worktree, branch

Work credits the worker identity, enabling:
- Performance tracking per worker
- Capability-based routing (send Go work to workers with Go track records)
- Model comparison (A/B test different models via different workers)

See [worker-lifecycle.md](worker-lifecycle.md#worker-identity) for details.

### Skills Are Derived

Your CV emerges from querying work evidence:

```bash
# All work by owner (across all agents)
git log --author="steve@example.com"
bd list --owner=steve@example.com

# Skills derived from evidence
# - .go files touched → Go skill
# - issue tags → domain skills
# - commit patterns → activity types
```

### Multi-Workspace Aggregation

A human with multiple workspaces has one CV:

```bash
# Future: federated CV query
bd cv steve@example.com
# Discovers all workspaces, aggregates work, derives skills
```

See `~/gt/docs/hop/decisions/008-identity-model.md` for architectural rationale.

## Enterprise Use Cases

### Compliance and Audit

```bash
# Who touched this file in the last 90 days?
git log --since="90 days ago" -- path/to/sensitive/file.go

# All changes by a specific agent
bd audit --actor=gastown/workers/toast --since=2025-01-01
```

### Performance Tracking

```bash
# Completion rate by agent
bd stats --group-by=actor

# Average time to completion
bd stats --actor=gastown/workers/* --metric=cycle-time
```

### Model Comparison

When agents use different underlying models, attribution enables A/B comparison:

```bash
# Tag agents by model
# gastown/workers/claude-1 uses Claude
# gastown/workers/gpt-1 uses GPT-4

# Compare quality signals
bd stats --actor=gastown/workers/claude-* --metric=revision-count
bd stats --actor=gastown/workers/gpt-* --metric=revision-count
```

Lower revision counts suggest higher first-pass quality.
