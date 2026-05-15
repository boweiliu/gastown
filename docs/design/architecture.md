# Gas Town Architecture

Technical architecture for Gas Town multi-agent workspace management.

## Two-Level Beads Architecture

Gas Town uses a two-level beads architecture to separate organizational coordination
from project implementation work.

| Level | Location | Prefix | Purpose |
|-------|----------|--------|---------|
| **Workspace** | `~/gt/.beads/` | `hq-*` | Cross-project coordination, Coordinator mail, agent identity |
| **Project** | `<project>/coordinator/project/.beads/` | project prefix | Implementation work, MRs, project issues |

### Workspace-Level Beads (`~/gt/.beads/`)

Organizational chain for cross-project coordination:
- Coordinator mail and messages
- Batch coordination (batch work across projects)
- Strategic issues and decisions
- **Workspace-level agent beads** (Coordinator, Supervisor)
- **Role definition beads** (global templates)

### Project-Level Beads (`<project>/coordinator/project/.beads/`)

Project chain for implementation work:
- Bugs, features, tasks for the project
- Merge requests and code reviews
- Project-specific workflows
- **Project-level agent beads** (Watcher, Merger, Workers)

## Agent Bead Storage

Agent beads track lifecycle state for each agent. Storage location depends on
the agent's scope.

| Agent Type | Scope | Bead Location | Bead ID Format |
|------------|-------|---------------|----------------|
| Coordinator | Workspace | `~/gt/.beads/` | `hq-coordinator` |
| Supervisor | Workspace | `~/gt/.beads/` | `hq-supervisor` |
| Boot | Workspace | `~/gt/.beads/` | `hq-boot` |
| Helpers | Workspace | `~/gt/.beads/` | `hq-helper-<name>` |
| Watcher | Project | `<project>/.beads/` | `<prefix>-<project>-watcher` |
| Merger | Project | `<project>/.beads/` | `<prefix>-<project>-merger` |
| Workers | Project | `<project>/.beads/` | `<prefix>-<project>-worker-<name>` |
| Team | Project | `<project>/.beads/` | `<prefix>-<project>-team-<name>` |

### Role Beads

Role beads are global templates stored in workspace beads with `hq-` prefix:
- `hq-coordinator-role` - Coordinator role definition
- `hq-supervisor-role` - Supervisor role definition
- `hq-boot-role` - Boot role definition
- `hq-watcher-role` - Watcher role definition
- `hq-merger-role` - Merger role definition
- `hq-worker-role` - Worker role definition
- `hq-team-role` - Team role definition
- `hq-helper-role` - Helper role definition

Each agent bead references its role bead via the `role_bead` field.

## Agent Taxonomy

### Workspace-Level Agents (Cross-Project)

| Agent | Role | Persistence |
|-------|------|-------------|
| **Coordinator** | Global coordinator, handles cross-project communication and escalations | Persistent |
| **Supervisor** | Daemon beacon — receives heartbeats, runs plugins and monitoring | Persistent |
| **Boot** | Supervisor watchdog — spawned by daemon for triage decisions when Supervisor is down | Ephemeral |
| **Helpers** | Long-running workers for cross-project batch work | Variable |

### Project-Level Agents (Per-Project)

| Agent | Role | Persistence |
|-------|------|-------------|
| **Watcher** | Monitors worker health, handles nudging and cleanup | Persistent |
| **Merger** | Processes merge queue, runs verification | Persistent |
| **Workers** | Workers with persistent identity, assigned to specific issues | Persistent identity, ephemeral sessions |
| **Team** | Human workspaces — full git clones, user-managed lifecycle | Persistent |

## Directory Structure

```
~/gt/                           Workspace root
├── .beads/                     Workspace-level beads (hq-* prefix)
│   ├── metadata.json           Beads config (dolt_mode, dolt_database)
│   └── routes.jsonl            Prefix → project routing table
├── .dolt-data/                 Centralized Dolt data directory
│   ├── hq/                     Workspace beads database (hq-* prefix)
│   ├── gastown/                gastown project database (gt-* prefix)
│   ├── beads/                  Beads project database (bd-* prefix)
│   └── <other projects>/           Per-project databases
├── daemon/                     Daemon runtime state
│   ├── dolt-state.json         Dolt server state (pid, port, databases)
│   ├── dolt-server.log         Server log
│   └── dolt.pid                Server PID file
├── supervisor/                     Supervisor workspace
│   └── helpers/<name>/            Helper worker directories
├── coordinator/                      Coordinator agent home
│   ├── workspace.json               Workspace configuration
│   ├── projects.json               Project registry
│   ├── daemon.json             Daemon sweep config
│   └── accounts.json           Claude Code account management
├── settings/                   Workspace-level settings
│   ├── config.json             Workspace settings (agents, themes)
│   └── escalation.json         Escalation routes and contacts
├── directives/                 Workspace-level role directives (operator policy)
│   └── <role>.md               Markdown injected at prime time
├── template-overlays/           Workspace-level template overlays
│   └── <template>.toml          TOML step overrides (replace/append/skip)
├── config/
│   └── messaging.json          Mail lists, queues, channels
└── <project>/                      Project container (NOT a git clone)
    ├── config.json             Project identity and beads prefix
    ├── directives/             Project-level role directives (overrides workspace)
    │   └── <role>.md
    ├── template-overlays/       Project-level template overlays (full precedence)
    │   └── <template>.toml
    ├── coordinator/project/              Canonical clone (beads live here, NOT an agent)
    │   └── .beads/             Project-level beads (redirected to Dolt)
    ├── merger/               Merger agent home
    │   └── project/                Worktree from coordinator/project
    ├── watcher/                Watcher agent home (no clone)
    ├── team/                   Team parent
    │   └── <name>/             Human workspaces (full clones)
    └── workers/               Workers parent
        └── <name>/<rigname>/   Worker worktrees from coordinator/project
```

**Note**: No per-directory CLAUDE.md or AGENTS.md is created. Only `~/gt/CLAUDE.md`
(workspace-root identity anchor) exists on disk. Full context is injected by `gt prime`
via SessionStart hook.

### Worktree Architecture

Workers and merger are git worktrees, not full clones. This enables fast spawning
and shared object storage. The worktree base is `coordinator/project`:

```go
// From worker/manager.go - worktrees are based on coordinator/project
git worktree add -b worker/<name>-<timestamp> workers/<name>
```

Team workspaces (`team/<name>/`) are full git clones for human developers who need
independent repos. Worker sessions are ephemeral and benefit from worktree efficiency.

## Storage Layer: Dolt SQL Server

All beads data is stored in a single Dolt SQL Server process per workspace. There is
no embedded Dolt fallback — if the server is down, `bd` fails fast with a clear
error pointing to `gt dolt start`.

```
┌─────────────────────────────────┐
│  Dolt SQL Server (per workspace)     │
│  Port 3307, managed by daemon   │
│  Data: ~/gt/.dolt-data/         │
└──────────┬──────────────────────┘
           │ MySQL protocol
    ┌──────┼──────┬──────────┐
    │      │      │          │
  USE hq  USE gastown  USE beads  ...
```

Each project database is a subdirectory under `.dolt-data/`. The daemon monitors
the server on every heartbeat and auto-restarts on crash.

For write concurrency, all agents write directly to `main` using transaction
discipline (`BEGIN` / `DOLT_COMMIT` / `COMMIT` atomically). This eliminates
branch proliferation and ensures immediate cross-agent visibility.

See [dolt-storage.md](dolt-storage.md) for full details.

## Beads Routing

The `routes.jsonl` file maps issue ID prefixes to project locations (relative to workspace root):

```jsonl
{"prefix":"hq-","path":"."}
{"prefix":"gt-","path":"gastown/coordinator/project"}
{"prefix":"bd-","path":"beads/coordinator/project"}
```

Routes point to `coordinator/project` because that's where the canonical `.beads/` lives.
This enables transparent cross-project beads operations:

```bash
bd show hq-coordinator    # Routes to workspace beads (~/.gt/.beads)
bd show gt-xyz      # Routes to gastown/coordinator/project/.beads
```

## Beads Redirects

Worktrees (workers, merger, team) don't have their own beads databases. Instead,
they use a `.beads/redirect` file that points to the canonical beads location:

```
workers/alpha/.beads/redirect → ../../coordinator/project/.beads
merger/project/.beads/redirect   → ../../coordinator/project/.beads
```

`ResolveBeadsDir()` follows redirect chains (max depth 3) with circular detection.
This ensures all agents in a project share a single beads database via the Dolt server.

## Merge Queue: Batch-then-Bisect

The merger processes MRs through a batch-then-bisect merge queue (Bors-style).
This is a core capability, not a pluggable strategy.

### How It Works

```
MRs waiting:  [A, B, C, D]
                    ↓
Batch:        Rebase A..D as a stack on main
                    ↓
Test tip:     Run tests on D (tip of stack)
                    ↓
If PASS:      Fast-forward merge all 4 → done
If FAIL:      Binary bisect → test B (midpoint)
                    ↓
              If B passes: C or D broke it → bisect [C,D]
              If B fails:  A or B broke it → bisect [A,B]
```

### Implementation Phases

| Phase | Bead | What | Status |
|-------|------|------|--------|
| 1: GatesParallel | gt-8b2i | Run test + lint concurrently per MR | In progress |
| 2: Batch-then-bisect | gt-i2vm | Bors-style batching with binary bisect | Blocked by Phase 1 |
| 3: Pre-verification | gt-lu84 | Workers run tests before MR submission | Blocked by Phase 2 |

Gates (test command, lint, etc.) are pluggable. The batching strategy is core.

Design doc: produced by gt-yxx0 review.

## Worker Lifecycle: Self-Managed Completion

Workers manage their own lifecycle end-to-end. The Watcher observes but does NOT
gate completion. This prevents the Watcher from becoming a bottleneck.

### Worker Completion Flow

```
Worker finishes work
  → Push branch to remote
  → Submit MR (bd update --mr-ready)
  → Update bead status
  → Tear down worktree
  → Go idle (available for next assignment)
```

The Watcher monitors for stuck/zombie workers (no activity for extended period)
and messages or escalates. It does NOT process completion — that's the worker's job.

Design bead: gt-0wkk.

## Data Plane Lifecycle

All beads data flows through a six-stage lifecycle managed by Helpers:

```
CREATE → LIVE → CLOSE → DECAY → COMPACT → FLATTEN
  │        │       │        │        │          │
  Dolt   active   done   DELETE   REBASE     SQUASH
  commit  work    bead    rows    commits    all history
                         >7-30d  together   to 1 commit
```

Stages 1-3 are automated today. Stages 4-6 are being shipped via Helper automation
(gt-at0i Reaper DELETE, gt-l8dc Compactor REBASE, gt-emm4 Doctor gc).

See [dolt-storage.md](dolt-storage.md) for full details.

## Deployment Artifacts

Gas Town and Beads are distributed through multiple channels. Tag pushes (`v*`)
trigger GitHub Actions release workflows that build and publish everything.

### Gas Town (`gt`)

| Channel | Artifact | Trigger |
|---------|----------|---------|
| **GitHub Releases** | Platform binaries (darwin/linux/windows, amd64/arm64) + checksums | GoReleaser on tag push |
| **Homebrew** | `brew install steveyegge/gastown/gt` — template auto-updated on release | `update-homebrew` job pushes to `steveyegge/homebrew-gastown` |
| **npm** | `npx @gastown/gt` — wrapper that downloads the correct binary | OIDC trusted publishing (no token) |
| **Local build** | `go build -o $(go env GOPATH)/bin/gt ./cmd/gt` | Manual |

### Beads (`bd`)

| Channel | Artifact | Trigger |
|---------|----------|---------|
| **GitHub Releases** | Platform binaries + checksums | GoReleaser on tag push |
| **Homebrew** | `brew install steveyegge/beads/bd` | `update-homebrew` job |
| **npm** | `npx @beads/bd` — wrapper that downloads the correct binary | OIDC trusted publishing (no token) |
| **PyPI** | `beads-mcp` — MCP server integration | `publish-pypi` job with `PYPI_API_TOKEN` secret |
| **Local build** | `go build -o $(go env GOPATH)/bin/bd ./cmd/bd` | Manual |

### npm Authentication

Both repos use **OIDC trusted publishing** — no `NPM_TOKEN` secret needed.
Authentication is handled by GitHub's OIDC provider. The workflow needs:

```yaml
permissions:
  id-token: write  # Required for npm trusted publishing
```

Configure on npmjs.com: Package Settings → Trusted Publishers → link to the
GitHub repo and `release.yml` workflow file.

### What the binary embeds

The Go binary is the primary distribution vehicle. It embeds:
- **Role templates** — Agent priming context, served by `gt prime`
- **Template definitions** — Workflow workflows, served by `bd workflow`
- **Doctor checks** — Health diagnostics, including migration checks
- **Default configs** — `daemon.json` lifecycle defaults, operational thresholds

This means upgrading the binary automatically propagates most fixes. Files that
are NOT embedded (and require `gt doctor` or `gt upgrade` to update):
- Workspace-root `CLAUDE.md` (created at `gt install` time)
- `daemon.json` sweep entries (created at install, extended by `EnsureLifecycleDefaults`)
- Claude Code hooks (`.claude/settings.json` managed sections)
- Dolt schema (migrations run on first `bd` command after upgrade)

## Role Directives and Template Overlays

Operators can customize agent behavior at the workspace or project level without
modifying the Go binary or embedded templates. This follows the property layer
model (project > workspace > system) and the hooks override precedent.

### Role Directives

Per-role Markdown files injected during `gt prime`, after the role template but
before context files and transfer content. Operator policy that overrides template
instructions where they conflict.

```
~/gt/directives/<role>.md              # Workspace-level (all projects)
~/gt/<project>/directives/<role>.md        # Project-level
```

Both levels concatenate (project content appears last and wins conflicts).
Implemented in `internal/config/directives.go` (`LoadRoleDirective`),
integrated via `outputRoleDirectives()` in `internal/cmd/prime_output.go`.

### Template Overlays

Per-template TOML files that modify individual steps. Applied post-parse before
rendering in `showFormulaStepsFull()`.

```
~/gt/template-overlays/<template>.toml   # Workspace-level
~/gt/<project>/template-overlays/<template>.toml  # Project-level (full precedence)
```

Project-level overlays fully replace workspace-level (not merged). Three override modes:

| Mode | Effect |
|------|--------|
| `replace` | Swap the step description entirely |
| `append` | Add text after the existing step description |
| `skip` | Remove the step (dependents inherit its needs) |

Implemented in `internal/template/overlay.go` (`LoadFormulaOverlay`,
`ApplyOverlays`). `gt doctor` validates overlay step IDs against current
template definitions and can auto-fix stale references.

See [directives-and-overlays.md](directives-and-overlays.md) for the full
reference with examples and design rationale.

## See Also

- [dolt-storage.md](dolt-storage.md) - Dolt storage architecture
- [reference.md](../reference.md) - Command reference
- [directives-and-overlays.md](directives-and-overlays.md) - Directives and overlays reference
- [workflows.md](../concepts/workflows.md) - Workflow workflows
- [identity.md](../concepts/identity.md) - Agent identity and BD_ACTOR
