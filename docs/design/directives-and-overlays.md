# Role Directives and Template Overlays

> Operator-customizable agent behavior without modifying the Go binary.

> **Reference examples:** [`docs/contrib-harnesses/`](../contrib-harnesses/)
> contains copy-and-adapt directives and overlays that contributors can drop
> into their own project. See for example `worker-pr-flow/` for a project that gates
> work on GitHub PR review rather than the canonical Merger merge queue.

## Problem

The Work Decomposition stack embeds templates and role templates in the binary — intentionally
centralized for consistency, but leaving no override path. Operators cannot
customize agent behavior at the project or workspace level.

**Concrete failure:** Multiple team members autonomously posted `gh pr review`
comments on GitHub during PR review tasks. The template says "post to GitHub,"
and there was no way for the operator to say "actually, in this project, report
back instead."

## Design: Two Levels

### Level 1: Role Directives

Per-role behavioral boundaries injected at prime time. Operator-authored
Markdown that modifies how agents of a given role behave, regardless of which
template they are running.

**File layout:**

```
~/gt/directives/<role>.md              # Workspace-level (all projects)
~/gt/<project>/directives/<role>.md        # Project-level (wins by appearing last)
```

**Injection point:** After the role template, before context files and transfer
content. Directives carry an authority marker: "Project Policy — overrides template
instructions where they conflict."

**Precedence:** Workspace and project directives **concatenate**. If both exist, the
combined output is `<workspace content>\n<project content>`. The project directive gets the
last word, so it effectively overrides the workspace directive on conflicting
instructions.

**Implementation:**
- Loader: `internal/config/directives.go` → `LoadRoleDirective(role, townRoot, rigName) string`
- Integration: `internal/cmd/prime_output.go` → `outputRoleDirectives(ctx RoleContext)`
- Called in the `gt prime` pipeline after `outputPrimeContext()`

### Level 2: Template Overlays

Per-template, per-step overrides at project or workspace scope. CSS-like step
modifications applied post-parse before rendering at prime time.

**File layout:**

```
~/gt/template-overlays/<template>.toml        # Workspace-level
~/gt/<project>/template-overlays/<template>.toml  # Project-level (full precedence)
```

**Precedence:** Project-level overlays **fully replace** workspace-level overlays (not
merged). If a project overlay exists, the workspace overlay is completely ignored. This
prevents conflicting step modifications from merging unpredictably.

**Implementation:**
- Loader: `internal/template/overlay.go` → `LoadFormulaOverlay(formulaName, townRoot, rigName) (*FormulaOverlay, error)`
- Applier: `internal/template/overlay.go` → `ApplyOverlays(f *Template, overlay *FormulaOverlay) []string`
- Integration: `internal/cmd/prime_workflow.go` → `applyFormulaOverlays()` called in `showFormulaStepsFull()`

## TOML Format (Overlays)

Template overlays use TOML with a `[[step-overrides]]` array:

```toml
[[step-overrides]]
step_id = "submit-review"
mode = "replace"
description = """
Report your review findings back to the conversation instead of posting
to GitHub. Format as a structured summary with grade and findings."""

[[step-overrides]]
step_id = "build"
mode = "append"
description = """
Also run integration tests: npm run test:integration"""

[[step-overrides]]
step_id = "deprecated-step"
mode = "skip"
```

### Override Modes

| Mode | Effect | `description` Required |
|------|--------|----------------------|
| `replace` | Swap the step description entirely | Yes |
| `append` | Add text after the existing step description (newline-separated) | Yes |
| `skip` | Remove the step from the template | No |

### Skip Mode Dependency Handling

When a step is skipped, steps that depended on it inherit its `needs`
(dependencies). This preserves the template DAG integrity. For example, if
step B depends on step A, and step A is skipped, then step B inherits
whatever step A depended on.

### Validation Rules

- `step_id` is required on every override
- `mode` must be one of: `replace`, `append`, `skip`
- Malformed TOML returns an error during load
- Step IDs that don't match any template step generate warnings (stale overrides)

## Directive Format (Markdown)

Role directives are plain Markdown files. There is no special syntax — the
content is injected verbatim into the agent's prime output with an authority
header.

```markdown
## PR Review Policy

Do NOT post review comments directly to GitHub via `gh pr review`.
Instead, report your findings back in the conversation as a structured summary.

## Code Style

Always run `npm run lint --fix` before committing.
Follow existing patterns in the codebase.
```

## CLI Commands

> **Note:** CLI commands are being added in gt-3kg.5. The interface below
> reflects the planned design.

### Directive Commands

```bash
gt directive show <role> [--project <project>]    # Show active directive with source
gt directive edit <role> [--project <project>]    # Open in editor (creates file if needed)
gt directive list                         # List all directive files
```

### Overlay Commands

```bash
gt template overlay show <template> [--project <project>]   # Show active overlay with source
gt template overlay edit <template> [--project <project>]   # Open in editor (creates file if needed)
gt template overlay list                           # List all overlay files
```

The `edit` commands create the directory and file if they don't exist (following
the `gt hooks override` precedent). The `show` commands display the resolved
content with source annotation (workspace vs project).

## gt doctor Integration

The `overlay-health` doctor check validates template overlays:

```bash
gt doctor                    # Runs all checks including overlay health
```

**What it checks:**
- Scans all workspace-level and project-level overlay TOML files
- Parses each overlay and loads the corresponding embedded template
- Validates every `step_id` exists in the current template version
- Reports stale step IDs (template was updated, overlay wasn't)

**Results:**
- **OK:** "N overlay(s) healthy" or "no overlay files found"
- **Warning:** Stale step IDs found (auto-fixable)
- **Error:** Malformed TOML (requires manual fix)

**Auto-fix:**

```bash
gt doctor --fix              # Removes stale step-override entries
```

The fix removes step overrides that reference non-existent step IDs. If all
overrides in a file are stale, the entire file is removed. Malformed TOML
is left untouched.

**Implementation:** `internal/doctor/overlay_health_check.go`

## Worked Example: The PR Review Override

This is the motivating use case that drove the feature.

### The Problem

The `wf-worker-work` template has a step called `submit-review` that tells
workers to post review results to GitHub using `gh pr review --comment`.
In the gastown project, the operator wants workers to report findings back in
conversation instead.

### The Solution

**Step 1: Create a project-level template overlay.**

```bash
mkdir -p ~/gt/gastown/template-overlays
```

Create `~/gt/gastown/template-overlays/wf-worker-work.toml`:

```toml
[[step-overrides]]
step_id = "submit-review"
mode = "replace"
description = """
Report your review findings back to the conversation. Format as:

## Review: <file or component>
**Grade:** A-F
**Findings:**
- CRITICAL: ...
- MAJOR: ...
- MINOR: ...

Do NOT post comments to GitHub via gh pr review."""
```

**Step 2: Verify with gt doctor.**

```bash
gt doctor
# ✓ overlay-health: 1 overlay(s) healthy
```

**Step 3: Test with gt prime.**

```bash
gt prime --explain
# Shows: "Template overlay: applying 1 override(s) for wf-worker-work (project=gastown)"
```

Now any worker in the gastown project running `wf-worker-work` will see the
replacement step instead of the original "post to GitHub" instruction.

### What If the Template Changes?

If a future `gt` release renames `submit-review` to `post-results`, the
overlay's `step_id` becomes stale. On next `gt doctor` run:

```
⚠ overlay-health: stale step IDs in gastown/template-overlays/wf-worker-work.toml:
  - step_id "submit-review" not found in template wf-worker-work
```

Running `gt doctor --fix` removes the stale override. The operator then
creates a new override targeting `post-results`.

## Design Rationale

### Why Two Levels, Not One?

Directives and overlays solve different problems at different granularities:

| Aspect | Directives | Overlays |
|--------|-----------|----------|
| Scope | Entire role behavior | Individual template steps |
| Granularity | Broad policy | Surgical modification |
| Format | Markdown (prose) | TOML (structured) |
| Precedence | Concatenate (additive) | Replace (exclusive) |
| Example | "Never post to GitHub" | "In step X, do Y instead" |

A role directive saying "never post to GitHub" applies everywhere — any template,
any step. An overlay targeting `submit-review` in `wf-worker-work` applies
only to that specific step in that specific template.

Both are needed: directives for broad guardrails, overlays for surgical fixes.

### Why Not Modify Templates Directly?

Templates are embedded in the Go binary. Modifying them requires rebuilding
and redeploying. Directives and overlays are external config files that take
effect immediately on the next `gt prime`.

### Architectural Harmony

- **Fits gt prime pipeline:** Role template → directives → context → transfer → template
- **Follows hooks override precedent:** `~/.gt/hooks-overrides/<target>.json`
- **Extends property layers:** Project > workspace > system precedence
- **ZFC-compliant:** Go transports the content, agents interpret the instructions
- **Only touches gt:** `bd` doesn't render templates, so overlays are gt-only

### Dissonance to Manage

- **Conflicting instructions:** Directive says "don't X", template says "do X" →
  mitigated with clear authority framing at injection ("Project Policy — overrides
  template instructions where they conflict")
- **Unstable step IDs:** Template steps are not a stable API; step IDs can change
  across versions → `gt doctor` warns about stale overlays
- **Discoverability:** `gt prime --explain` shows active directives/overlays
  with source annotations

## File Reference

| File | Purpose |
|------|---------|
| `internal/config/directives.go` | Directive loader (`LoadRoleDirective`) |
| `internal/config/directives_test.go` | Directive tests |
| `internal/template/overlay.go` | Overlay loader and applier |
| `internal/template/overlay_test.go` | Overlay tests |
| `internal/cmd/prime_output.go` | `outputRoleDirectives()` integration |
| `internal/cmd/prime_workflow.go` | `applyFormulaOverlays()` integration |
| `internal/doctor/overlay_health_check.go` | Doctor check and auto-fix |
