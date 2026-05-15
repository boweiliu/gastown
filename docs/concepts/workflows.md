# Workflows

Workflows are workflow templates that coordinate multi-step work in Gas Town.

## Workflow Lifecycle

```
Template (source TOML) ─── "Ice-9"
    │
    ▼ bd cook
WorkflowTemplate (frozen template) ─── Solid
    │
    ├─▶ bd workflow pour ──▶ Mol (persistent) ─── Liquid
    │
    └─▶ bd workflow ephemeral --root-only ──▶ Root Ephemeral (ephemeral) ─── Vapor
```

**Root-only ephemerals** (default): Template steps are NOT materialized as database rows.
Only a single root ephemeral is created. Agents read steps inline from the embedded
template at prime time. This prevents ephemeral accumulation (~6,000+ rows/day → ~400/day).

**Poured ephemerals** (`pour = true`): Steps ARE materialized as sub-ephemerals with
checkpoint recovery. If a session dies, completed steps remain closed and work
resumes from the last checkpoint. Use pour for expensive, low-frequency workflows
where losing progress would be costly (e.g., release workflows).

## Core Concepts

| Term | Description |
|------|-------------|
| **Template** | Source TOML template defining workflow steps |
| **WorkflowTemplate** | Frozen template ready for instantiation |
| **Workflow** | Active workflow instance (root ephemeral only) |
| **Ephemeral** | Ephemeral workflow for sweeps and worker work (never synced) |
| **Root-only** | Only root ephemeral created; steps read from embedded template |
| **Pour** | Template flag (`pour = true`); steps materialized as sub-ephemerals with checkpoint recovery |

## How Agents See Steps

Agents do NOT use `bd workflow current` or `bd close <step-id>` for template workflows.
Instead, template steps are rendered inline when the agent runs `gt prime`:

```
**Template Checklist** (10 steps from wf-worker-work):

### Step 1: Load context and verify assignment
Initialize your session and understand your assignment...

### Step 2: Set up working branch
Ensure you're on a clean feature branch...
```

The agent works through the checklist and runs `gt done` (workers) or
`gt sweep report` (sweep agents) when complete.

## Workflow Commands

### Beads Operations (bd)

```bash
# Templates
bd template list              # Available templates
bd template show <name>       # Template details
bd cook <template>            # Template → Proto

# Workflows (data operations)
bd workflow list                  # Available protos
bd workflow show <id>             # Proto details
bd workflow ephemeral <proto>          # Create ephemeral (root-only by default)
bd workflow bond <proto> <parent> # Attach to existing mol
```

### Agent Operations (gt)

```bash
# Hook management
gt assignment                    # What's on MY hook?
gt prime                   # Shows inline template checklist
gt workflow attach <bead> <mol>   # Pin workflow to bead
gt workflow detach <bead>         # Unpin workflow from bead

# Sweep lifecycle
gt sweep new              # Create sweep ephemeral and hook it
gt sweep report --summary "..."  # Close current sweep, start next cycle
```

## Worker Workflow

Workers receive work via their hook — a root ephemeral attached to an issue.
They see the template checklist inline when they run `gt prime` and work
through each step in order.

### Worker Workflow Summary

```
1. Spawn with work on hook
2. gt prime               # Shows template checklist inline
3. Work through each step
4. Persist findings: bd update <issue> --notes "..."
5. gt done                # Submit, nuke sandbox, exit
```

### Workflow Types

| Type | Storage | Use Case |
|------|---------|----------|
| **Root-only Ephemeral** (`pour = false`) | `.beads/` (ephemeral) | Worker work, sweeps — high frequency, cheap steps |
| **Poured Ephemeral** (`pour = true`) | `.beads/` (sub-ephemerals) | Releases, long workflows — low frequency, expensive steps |

**Heuristic**: If you would curse losing the progress after a crash, set `pour = true`.
High frequency + cheap steps = inline (default). Low frequency + expensive steps = pour.

## Sweep Workflow

Sweep agents (Supervisor, Watcher, Merger) cycle through sweep templates:

```
1. gt sweep new          # Create root-only sweep ephemeral
2. gt prime               # Shows sweep checklist inline
3. Work through each step
4. gt sweep report --summary "..."  # Close + start next cycle
```

`gt sweep report` atomically closes the current sweep root and spawns
a new one for the next cycle.

## Best Practices

1. **Persist findings early** — `bd update <issue> --notes "..."` before session death
2. **Run `gt done` when complete** — mandatory for workers (pushes, submits to MQ, nukes)
3. **Use `gt sweep report`** — for sweep agents to cycle (replaces squash+new pattern)
4. **File discovered work** — `bd create` for bugs found, don't fix them yourself
