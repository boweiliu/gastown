# Gas Town Reference

Technical reference for Gas Town internals. Read the README first.

> For directory structure details, see [architecture.md](design/architecture.md).

## Beads Routing

Gas Town routes beads commands based on issue ID prefix. You don't need to think
about which database to use - just use the issue ID.

```bash
bd show gp-xyz    # Routes to greenplace project's beads
bd show hq-abc    # Routes to workspace-level beads
bd show wyv-123   # Routes to wyvern project's beads
```

**How it works**: Routes are defined in `~/gt/.beads/routes.jsonl`. Each project's
prefix maps to its beads location (the coordinator's clone in that project).

| Prefix | Routes To | Purpose |
|--------|-----------|---------|
| `hq-*` | `~/gt/.beads/` | Coordinator mail, cross-project coordination |
| `gp-*` | `~/gt/greenplace/coordinator/project/.beads/` | Greenplace project issues |
| `wyv-*` | `~/gt/wyvern/coordinator/project/.beads/` | Wyvern project issues |

Debug routing: `BD_DEBUG_ROUTING=1 bd show <id>`

## Configuration

### Project Config (`config.json`)

```json
{
  "type": "project",
  "name": "myproject",
  "git_url": "https://github.com/...",
  "default_branch": "main",
  "beads": { "prefix": "mp" }
}
```

**Project config fields:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `default_branch` | `string` | `"main"` | Default branch for the project. Auto-detected from remote during `gt project add`. Used as the merge target by the Merger and as the base for workers when no integration branch is active. |

### Settings (`settings/config.json`)

```json
{
  "theme": {
    "disabled": false,
    "name": "forest",
    "custom": {
      "bg": "#111111",
      "fg": "#eeeeee"
    },
    "role_themes": {
      "watcher": "rust",
      "merger": "plum",
      "team": "none"
    }
  },
  "merge_queue": {
    "enabled": true,
    "run_tests": true,
    "setup_command": "",
    "typecheck_command": "",
    "lint_command": "",
    "test_command": "",
    "build_command": "",
    "on_conflict": "assign_back",
    "delete_merged_branches": true,
    "retry_flaky_tests": 1,
    "poll_interval": "30s",
    "max_concurrent": 1,
    "integration_branch_worker_enabled": true,
    "integration_branch_merger_enabled": true,
    "integration_branch_template": "integration/{title}",
    "integration_branch_auto_land": false
  }
}
```

**Theme fields:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `disabled` | `bool` | `false` | Disable tmux status/window theming for the project |
| `name` | `string` | auto-assigned by project name | Use a named built-in palette theme |
| `custom.bg` | `string` | unset | Custom tmux background color |
| `custom.fg` | `string` | unset | Custom tmux foreground color |
| `role_themes` | `map[string]string` | unset | Per-role overrides for `watcher`, `merger`, `team`, `worker`; use `"none"` to disable theming for a role |

Theme resolution:
- No `theme` config: auto-assign a built-in palette theme by project name
- `disabled: true`: skip both `status-style` and `window-style`
- `name`: use that built-in theme
- `custom`: use exact `{bg, fg}` colors
- `role_themes`: override role-specific sessions within the project

Workspace-level role defaults live in `coordinator/config.json` under:

```json
{
  "theme": {
    "disabled": false,
    "name": "forest",
    "custom": {
      "bg": "#111111",
      "fg": "#eeeeee"
    },
    "role_defaults": {
      "coordinator": "forest",
      "supervisor": "plum",
      "watcher": "rust",
      "team": "none"
    }
  }
}
```

`role_defaults` supports `coordinator`, `supervisor`, `watcher`, `merger`, `team`, and `worker`.

**Merge queue fields:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | `bool` | `true` | Whether the merge queue is active |
| `run_tests` | `bool` | `true` | Run tests before merging |
| `setup_command` | `string` | `""` | Setup/install command (e.g., `pnpm install`) |
| `typecheck_command` | `string` | `""` | Type check command (e.g., `tsc --noEmit`) |
| `lint_command` | `string` | `""` | Lint command (e.g., `eslint .`) |
| `test_command` | `string` | `""` | Test command to run. Empty = skip. |
| `build_command` | `string` | `""` | Build command (e.g., `go build ./...`) |
| `on_conflict` | `string` | `"assign_back"` | Conflict strategy: `assign_back` or `auto_rebase` |
| `delete_merged_branches` | `bool` | `true` | Delete source branches after merging |
| `retry_flaky_tests` | `int` | `1` | Number of times to retry flaky tests |
| `poll_interval` | `string` | `"30s"` | How often Merger polls for new MRs |
| `max_concurrent` | `int` | `1` | Maximum concurrent merges |
| `integration_branch_worker_enabled` | `*bool` | `true` | Workers auto-source worktrees from integration branches |
| `integration_branch_merger_enabled` | `*bool` | `true` | `gt done` / `gt mq submit` auto-target integration branches |
| `integration_branch_template` | `string` | `"integration/{title}"` | Branch name template (`{title}`, `{epic}`, `{prefix}`, `{user}`) |
| `integration_branch_auto_land` | `*bool` | `false` | Merger sweep auto-lands when all children closed |

See [Integration Branches](concepts/integration-branches.md) for integration branch details.

### Runtime (`.runtime/` - gitignored)

Process state, PIDs, ephemeral data.

### Project-Level Configuration

Projects support layered configuration through:
1. **Ephemeral layer** (`.beads-ephemeral/config/`) - transient, local overrides
2. **Project identity bead labels** - persistent project settings
3. **Workspace defaults** (`~/gt/settings/config.json`)
4. **System defaults** - compiled-in fallbacks

#### Worker Branch Naming

Configure custom branch name templates for workers:

```bash
# Set via ephemeral (transient - for testing)
echo '{"Worker_branch_template": "adam/{year}/{month}/{description}"}' > \
  ~/gt/.beads-ephemeral/config/myrig.json

# Or set via project identity bead labels (persistent)
bd update gt-project-myrig --labels="Worker_branch_template:adam/{year}/{month}/{description}"
```

**Template Variables:**

| Variable | Description | Example |
|----------|-------------|---------|
| `{user}` | From `git config user.name` | `adam` |
| `{year}` | Current year (YY format) | `26` |
| `{month}` | Current month (MM format) | `01` |
| `{name}` | Worker name | `alpha` |
| `{issue}` | Issue ID without prefix | `123` (from `gt-123`) |
| `{description}` | Sanitized issue title | `fix-auth-bug` |
| `{timestamp}` | Unique timestamp | `1ks7f9a` |

**Default Behavior (backward compatible):**

When `Worker_branch_template` is empty or not set:
- With issue: `worker/{name}/{issue}@{timestamp}`
- Without issue: `worker/{name}-{timestamp}`

**Example Configurations:**

```bash
# GitHub enterprise format
"adam/{year}/{month}/{description}"

# Simple feature branches
"feature/{issue}"

# Include worker name for clarity
"work/{name}/{issue}"
```

## Template Format

```toml
template = "name"
type = "workflow"           # workflow | expansion | aspect
version = 1
description = "..."

[vars.feature]
description = "..."
required = true

[[steps]]
id = "step-id"
title = "{{feature}}"
description = "..."
needs = ["other-step"]      # Dependencies
```

**Composition:**

```toml
extends = ["base-template"]

[compose]
aspects = ["cross-cutting"]

[[compose.expand]]
target = "step-id"
with = "macro-template"
```

## Workflow Lifecycle

> For the full lifecycle diagram and detailed command reference, see [concepts/workflows.md](concepts/workflows.md).

**Summary**: Template (TOML) --`bd cook`--> WorkflowTemplate --`bd workflow pour`--> Mol (persistent) or Ephemeral (ephemeral) --`bd squash`--> Digest.

| Operation | bd (data) | gt (agent) |
|-----------|-----------|------------|
| Cook/pour/ephemeral | `bd cook`, `bd workflow pour/ephemeral` | — |
| Squash/burn | `bd workflow squash/burn <id>` | `gt workflow squash/burn` (attached) |
| Navigate | `bd workflow current`, `bd workflow show` | `gt assignment`, `gt workflow current` |
| Attach | — | `gt workflow attach/detach` |

## Agent Lifecycle

### Worker Shutdown

```
1. Work through template checklist (shown inline by gt prime)
2. Submit to merge queue via gt done
3. gt done nukes sandbox and exits
4. Watcher removes worktree + branch
```

### Session Cycling

```
1. Agent notices context filling
2. gt transfer (sends mail to self)
3. Manager kills session
4. Manager starts new session
5. New session reads transfer mail
```

## Environment Variables

Gas Town sets environment variables for each agent session via `config.AgentEnv()`.
These are set in tmux session environment when agents are spawned.

### Core Variables (All Agents)

| Variable | Purpose | Example |
|----------|---------|---------|
| `GT_ROLE` | Agent role type | `coordinator`, `watcher`, `worker`, `team` |
| `GT_ROOT` | Workspace root directory | `/home/user/gt` |
| `BD_ACTOR` | Agent identity for attribution | `gastown/workers/toast` |
| `GIT_AUTHOR_NAME` | Commit attribution (same as BD_ACTOR) | `gastown/workers/toast` |
| `BEADS_DIR` | Beads database location | `/home/user/gt/gastown/.beads` |

### Project-Level Variables

| Variable | Purpose | Roles |
|----------|---------|-------|
| `GT_RIG` | Project name | watcher, merger, worker, team |
| `GT_worker` | Worker worker name | worker only |
| `GT_team` | Team worker name | team only |
| `BEADS_AGENT_NAME` | Agent name for beads operations | worker, team |

### Other Variables

| Variable | Purpose |
|----------|---------|
| `GIT_AUTHOR_EMAIL` | Workspace owner email (from git config) |
| `GT_TOWN_ROOT` | Override workspace root detection (manual use) |
| `CLAUDE_RUNTIME_CONFIG_DIR` | Custom Claude settings directory |

### Environment by Role

| Role | Key Variables |
|------|---------------|
| **Coordinator** | `GT_ROLE=coordinator`, `BD_ACTOR=coordinator` |
| **Supervisor** | `GT_ROLE=supervisor`, `BD_ACTOR=supervisor` |
| **Boot** | `GT_ROLE=supervisor/boot`, `BD_ACTOR=supervisor-boot` |
| **Watcher** | `GT_ROLE=watcher`, `GT_RIG=<project>`, `BD_ACTOR=<project>/watcher` |
| **Merger** | `GT_ROLE=merger`, `GT_RIG=<project>`, `BD_ACTOR=<project>/merger` |
| **Worker** | `GT_ROLE=worker`, `GT_RIG=<project>`, `GT_worker=<name>`, `BD_ACTOR=<project>/workers/<name>` |
| **Team** | `GT_ROLE=team`, `GT_RIG=<project>`, `GT_team=<name>`, `BD_ACTOR=<project>/team/<name>` |

### Doctor Check

The `gt doctor` command verifies that running tmux sessions have correct
environment variables. Mismatches are reported as warnings:

```
⚠ env-vars: Found 3 env var mismatch(es) across 1 session(s)
    hq-coordinator: missing GT_ROOT (expected "/home/user/gt")
```

Fix by restarting sessions: `gt shutdown && gt up`

## Agent Working Directories and Settings

Each agent runs in a specific working directory and has its own Claude settings.
Understanding this hierarchy is essential for proper configuration.

### Working Directories by Role

| Role | Working Directory | Notes |
|------|-------------------|-------|
| **Coordinator** | `~/gt/coordinator/` | Workspace-level coordinator, isolated from projects |
| **Supervisor** | `~/gt/supervisor/` | Background supervisor daemon |
| **Watcher** | `~/gt/<project>/watcher/` | No git clone, monitors workers only |
| **Merger** | `~/gt/<project>/merger/project/` | Worktree on main branch |
| **Team** | `~/gt/<project>/team/<name>/project/` | Persistent human workspace clone |
| **Worker** | `~/gt/<project>/workers/<name>/project/` | Worker worktree (ephemeral sandbox) |

Note: The per-project `<project>/coordinator/project/` directory is NOT a working directory—it's
a git clone that holds the canonical `.beads/` database for that project.

### Settings File Locations

Settings are installed in gastown-managed parent directories and passed to
Claude Code via the `--settings` flag. This keeps customer repos clean:

```
~/gt/
├── coordinator/.claude/settings.json              # Coordinator settings (cwd = settings dir)
├── supervisor/.claude/settings.json             # Supervisor settings (cwd = settings dir)
└── <project>/
    ├── team/.claude/settings.json           # Shared by all team members
    ├── workers/.claude/settings.json       # Shared by all workers
    ├── watcher/.claude/settings.json        # Watcher settings
    └── merger/.claude/settings.json       # Merger settings
```

The `--settings` flag loads these as a separate priority tier that merges
additively with any project-level settings in the customer repo.

### CLAUDE.md

Only `~/gt/CLAUDE.md` exists on disk — a minimal identity anchor that prevents
agents from losing their Gas Town identity after context compaction or new sessions.

Full role context (~300-500 lines per role) is injected ephemerally by `gt prime`
via the SessionStart hook. No per-directory CLAUDE.md or AGENTS.md files are created.

**Why no per-directory files?**
- Claude Code traverses upward from CWD for CLAUDE.md — all agents under `~/gt/` find the workspace-root file
- AGENTS.md (for Codex) uses downward traversal from git root — parent directories are invisible, so per-directory AGENTS.md never worked
- The real context comes from `gt prime`, making on-disk bootstrap pointers redundant

### Customer Repo Files (CLAUDE.md and .claude/)

Gas Town no longer uses git sparse checkout to hide customer repo files. Customer
repositories can have their own `.claude/` directory and `CLAUDE.md` — these are
preserved in all worktrees (team, workers, merger, coordinator/project).

Gas Town's context comes from the workspace-root `CLAUDE.md` identity anchor
(picked up by all agents via Claude Code's upward directory traversal),
`gt prime` via the SessionStart hook, and the customer repo's own `CLAUDE.md`.
These coexist safely because:

- **`--settings` flag provides Gas Town settings** as a separate tier that merges
  additively with customer project settings, so both coexist cleanly
- **`gt prime` injects role context** ephemerally via SessionStart hook, which is
  additive with the customer's `CLAUDE.md` — both are loaded
- Gas Town settings live in parent directories (not in customer repos), so
  customer `.claude/` files are fully preserved

**Doctor check**: `gt doctor` warns if legacy sparse checkout is still configured.
Run `gt doctor --fix` to remove it. Tracked `settings.json` files in worktrees are
recognized as customer project config and are not flagged as stale.

### Settings Inheritance

Claude Code's settings are layered from multiple sources:

1. `.claude/settings.json` in current working directory (customer project)
2. `.claude/settings.json` in parent directories (traversing up)
3. `~/.claude/settings.json` (user global settings)
4. `--settings <path>` flag (loaded as a separate additive tier)

Gas Town uses the `--settings` flag to inject role-specific settings from
gastown-managed parent directories. This merges additively with customer
project settings rather than overriding them.

### Settings Templates

Gas Town uses two settings templates based on role type:

| Type | Roles | Key Difference |
|------|-------|----------------|
| **Interactive** | Coordinator, Team | Mail injected on `UserPromptSubmit` hook |
| **Autonomous** | Worker, Watcher, Merger, Supervisor | Mail injected on `SessionStart` hook |

Autonomous agents may start without user input, so they need mail checked
at session start. Interactive agents wait for user prompts.

### Troubleshooting

| Problem | Solution |
|---------|----------|
| Agent using wrong settings | Check `gt doctor`, verify `.claude/settings.json` in role parent dir |
| Settings not found | Run `gt install` to recreate settings, or `gt doctor --fix` |
| Source repo settings leaking | Run `gt doctor --fix` to remove legacy sparse checkout |
| Coordinator settings affecting workers | Coordinator should run in `coordinator/`, not workspace root |

## CLI Reference

### Workspace Management

```bash
gt install [path]            # Create workspace
gt install --git             # With git init
gt doctor                    # Health check
gt doctor --fix              # Auto-repair
```

### Configuration

```bash
# Agent management
gt config agent list [--json]     # List all agents (built-in + custom)
gt config agent get <name>        # Show agent configuration
gt config agent set <name> <cmd>  # Create or update custom agent
gt config agent remove <name>     # Remove custom agent (built-ins protected)

# Default agent
gt config default-agent [name]    # Get or set workspace default agent
```

**Built-in agents**: `claude`, `gemini`, `codex`, `cursor`, `auggie`, `amp`, `opencode`, `copilot`

> **Note on GitHub Copilot**: The `copilot` preset uses executable lifecycle hooks in
> `.github/hooks/gastown.json` (`sessionStart`, `userPromptSubmitted`, `preToolUse`,
> `sessionEnd`) — the same lifecycle events as Claude Code, in Copilot's JSON format.
> Copilot uses a 5-second ready delay instead of prompt-based detection. Requires a
> Copilot seat and org-level CLI policy enabled.

**Custom agents**: Define per-workspace via CLI or JSON:
```bash
gt config agent set claude-glm "claude-glm --model glm-4"
gt config agent set claude "claude-opus"  # Override built-in
gt config default-agent claude-glm       # Set default
```

**Advanced agent config** (`settings/agents.json`):
```json
{
  "version": 1,
  "agents": {
    "opencode": {
      "command": "opencode",
      "args": [],
      "resume_flag": "--session",
      "resume_style": "flag",
      "non_interactive": {
        "subcommand": "run",
        "output_flag": "--format json"
      }
    }
  }
}
```

**Project-level agents** (`<project>/settings/config.json`):
```json
{
  "type": "project-settings",
  "version": 1,
  "agent": "opencode",
  "agents": {
    "opencode": {
      "command": "opencode",
      "args": ["--session"]
    }
  }
}
```

**ACP-enabled custom agents** (`settings/config.json`):
```json
{
  "type": "workspace-settings",
  "version": 1,
  "default_agent": "opencode-acp-debug",
  "agents": {
    "opencode-acp-debug": {
      "command": "opencode",
      "acp": {
        "command": "acp",
        "args": ["--debug", "--print-logs"]
      }
    }
  }
}
```

The `acp` field configures Agent Communication Protocol support:
- `command`: ACP subcommand (e.g., `"acp"` for `opencode acp`)
- `args`: Additional arguments passed to the ACP subcommand

Custom agents inherit ACP support from their base command's preset. For example,
a custom agent with `"command": "opencode"` automatically inherits ACP support
from the opencode preset. You can override or extend the ACP args by specifying
the `acp` field explicitly.

**Agent resolution order**: project-level → workspace-level → built-in presets.

For OpenCode autonomous mode, set env var in your shell profile:
```bash
export OPENCODE_PERMISSION='{"*":"allow"}'
```

### Project Management

```bash
gt project add <name> <url>
gt project list
gt project remove <name>
```

### Batch Management (Primary Dashboard)

```bash
gt batch list                          # Dashboard of active batches
gt batch status [batch-id]            # Show progress (🚚 hq-cv-*)
gt batch create "name" [issues...]     # Create batch tracking issues
gt batch create "name" gt-a bd-b --notify coordinator/  # With notification
gt batch list --all                    # Include landed batches
gt batch list --status=closed          # Only landed batches
```

Note: "Swarm" is ephemeral (workers on a batch's issues). See [Batches](concepts/batch.md).

### Work Assignment

```bash
# Standard workflow: batch first, then dispatch
gt batch create "Feature X" gt-abc gt-def
gt dispatch gt-abc <project>                    # Assign to worker
gt dispatch gt-abc <project> --agent codex      # Override runtime for this dispatch/spawn
gt dispatch <proto> --on gt-def <project>       # With workflow template

# Quick dispatch (auto-creates batch)
gt dispatch <bead> <project>                    # Auto-batch for dashboard visibility
```

Agent overrides:

- `gt start --agent <alias>` overrides the Coordinator/Supervisor runtime for this launch.
- `gt coordinator start|attach|restart --agent <alias>` and `gt supervisor start|attach|restart --agent <alias>` do the same.
- `gt start team <name> --agent <alias>` and `gt team at <name> --agent <alias>` override the team worker runtime.

### Communication

```bash
gt mail inbox
gt mail read <id>
gt mail send <addr> -s "Subject" -m "Body"
gt mail send --human -s "..."    # To overseer
```

### Escalation

```bash
gt escalate "topic"              # Default: MEDIUM severity
gt escalate -s CRITICAL "msg"    # Urgent, immediate attention
gt escalate -s HIGH "msg"        # Important blocker
gt escalate -s MEDIUM "msg" -m "Details..."
```

See [escalation.md](design/escalation.md) for full protocol.

### Sessions

```bash
gt transfer                   # Request cycle (context-aware)
gt transfer --shutdown        # Terminate (workers)
gt session stop <project>/<agent>
gt inspect <agent>              # Check health
gt message <agent> "message"   # Send message to agent
gt recall                    # List discoverable predecessor sessions
gt recall --talk <id>        # Talk to predecessor (full context)
gt recall --talk <id> -p "Where is X?"  # One-shot question
```

**Session Discovery**: Each session has a startup message that becomes searchable
in Claude's `/resume` picker:

```
[GAS TOWN] recipient <- sender • timestamp • topic[:wf-id]
```

Example: `[GAS TOWN] gastown/team/gus <- human • 2025-12-30T15:42 • restart`

**IMPORTANT**: Always use `gt message` to send messages to Claude sessions.
Never use raw `tmux send-keys` - it doesn't handle Claude's input correctly.
`gt message` uses literal mode + debounce + separate Enter for reliable delivery.

### Emergency

```bash
gt stop --all                # Kill all sessions
gt stop --project <name>         # Kill project sessions
```

### Health Check

```bash
gt supervisor health-check <agent>   # Send health check ping, track response
gt supervisor health-state           # Show health check state for all agents
```

### Merge Queue (MQ)

```bash
gt mq list [project]             # Show the merge queue
gt mq next [project]             # Show highest-priority merge request
gt mq submit                 # Submit current branch to merge queue
gt mq status <id>            # Show detailed merge request status
gt mq retry <id>             # Retry a failed merge request
gt mq reject <id>            # Reject a merge request
```

#### Integration Branch Commands

```bash
gt mq integration create <epic-id>              # Create integration branch
gt mq integration create <epic-id> --branch "feat/{title}"  # Custom template
gt mq integration create <epic-id> --base-branch develop   # Non-main base
gt mq integration status <epic-id>              # Show branch status
gt mq integration status <epic-id> --json       # JSON output
gt mq integration land <epic-id>                # Merge to base branch (default: main)
gt mq integration land <epic-id> --dry-run      # Preview only
gt mq integration land <epic-id> --force        # Land with open MRs
gt mq integration land <epic-id> --skip-tests   # Skip test run
```

See [Integration Branches](concepts/integration-branches.md) for the full workflow.

## Beads Commands (bd)

```bash
bd ready                     # Work with no blockers
bd list --status=open
bd list --status=in_progress
bd show <id>
bd create --title="..." --type=task
bd update <id> --status=in_progress
bd close <id>
bd dep add <child> <parent>  # child depends on parent
```

## Sweep Agents

Supervisor, Watcher, and Merger run continuous sweep loops using ephemerals:

| Agent | Sweep Workflow | Responsibility |
|-------|-----------------|----------------|
| **Supervisor** | `wf-supervisor-sweep` | Agent lifecycle, plugin execution, health checks |
| **Watcher** | `wf-watcher-sweep` | Monitor workers, message stuck workers |
| **Merger** | `wf-merger-sweep` | Process merge queue, review MRs, check integration branches |

```
1. gt sweep new               # Create root-only ephemeral
2. gt prime                    # Shows sweep checklist inline
3. Work through each step
4. gt sweep report --summary "..."  # Close + start next cycle
```

## Plugin Workflows

Plugins are workflows with specific labels:

```json
{
  "id": "wf-security-scan",
  "labels": ["template", "plugin", "watcher", "tier:haiku"]
}
```

Sweep workflows bond plugins dynamically:

```bash
bd workflow bond wf-security-scan $PATROL_ID --var scope="$SCOPE"
```

## Template Invocation Patterns

**CRITICAL**: Different template types require different invocation methods.

### Workflow Templates (sequential steps, single worker)

Examples: `shiny`, `shiny-enterprise`, `wf-worker-work`

```bash
gt dispatch <template> --on <bead-id> <target>
gt dispatch shiny-enterprise --on gt-abc123 gastown
```

### Batch Templates (parallel legs, multiple workers)

Examples: `code-review`

**DO NOT use `gt dispatch` for batch templates!** It fails with "batch type not supported".

```bash
# Correct invocation - use gt template run:
gt template run code-review --pr=123
gt template run code-review --files="src/*.go"

# Dry run to preview:
gt template run code-review --pr=123 --dry-run
```

### Identifying Template Type

```bash
gt template show <name>   # Shows "Type: batch" or "Type: workflow"
bd template list          # Lists templates by type
```

### Why This Matters

- `gt dispatch` attempts to cook+pour the template, which fails for batch type
- `gt template run` handles batch dispatch directly, spawning parallel workers
- Batch templates create multiple workers (one per leg) + synthesis step

## Common Issues

| Problem | Solution |
|---------|----------|
| Agent in wrong directory | Check cwd, `gt doctor` |
| Beads prefix mismatch | Check `bd show` vs project config |
| Worktree conflicts | Check worktree state, `gt doctor` |
| Stuck worker | `gt message`, then `gt inspect` |
| Dirty git state | Commit or discard, then `gt transfer` |

> For architecture details (bare repo pattern, beads as control plane, nondeterministic idempotence), see [architecture.md](design/architecture.md).
