# Template Resolution Architecture

> **Status: Partially implemented** — Basic template resolution works. Tier enforcement, Mol Mall integration, and HOP federation are planned.

> Where templates live, how they're found, and how they'll scale to Mol Mall

## The Problem

Templates currently exist in multiple locations with no clear precedence:
- `internal/template/templates/` (source of truth, embedded in binary)
- `.beads/templates/` (provisioned at runtime by `gt install`)
- Team directories have their own `.beads/templates/` (diverging copies)

When an agent runs `bd cook wf-worker-work`, which version do they get?

## Design Goals

1. **Predictable resolution** - Clear precedence rules
2. **Local customization** - Override system defaults without forking
3. **Project-specific templates** - Committed workflows for collaborators
4. **Mol Mall ready** - Architecture supports remote template installation
5. **Federation ready** - Templates are shareable across workspaces via HOP (Highway Operations Protocol)

## Three-Tier Resolution

```
┌─────────────────────────────────────────────────────────────────┐
│                     FORMULA RESOLUTION ORDER                     │
│                    (most specific wins)                          │
└─────────────────────────────────────────────────────────────────┘

TIER 1: PROJECT (project-level)
  Location: <project>/.beads/templates/
  Source:   Committed to project repo
  Use case: Project-specific workflows (deploy, test, release)
  Example:  ~/gt/gastown/.beads/templates/wf-gastown-release.template.toml

TIER 2: TOWN (user-level)
  Location: ~/gt/.beads/templates/
  Source:   Mol Mall installs, user customizations
  Use case: Cross-project workflows, personal preferences
  Example:  ~/gt/.beads/templates/wf-worker-work.template.toml (customized)

TIER 3: SYSTEM (embedded)
  Location: Compiled into gt binary
  Source:   internal/template/templates/ at build time
  Use case: Defaults, blessed patterns, fallback
  Example:  wf-worker-work.template.toml (factory default)
```

### Resolution Algorithm

```go
func ResolveFormula(name string, cwd string) (Template, Tier, error) {
    // Tier 1: Project-level (walk up from cwd to find .beads/templates/)
    if projectDir := findProjectRoot(cwd); projectDir != "" {
        path := filepath.Join(projectDir, ".beads", "templates", name+".template.toml")
        if f, err := loadFormula(path); err == nil {
            return f, TierProject, nil
        }
    }

    // Tier 2: Workspace-level
    townDir := getTownRoot() // ~/gt or $GT_HOME
    path := filepath.Join(townDir, ".beads", "templates", name+".template.toml")
    if f, err := loadFormula(path); err == nil {
        return f, TierTown, nil
    }

    // Tier 3: Embedded (system)
    if f, err := loadEmbeddedFormula(name); err == nil {
        return f, TierSystem, nil
    }

    return nil, 0, ErrFormulaNotFound
}
```

### Why This Order

**Project wins** because:
- Project maintainers know their workflows best
- Collaborators get consistent behavior via git
- CI/CD uses the same templates as developers

**Workspace is middle** because:
- User customizations override system defaults
- Mol Mall installs don't require project changes
- Cross-project consistency for the user

**System is fallback** because:
- Always available (compiled in)
- Factory reset target
- The "blessed" versions

## Template Identity

### Current Format

```toml
template = "wf-worker-work"
version = 4
description = "..."
```

### Extended Format (Mol Mall Ready)

```toml
[template]
name = "wf-worker-work"
version = "4.0.0"                          # Semver
author = "steve@gastown.io"                # Author identity
license = "MIT"
repository = "https://github.com/steveyegge/gastown"

[template.registry]
uri = "hop://molmall.gastown.io/templates/wf-worker-work@4.0.0"
checksum = "sha256:abc123..."              # Integrity verification
signed_by = "steve@gastown.io"             # Optional signing

[template.capabilities]
# What capabilities does this template exercise? Used for agent routing.
primary = ["go", "testing", "code-review"]
secondary = ["git", "ci-cd"]
```

### Version Resolution

When multiple versions exist:

```bash
bd cook wf-worker-work          # Resolves per tier order
bd cook wf-worker-work@4        # Specific major version
bd cook wf-worker-work@4.0.0    # Exact version
bd cook wf-worker-work@latest   # Explicit latest
```

## Team Directory Problem

### Current State

Team directories (`gastown/team/max/`) are git worktrees of the rigged repo. They have:
- Their own `.beads/templates/` (from the worktree)
- These can diverge from `coordinator/project/.beads/templates/`

### The Fix

Team should NOT have their own template copies. Options:

**Option A: Symlink/Redirect**
```bash
# team/max/.beads/templates -> ../../coordinator/project/.beads/templates
```
All team share the project's templates.

**Option B: Provision on Demand**
Team directories don't have `.beads/templates/`. Resolution falls through to:
1. Workspace-level (~/gt/.beads/templates/)
2. System (embedded)

**Option C: Gitignore Exclusion**
Exclude `.beads/templates/` from team worktrees via `.gitignore`.

**Recommendation: Option B** - Team shouldn't need project-level templates. They work on the project, they don't define its workflows.

## Commands

### Existing

```bash
bd template list              # Available templates (should show tier)
bd template show <name>       # Template details
bd cook <template>            # Template → Proto
```

### Enhanced

```bash
# List with tier information
bd template list
  wf-worker-work          v4    [project]
  wf-worker-code-review   v1    [workspace]
  wf-watcher-sweep        v2    [system]

# Show resolution path
bd template show wf-worker-work --resolve
  Resolving: wf-worker-work
  ✓ Found at: ~/gt/gastown/.beads/templates/wf-worker-work.template.toml
  Tier: project
  Version: 4

  Resolution path checked:
  1. [project] ~/gt/gastown/.beads/templates/ ← FOUND
  2. [workspace]    ~/gt/.beads/templates/
  3. [system]  <embedded>

# Override tier for testing
bd cook wf-worker-work --tier=system    # Force embedded version
bd cook wf-worker-work --tier=workspace      # Force workspace version
```

### Future (Mol Mall)

```bash
# Install from Mol Mall
gt template install wf-code-review-strict
gt template install wf-code-review-strict@2.0.0
gt template install hop://acme.corp/templates/wf-deploy

# Manage installed templates
gt template list --installed              # What's in workspace-level
gt template upgrade wf-worker-work      # Update to latest
gt template pin wf-worker-work@4.0.0    # Lock version
gt template uninstall wf-code-review-strict
```

## Migration Path

### Phase 1: Resolution Order (Now)

1. Implement three-tier resolution in `bd cook`
2. Add `--resolve` flag to show resolution path
3. Update `bd template list` to show tiers
4. Fix team directories (Option B)

### Phase 2: Workspace-Level Templates

1. Establish `~/gt/.beads/templates/` as workspace template location
2. Add `gt template` commands for managing workspace templates
3. Support manual installation (copy file, track in `.installed.json`)

### Phase 3: Mol Mall Integration

1. Define registry API (see wf-mall-design.md)
2. Implement `gt template install` from remote
3. Add version pinning and upgrade flows
4. Add integrity verification (checksums, optional signing)

### Phase 4: Federation (HOP)

1. Add capability tags to template schema
2. Track template execution for agent accountability
3. Enable federation (cross-workspace template sharing via Highway Operations Protocol)
4. Author attribution and validation records

## Related Documents

- [Mol Mall Design](wf-mall-design.md) - Registry architecture
- [Workflows](../concepts/workflows.md) - Template → Proto → Mol lifecycle
