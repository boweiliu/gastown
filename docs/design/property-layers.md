# Property Layers: Multi-Level Configuration

> Implementation guide for Gas Town's configuration system.
> Created: 2025-01-06

## Overview

Gas Town uses a layered property system for configuration. Properties are
looked up through multiple layers, with earlier layers overriding later ones.
This enables both local control and global coordination.

## The Four Layers

```
┌─────────────────────────────────────────────────────────────┐
│ 1. EPHEMERAL LAYER (transient, workspace-local)                       │
│    Location: <project>/.beads-ephemeral/config/                      │
│    Synced: Never                                            │
│    Use: Temporary local overrides                           │
└─────────────────────────────┬───────────────────────────────┘
                              │ if missing
                              ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. RIG BEAD LAYER (persistent, synced globally)             │
│    Location: <project>/.beads/ (project identity bead labels)       │
│    Synced: Via git (all clones see it)                      │
│    Use: Project-wide operational state                      │
└─────────────────────────────┬───────────────────────────────┘
                              │ if missing
                              ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. TOWN DEFAULTS                                            │
│    Location: ~/gt/config.json or ~/gt/.beads/               │
│    Synced: N/A (per-workspace)                                   │
│    Use: Workspace-wide policies                                  │
└─────────────────────────────┬───────────────────────────────┘
                              │ if missing
                              ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. SYSTEM DEFAULTS (compiled in)                            │
│    Use: Fallback when nothing else specified                │
└─────────────────────────────────────────────────────────────┘
```

## Lookup Behavior

### Override Semantics (Default)

For most properties, the first non-nil value wins:

```go
func GetConfig(key string) interface{} {
    if val := ephemeral.Get(key); val != nil {
        if val == Blocked { return nil }
        return val
    }
    if val := rigBead.GetLabel(key); val != nil {
        return val
    }
    if val := townDefaults.Get(key); val != nil {
        return val
    }
    return systemDefaults[key]
}
```

### Stacking Semantics (Integers)

For integer properties, values from ephemeral and bead layers **add** to the base:

```go
func GetIntConfig(key string) int {
    base := getBaseDefault(key)    // Workspace or system default
    beadAdj := rigBead.GetInt(key) // 0 if missing
    ephemeralAdj := ephemeral.GetInt(key)    // 0 if missing
    return base + beadAdj + ephemeralAdj
}
```

This enables temporary adjustments without changing the base value.

### Blocking Inheritance

You can explicitly block a property from being inherited:

```bash
gt project config set gastown auto_restart --block
```

This creates a "blocked" marker in the ephemeral layer. Even if the project bead
or defaults say `auto_restart: true`, the lookup returns nil.

## Project Identity Beads

Each project has an identity bead for operational state:

```yaml
id: gt-project-gastown
type: project
name: gastown
repo: git@github.com:steveyegge/gastown.git
prefix: gt

labels:
  - status:operational
  - priority:normal
```

These beads sync via git, so all clones of the project see the same state.

## Two-Level Project Control

### Level 1: Park (Local, Ephemeral)

```bash
gt project park gastown      # Stop services, daemon won't restart
gt project unpark gastown    # Allow services to run
```

- Stored in ephemeral layer (`.beads-ephemeral/config/`)
- Only affects this workspace
- Disappears on cleanup
- Use: Local maintenance, debugging

### Level 2: Dock (Global, Persistent)

```bash
gt project dock gastown      # Set status:docked label on project bead
gt project undock gastown    # Remove label
```

- Stored on project identity bead
- Syncs to all clones via git
- Permanent until explicitly changed
- Use: Project-wide maintenance, coordinated downtime

### Daemon Behavior

The daemon checks both levels before auto-restarting:

```go
func shouldAutoRestart(project *Project) bool {
    status := project.GetConfig("status")
    if status == "parked" || status == "docked" {
        return false
    }
    return true
}
```

## Configuration Keys

| Key | Type | Behavior | Description |
|-----|------|----------|-------------|
| `status` | string | Override | operational/parked/docked |
| `auto_restart` | bool | Override | Daemon auto-restart behavior |
| `max_workers` | int | Override | Maximum concurrent workers |
| `priority_adjustment` | int | **Stack** | Scheduling priority modifier |
| `maintenance_window` | string | Override | When maintenance allowed |
| `dnd` | bool | Override | Do not disturb mode |

## Commands

### View Configuration

```bash
gt project config show gastown           # Show effective config (all layers)
gt project config show gastown --layer   # Show which layer each value comes from
```

### Set Configuration

```bash
# Set in ephemeral layer (local, ephemeral)
gt project config set gastown key value

# Set in bead layer (global, permanent)
gt project config set gastown key value --global

# Block inheritance
gt project config set gastown key --block

# Clear from ephemeral layer
gt project config unset gastown key
```

### Project Lifecycle

```bash
gt project park gastown          # Local: stop + prevent restart
gt project unpark gastown        # Local: allow restart

gt project dock gastown          # Global: mark as offline
gt project undock gastown        # Global: mark as operational

gt project status gastown        # Show current state
```

## Examples

### Temporary Priority Boost

```bash
# Base priority: 0 (from defaults)
# Give this project temporary priority boost for urgent work

gt project config set gastown priority_adjustment 10

# Effective priority: 0 + 10 = 10
# When done, clear it:

gt project config unset gastown priority_adjustment
```

### Local Maintenance

```bash
# I'm upgrading the local clone, don't restart services
gt project park gastown

# ... do maintenance ...

gt project unpark gastown
```

### Project-Wide Maintenance

```bash
# Major refactor in progress, all clones should pause
gt project dock gastown

# Syncs via git - other workspaces see the project as docked
bd sync

# When done:
gt project undock gastown
bd sync
```

### Block Auto-Restart Locally

```bash
# Project bead says auto_restart: true
# But I'm debugging and don't want that here

gt project config set gastown auto_restart --block

# Now auto_restart returns nil for this workspace only
```

## Implementation Notes

### Ephemeral Storage

Ephemeral config stored in `.beads-ephemeral/config/<project>.json`:

```json
{
  "project": "gastown",
  "values": {
    "status": "parked",
    "priority_adjustment": 10
  },
  "blocked": ["auto_restart"]
}
```

### Project Bead Labels

Project operational state stored as labels on the project identity bead:

```bash
bd label add gt-project-gastown status:docked
bd label remove gt-project-gastown status:docked
```

### Daemon Integration

The daemon's lifecycle manager checks config before starting services:

```go
func (d *Daemon) maybeStartRigServices(project string) {
    r := d.getRig(project)

    status := r.GetConfig("status")
    if status == "parked" || status == "docked" {
        log.Info("Project %s is offline, skipping auto-start", project)
        return
    }

    d.ensureWatcher(project)
    d.ensureMerger(project)
}
```

## Operational State Events

Operational state changes are tracked as event beads, providing an immutable audit
trail. Labels cache the current state for fast queries.

### Event Types

| Event Type | Description | Payload |
|------------|-------------|---------|
| `sweep.muted` | Sweep cycle disabled | `{reason, until?}` |
| `sweep.unmuted` | Sweep cycle re-enabled | `{reason?}` |
| `agent.started` | Agent session began | `{session_id?}` |
| `agent.stopped` | Agent session ended | `{reason, outcome?}` |
| `mode.degraded` | System entered degraded mode | `{reason}` |
| `mode.normal` | System returned to normal | `{}` |

### Creating and Querying Events

```bash
# Create operational event
bd create --type=event --event-type=sweep.muted \
  --actor=human:overseer --target=agent:supervisor \
  --payload='{"reason":"fixing batch deadlock","until":"gt-abc1"}'

# Query recent events for an agent
bd list --type=event --target=agent:supervisor --limit=10

# Query current state via labels
bd list --type=role --label=sweep:muted
```

### Labels-as-State Pattern

Events capture the full history. Labels cache the current state:

- `sweep:muted` / `sweep:active`
- `mode:degraded` / `mode:normal`
- `status:idle` / `status:working`

State change flow: create event bead (immutable), then update role bead labels (cache).

```bash
# Mute sweep
bd create --type=event --event-type=sweep.muted ...
bd update role-supervisor --add-label=sweep:muted --remove-label=sweep:active
```

### Configuration vs State

| Type | Storage | Example |
|------|---------|---------|
| **Static config** | TOML files | Daemon tick interval |
| **Role directives** | Markdown files | Operator behavioral policy per role |
| **Template overlays** | TOML files | Per-step template modifications |
| **Operational state** | Beads (events + labels) | Sweep muted |
| **Runtime flags** | Marker files | `.supervisor-disabled` |

*Events are the source of truth. Labels are the cache.*

For Boot triage and degraded mode details, see [Watchdog Chain](watchdog-chain.md).

## Role Directives and Template Overlays

Directives and overlays extend the property layer model to agent behavior.
They follow the same project > workspace > system precedence as other config.

### Directives (Behavioral Policy)

Per-role Markdown files that modify agent behavior at prime time:

```
SYSTEM LAYER:   Embedded role template (compiled in)
                        │ if directive exists
                        ▼
TOWN LAYER:     ~/gt/directives/<role>.md
                        │ concatenated with
                        ▼
RIG LAYER:      ~/gt/<project>/directives/<role>.md
```

Both workspace and project directives concatenate. Project content appears last and wins
conflicts (same as CSS specificity — later rules override earlier ones).

### Overlays (Template Modifications)

Per-template TOML files that modify individual steps:

```
SYSTEM LAYER:   Embedded template (compiled in)
                        │ if overlay exists
                        ▼
TOWN LAYER:     ~/gt/template-overlays/<template>.toml
                        │ project replaces workspace entirely
                        ▼
RIG LAYER:      ~/gt/<project>/template-overlays/<template>.toml
```

Unlike directives, overlays use **full replacement** at the project level — if a
project overlay exists, the workspace overlay is ignored entirely. This prevents
conflicting step modifications from merging unpredictably.

### Precedence Summary

| Config Type | Workspace + Project Interaction | Rationale |
|-------------|----------------------|-----------|
| Project properties | First non-nil wins (override) | Standard config lookup |
| Integer properties | Values stack (additive) | Allows adjustments |
| Role directives | Concatenate (project last) | Additive policy; project gets last word |
| Template overlays | Project replaces workspace | Step mods can conflict; full replacement is safer |

See [directives-and-overlays.md](directives-and-overlays.md) for the full
reference with TOML format, examples, and `gt doctor` integration.

## Related Documents

- `~/gt/docs/hop/PROPERTY-LAYERS.md` - Strategic architecture
- `ephemeral-architecture.md` - Ephemeral system design
- `agent-as-bead.md` - Agent identity beads (similar pattern)
- [directives-and-overlays.md](directives-and-overlays.md) - Full reference
