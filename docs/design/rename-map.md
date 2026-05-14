# Rename Map: Mad Max → Boring Names

All Gas Town internal concepts get renamed to conventional, self-explanatory names.
"Gas Town" remains as the product name.

## Concept Rename Table

| Old Name | New Name | Scope | Notes |
|----------|----------|-------|-------|
| **Polecat** | **Worker** | Package, struct, CLI, docs | `internal/polecat/` → `internal/worker/` |
| **Convoy** | **Batch** | Package, struct, CLI, docs | `internal/convoy/` → `internal/batch/` |
| **Witness** | **Watcher** | Package, struct, CLI, docs | `internal/witness/` → `internal/watcher/` |
| **Refinery** | **Merger** | Package, struct, CLI, docs | `internal/refinery/` → `internal/merger/` |
| **Deacon** | **Supervisor** | Package, struct, CLI, docs | `internal/deacon/` → `internal/supervisor/` |
| **Dog(s)** | **Helper(s)** | Package, struct, CLI, docs | `internal/dog/` → `internal/helper/` |
| **Rig** | **Project** | Package, struct, CLI, docs | `internal/rig/` → `internal/project/` |
| **Town** | **Workspace** | Struct, CLI, docs | No package rename (used in compound names) |
| **Formula** | **Template** | Package, struct, CLI, docs | `internal/formula/` → `internal/template/` |
| **Molecule** | **Workflow** | Struct, CLI, docs | No separate package (lives in formula/template) |
| **Wisp** | **Ephemeral** | Struct, CLI, docs | Used for ephemeral beads |
| **Wasteland** | **Archive** | Package, struct, CLI, docs | `internal/wasteland/` → `internal/archive/` |
| **Mayor** | **Coordinator** | Package, struct, CLI, docs | `internal/mayor/` → `internal/coordinator/` |
| **Crew** | **Team** | Package, struct, CLI, docs | `internal/crew/` → `internal/team/` |

## Command Rename Table

| Old Name | New Name | Notes |
|----------|----------|-------|
| **Hook** | **Assignment** | `gt hook` → `gt assignment` |
| **Sling** | **Dispatch** | `gt sling` → `gt dispatch` |
| **Nudge** | **Message** | `gt nudge` → `gt message` |
| **Handoff** | **Transfer** | `gt handoff` → `gt transfer` |
| **Seance** | **Recall** | `gt seance` → `gt recall` |
| **Patrol** | **Sweep** | Internal concept (deacon/witness patrol cycles) |
| **Unsling** | **Unassign** | `gt unsling` → `gt unassign` |
| **Mountain** | **Launch** | `gt mountain` → `gt launch` (epic launcher) |
| **Boot** | **Watchdog** | `gt boot` → `gt watchdog` |

## Acronym / Principle Rename Table

| Old Name | New Name | Notes |
|----------|----------|-------|
| **GUPP** | **Auto-Execute Rule** | "If work is assigned, execute it" |
| **MEOW** | **Work Decomposition** | Breaking goals into atomic units |
| **NDI** | **Eventual Completion** | Nondeterministic idempotence → plain name |

## CLI Subcommand Mapping

Full mapping of `gt` subcommands that change:

```
gt convoy        → gt batch
gt sling         → gt dispatch
gt unsling       → gt unassign
gt polecat       → gt worker
gt deacon        → gt supervisor
gt dog           → gt helper
gt witness       → gt watcher
gt refinery      → gt merger
gt hook          → gt assignment
gt nudge         → gt message
gt handoff       → gt transfer
gt seance        → gt recall
gt mol           → gt workflow
gt formula       → gt template
gt wl            → gt archive
gt mountain      → gt launch
gt boot          → gt watchdog
gt mayor         → gt coordinator
gt crew          → gt team
gt orphans       → gt lost-work
gt peek          → gt inspect
gt broadcast     → gt announce
```

Subcommands that stay the same:
```
gt done, gt up, gt down, gt start, gt show, gt close, gt assign,
gt ready, gt commit, gt mail, gt escalate, gt status, gt prime,
gt dolt, gt daemon, gt config, gt session, gt role, gt agents,
gt trail, gt mq, gt bead, gt cat, gt compact, gt changelog,
gt release, gt remember, gt forget, gt memories, gt scheduler,
gt notify, gt dnd, gt estop, gt thaw, gt maintain, gt reaper,
gt quota, gt signal, gt cleanup, gt prune-branches, gt git-init,
gt doctor, gt health, gt resume, gt repair, gt install, gt uninstall,
gt upgrade, gt version, gt help
```

## Runtime Directory Mapping

```
~/gt/                        → ~/workspace/          (or keep ~/gt/)
~/gt/mayor/                  → ~/gt/coordinator/
~/gt/<rig>/polecats/         → ~/gt/<project>/workers/
~/gt/<rig>/polecats/<name>/  → ~/gt/<project>/workers/<name>/
~/gt/<rig>/refinery/         → ~/gt/<project>/merger/
~/gt/<rig>/witness/          → ~/gt/<project>/watcher/
~/gt/<rig>/.beads/           → (unchanged)
```

## Go Package Mapping

```
internal/polecat/    → internal/worker/
internal/convoy/     → internal/batch/
internal/witness/    → internal/watcher/
internal/refinery/   → internal/merger/
internal/deacon/     → internal/supervisor/
internal/dog/        → internal/helper/
internal/rig/        → internal/project/
internal/formula/    → internal/template/
internal/wisp/       → internal/ephemeral/
internal/nudge/      → internal/notification/
internal/wasteland/  → internal/archive/
internal/mayor/      → internal/coordinator/
internal/crew/       → internal/team/
internal/townlog/    → internal/workspacelog/
```

Note: `internal/nudge/` → `internal/notification/` (not `message/`) to avoid
confusion with `internal/mail/` which handles persistent messages.

## Go Type Mapping (key types)

```go
// internal/polecat/ → internal/worker/
type Polecat       → type Worker
type PolecatConfig → type WorkerConfig
type PolecatState  → type WorkerState

// internal/convoy/ → internal/batch/
type Convoy       → type Batch
type ConvoyConfig → type BatchConfig
type ConvoyState  → type BatchState

// internal/witness/ → internal/watcher/
type Witness → type Watcher

// internal/refinery/ → internal/merger/
type Refinery → type Merger

// internal/deacon/ → internal/supervisor/
type Deacon → type Supervisor

// internal/dog/ → internal/helper/
type Dog → type Helper

// internal/formula/ → internal/template/
type Formula       → type Template
type Molecule      → type Workflow
type Protomolecule → type WorkflowTemplate

// internal/wisp/ → internal/ephemeral/
type Wisp → type Ephemeral
```

## Template / Config File Mapping

Files that reference old names in templates/, .beads/formulas/, CLAUDE.md, etc.:

- `templates/polecat-CLAUDE.md` → `templates/worker-CLAUDE.md`
- `templates/witness-CLAUDE.md` → `templates/watcher-CLAUDE.md`
- All formula files referencing `mol-polecat-*` → `wf-worker-*`
- Agent prompt templates referencing polecat/witness/refinery/etc.
- `docs/WASTELAND.md` → `docs/ARCHIVE.md`
- `docs/concepts/polecat-lifecycle.md` → `docs/concepts/worker-lifecycle.md`
- `docs/concepts/convoy.md` → `docs/concepts/batch.md`
- `docs/concepts/propulsion-principle.md` → `docs/concepts/auto-execute-rule.md`
- `docs/concepts/molecules.md` → `docs/concepts/workflows.md`
- `docs/design/polecat-lifecycle-patrol.md` → `docs/design/worker-lifecycle-sweep.md`
- `docs/design/polecat-self-managed-completion.md` → `docs/design/worker-self-managed-completion.md`
- `docs/design/sandboxed-polecat-execution.md` → `docs/design/sandboxed-worker-execution.md`
- `docs/design/persistent-polecat-pool.md` → `docs/design/persistent-worker-pool.md`
- `docs/design/dog-execution-model.md` → `docs/design/helper-execution-model.md`
- `docs/design/dog-infrastructure.md` → `docs/design/helper-infrastructure.md`
- `docs/design/witness-at-team-lead.md` → `docs/design/watcher-at-team-lead.md`
- `docs/design/convoy/` → `docs/design/batch/`

## Phase Execution Order

1. **Documentation** — Rename in .md files only (no code changes)
2. **Go types/vars/comments** — Rename structs, variables, comments (one concept at a time)
3. **Go package directories** — Rename package dirs, update imports (one package at a time)
4. **CLI command surface** — Rename subcommands, add transition aliases
5. **Runtime directory structure** — Rename runtime dirs, add migration
6. **Config, templates, agent prompts** — Update all config/template/prompt files
7. **Remove old-name aliases** — Clean up transition period

## Transition Strategy

- CLI aliases for old names added in Phase 4, removed in Phase 7
- Runtime directory migration script in Phase 5
- All phases must pass `go build ./...` and `go vet ./...`
