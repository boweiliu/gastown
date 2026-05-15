# worker-pr-flow

A reference harness for projects that gate worker work on a **GitHub PR** rather
than the canonical Merger **merge-queue** flow.

It instructs workers, after their final build/pre-verify passes, to push
their branch and open (or confirm) a GitHub PR before running `gt done`.

## What's in here

| File | Purpose |
|------|---------|
| `worker.md` | Role directive for workers in a PR-flow project. Broad guardrail — applies to any template a worker runs. |
| `wf-worker-work.toml` | Template overlay that appends a PR-creation step to `wf-worker-work`'s `submit-and-exit` step. Surgical — only affects this template. |

Both layers are intentional: the directive sets the project-level expectation
("open PRs, don't merge them yourself"), and the overlay wires the concrete
commands into the workflow a worker actually sees at `gt prime` time.

## Install

```bash
# Role directive (project-scoped)
mkdir -p ~/gt/<project>/directives
cp worker.md ~/gt/<project>/directives/worker.md

# Template overlay (project-scoped)
mkdir -p ~/gt/<project>/template-overlays
cp wf-worker-work.toml ~/gt/<project>/template-overlays/wf-worker-work.toml
```

Replace `<project>` with your project's name (e.g. `gastown`, `longeye`). For
workspace-wide installation, drop the `<project>/` segment — but this is almost
never what you want, since different projects legitimately use different flows.

## Verify it's active

```bash
# Validate overlay step IDs against the current template
gt doctor
# Expect: overlay-health: N overlay(s) healthy

# Inspect the rendered template with the overlay applied
gt template overlay show wf-worker-work --project <project>

# See the directive text that will be injected at prime time
gt directive show worker --project <project>

# End-to-end: see what a worker would see
gt prime --explain
# Expect: "Template overlay: applying 1 override(s) for wf-worker-work (project=<project>)"
```

## What this does / does not do

**Does:**

- Tells the worker to push and open a PR before `gt done`
- Sets a project-level policy that a PR is the review artifact
- Surfaces `gh pr create` failure as an escalation to Watcher, not a silent skip

**Does not:**

- Modify `gt done` behavior (no Go changes)
- Force PR creation via framework-level validation (agents can still misbehave)
- Merge the PR (that's a maintainer / merge-queue concern)
- Replace the directive or overlay if your project also uses other customizations
  — merge this content with your existing files rather than overwriting them

## When to fork this

If your project needs additional PR-flow constraints (required reviewers, specific
labels, CODEOWNERS enforcement, CI checks before `gt done`), copy this harness
and adapt it. The point is a starting template, not a drop-in product.

## Fixing an existing PR (gh#3602)

By default `gt dispatch` creates a fresh `worker/<name>/<bead>@<ts>` branch from
`main`, which means re-dispatching a bead with an existing open PR opens a
**duplicate** PR rather than reusing the original. To resume an existing PR
branch, use `--branch` or `--pr`:

```bash
# Resume by branch name (works for any open or stashed branch)
gt dispatch <bead> <project> --branch worker/example/gh-1234@abcdef

# Resume by PR number (resolves the head ref via `gh pr view`)
gt dispatch <bead> <project> --pr 1234
```

The worker's worktree HEAD will land on the named branch, so its commits
extend the existing PR's history and `gt done` pushes back to the same ref.

**Constraints:**

- `--branch` and `--pr` are mutually exclusive.
- Neither can be combined with `--base-branch` (resume implies its own start point).
- The `--pr` form requires the `gh` CLI to be authenticated against the repo.

The resume branch name is also exposed to templates as the `resume_branch`
variable, alongside the existing `base_branch`, so overlays can react to it.
