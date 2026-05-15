# Local Project Bootstrap

For a NightRider-style local setup, prefer a clean bootstrap over `gt project add --adopt`.

`--adopt` is meant for registering an already-assembled project directory. It trusts the
existing shape, which makes it a poor fit for manually assembled local projects where
`.repo.git`, worktrees, and metadata may already be inconsistent.

Use the bootstrap script instead:

```bash
./scripts/bootstrap-local-project.sh \
  --workspace-root /gt \
  --project nightrider_local \
  --local-repo /gt/nightRider \
  --prefix nr \
  --worker-agent claude \
  --watcher-agent codex \
  --merger-agent codex
```

If you omit `--remote`, the script registers the project with `file://<local-repo>`.
That is usually the right choice for local-only or private repos inside the
gastown container, where the upstream remote may not be reachable or authenticated.

What this does:

- Uses `gt project add <name> <git-url> --local-repo <path>` so Gas Town creates a fresh,
  standard project container instead of inheriting a hand-built one.
- Reuses objects from the local repo, so bootstrap stays fast and does not modify the
  source repo.
- Leaves the resulting project with the normal `.repo.git`, `coordinator/project`, `merger/project`,
  `settings/`, and `.beads/` layout that Gas Town expects.
- Optionally pins per-project role agents in `settings/config.json`.

When to still use `--adopt`:

- You already have a real Gas Town project directory that was created elsewhere and you
  only need to register it in a workspace.
