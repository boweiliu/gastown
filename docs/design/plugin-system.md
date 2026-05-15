# Plugin System Design

> **Status: Design proposal -- not yet implemented**
>
> Design document for the Gas Town plugin system.
> Written 2026-01-11, team/george session.

## Problem Statement

Gas Town needs extensible, project-specific automation that runs during Supervisor sweep cycles. The immediate use case is rebuilding stale binaries (gt, bd, wv), but the pattern generalizes to any periodic maintenance task.

Current state:
- Plugin infrastructure exists conceptually (sweep step mentions it)
- `~/gt/plugins/` directory exists with README
- No actual plugins in production use
- No formalized execution model

## Design Principles Applied

### Discover, Don't Track
> Reality is truth. State is derived.

Plugin state (last run, run count, results) lives on the ledger as ephemerals, not in shadow state files. Gate evaluation queries the ledger directly.

### ZFC: Zero Framework Cognition
> Agent decides. Go transports.

The Supervisor (agent) evaluates gates and decides whether to dispatch. Go code provides transport (`gt helper dispatch`) but doesn't make decisions.

### Work Decomposition Stack Integration

| Layer | Plugin Analog |
|-------|---------------|
| **M**olecule | `plugin.md` - work template with TOML frontmatter |
| **E**phemeral | Plugin-run ephemerals - high-volume, digestible |
| **O**bservable | Plugin runs appear in `bd activity` feed |
| **W**orkflow | Gate → Dispatch → Execute → Record → Digest |

---

## Architecture

### Plugin Locations

```
~/gt/
├── plugins/                      # Workspace-level plugins (universal)
│   └── README.md
├── gastown/
│   └── plugins/                  # Project-level plugins
│       └── rebuild-gt/
│           └── plugin.md
├── beads/
│   └── plugins/
│       └── rebuild-bd/
│           └── plugin.md
└── wyvern/
    └── plugins/
        └── rebuild-wv/
            └── plugin.md
```

**Workspace-level** (`~/gt/plugins/`): Universal plugins that apply everywhere.
**Project-level** (`<project>/plugins/`): Project-specific plugins.

The Supervisor scans both locations during sweep.

### Execution Model: Helper Dispatch

**Key insight**: Plugin execution should not block Supervisor sweep.

Helpers are reusable workers designed for infrastructure tasks. Plugin execution is dispatched to helpers:

```
Supervisor Sweep                    Helper Worker
─────────────────               ─────────────────
1. Scan plugins
2. Evaluate gates
3. For open gates:
   └─ gt helper dispatch plugin     ──→ 4. Execute plugin
      (non-blocking)                  5. Create result ephemeral
                                      6. Send helper_DONE
4. Continue sweep
   ...
5. Process helper_DONE              ←── (next cycle)
```

Benefits:
- Supervisor stays responsive
- Multiple plugins can run concurrently (different helpers)
- Plugin failures don't stall sweep
- Consistent with Helpers' purpose (infrastructure work)

### State Tracking: Ephemerals on the Ledger

Each plugin run creates a ephemeral:

```bash
bd create --ephemeral-type sweep \
  --labels type:plugin-run,plugin:rebuild-gt,project:gastown,result:success \
  --description "Rebuilt gt: abc123 → def456 (5 commits)" \
  "Plugin: rebuild-gt [success]"
```

**Gate evaluation** queries ephemerals instead of state files:

```bash
# Cooldown check: any runs in last hour?
bd list --ephemeral-type sweep --label plugin:rebuild-gt --created-after 1h -n 1
```

**Derived state** (no state.json needed):

| Query | Command |
|-------|---------|
| Last run time | `bd list --label=plugin:X --limit=1 --json` |
| Run count | `bd list --label=plugin:X --json \| jq length` |
| Last result | Parse `result:` label from latest ephemeral |
| Failure rate | Count `result:failure` vs total |

### Digest Pattern

Like cost digests, plugin ephemerals accumulate and get squashed daily:

```bash
gt plugin digest --yesterday
```

Creates: `Plugin Digest 2026-01-10` bead with summary
Deletes: Individual plugin-run ephemerals from that day

This keeps the ledger clean while preserving audit history.

---

## Plugin Format Specification

### File Structure

```
rebuild-gt/
└── plugin.md      # Definition with TOML frontmatter
```

### plugin.md Format

```markdown
+++
name = "rebuild-gt"
description = "Rebuild stale gt binary from source"
version = 1

[gate]
type = "cooldown"
duration = "1h"

[tracking]
labels = ["plugin:rebuild-gt", "project:gastown", "category:maintenance"]
digest = true

[execution]
timeout = "5m"
notify_on_failure = true
+++

# Rebuild gt Binary

Instructions for the helper worker to execute...
```

### TOML Frontmatter Schema

```toml
# Required
name = "string"           # Unique plugin identifier
description = "string"    # Human-readable description
version = 1               # Schema version (for future evolution)

[gate]
type = "cooldown|cron|condition|event|manual"
# Type-specific fields:
duration = "1h"           # For cooldown
schedule = "0 9 * * *"    # For cron
check = "gt stale -q"     # For condition (exit 0 = run)
on = "startup"            # For event

[tracking]
labels = ["label:value", ...]  # Labels for execution ephemerals
digest = true|false            # Include in daily digest

[execution]
timeout = "5m"            # Max execution time
notify_on_failure = true  # Escalate on failure
severity = "low"          # Escalation severity if failed
```

### Gate Types

| Type | Config | Behavior |
|------|--------|----------|
| `cooldown` | `duration = "1h"` | Query ephemerals, run if none in window |
| `cron` | `schedule = "0 9 * * *"` | Run on cron schedule |
| `condition` | `check = "cmd"` | Run check command, run if exit 0 |
| `event` | `on = "startup"` | Run on Supervisor startup |
| `manual` | (no gate section) | Never auto-run, dispatch explicitly |

### Instructions Section

The markdown body after the frontmatter contains agent-executable instructions. The helper worker reads and executes these steps.

Standard sections:
- **Detection**: Check if action is needed
- **Action**: The actual work
- **Record Result**: Create the execution ephemeral
- **Notification**: On success/failure

---

## New Commands Required

- **`gt stale`** -- Expose binary staleness check (human-readable, `--json`, `--quiet` exit code)
- **`gt helper dispatch --plugin <name>`** -- Dispatch plugin execution to an idle helper (non-blocking)
- **`gt plugin list|show|run|digest|history`** -- Plugin management and execution history

---

## Implementation Plan

### Phase 1: Foundation

1. **`gt stale` command** - Expose CheckStaleBinary() via CLI
2. **Plugin format spec** - Finalize TOML schema
3. **Plugin scanning** - Supervisor scans workspace + project plugin dirs

### Phase 2: Execution

4. **`gt helper dispatch --plugin`** - Formalized helper dispatch
5. **Plugin execution in helpers** - Helper reads plugin.md, executes
6. **Ephemeral creation** - Record results on ledger

### Phase 3: Gates & State

7. **Gate evaluation** - Cooldown via ephemeral query
8. **Other gate types** - Cron, condition, event
9. **Plugin digest** - Daily squash of plugin ephemerals

### Phase 4: Escalation

10. **`gt escalate` command** - Unified escalation API
11. **Escalation routing** - Config-driven multi-channel
12. **Stale escalation sweep** - Check unacknowledged

### Phase 5: First Plugin

13. **`rebuild-gt` plugin** - The actual gastown plugin
14. **Documentation** - So Beads/Wyvern can create theirs

---

## Open Questions

1. **Plugin discovery in multiple clones**: If gastown has team/george, team/max, team/joe - which clone's plugins/ dir is canonical? Probably: scan all, dedupe by name, prefer project-root if exists.

2. **Helper assignment**: Should specific plugins prefer specific helpers? Or any idle helper?

3. **Plugin dependencies**: Can plugins depend on other plugins? Probably not in v1.

4. **Plugin disable/enable**: How to temporarily disable a plugin without deleting it? Label on a plugin bead? `enabled = false` in frontmatter?

---

## References

- PRIMING.md - Core design principles
- wf-supervisor-sweep.template.toml - Sweep step plugin-run
- ~/gt/plugins/README.md - Current plugin stub
