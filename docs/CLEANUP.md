# gastown/Beads Cleanup Commands Reference

A comprehensive catalog of all cleanup-related commands in the gastown/beads ecosystem, organized by scope and severity.

---

## Process Cleanup

| Command | What it does |
|---------|-------------|
| `gt cleanup` | Kills orphaned Claude processes not tied to active tmux sessions |
| `gt lost-work procs list` | Lists orphaned Claude processes (PPID=1) |
| `gt lost-work procs kill` | Kills orphaned Claude processes (`--aggressive` for tmux-verified) |
| `gt supervisor cleanup-orphans` | Kills orphaned Claude subagent processes (no controlling TTY) |
| `gt supervisor zombie-scan` | Finds/kills zombie Claude processes not in active tmux sessions |

## Worker (Agent Sandbox) Cleanup

| Command | What it does |
|---------|-------------|
| `gt worker remove <project>/<worker>` | Removes worker worktree/directory (fails if session running) |
| `gt worker nuke <project>/<worker>` | Nuclear: kills session, deletes worktree, deletes branch, closes bead |
| `gt worker nuke <project> --all` | Nukes all workers in a project |
| `gt worker gc <project>` | GC stale worker branches (orphaned, old timestamped) |
| `gt worker stale <project>` | Detects stale workers; `--cleanup` auto-nukes them |
| `gt worker check-recovery` | Pre-nuke safety check (SAFE_TO_NUKE vs NEEDS_RECOVERY) |
| `gt worker identity remove <project> <name>` | Removes a worker identity |
| `gt done` | Worker self-cleaning: pushes branch, submits MR (by default), self-nukes worktree, kills own session. MR skipped for `--status ESCALATED\|DEFERRED` or `no_merge` paths |

## Git Artifact Cleanup

| Command | What it does |
|---------|-------------|
| `gt prune-branches` | Removes stale local worker tracking branches (`git fetch --prune` + safe delete) |
| `gt lost-work` | Finds orphaned commits never merged (detection only) |
| `gt lost-work kill` | Prunes orphaned commits (`git gc --prune=now`) + kills orphaned processes |

## Project-Level Cleanup

| Command | What it does |
|---------|-------------|
| `gt project reset` | Resets transfer content, stale mail, orphaned in_progress issues |
| `gt project reset --transfer` | Clears transfer content only |
| `gt project reset --mail` | Clears stale mail only |
| `gt project reset --stale` | Resets orphaned in_progress issues |
| `gt project remove <name>` | Unregisters project from registry, cleans up beads routes |
| `gt project shutdown <project>` | Stops all agents: workers, merger, watcher |
| `gt project stop <project>...` | Stop one or more projects |
| `gt project restart <project>...` | Stop then start (stop phase cleans up) |

## Workspace-Wide Shutdown

| Command | What it does |
|---------|-------------|
| `gt down` | Stops all infrastructure (merger, watcher, coordinator, boot, supervisor, daemon, dolt) |
| `gt down --workers` | Also stops all worker sessions |
| `gt down --all` | Full shutdown with orphan cleanup and verification |
| `gt down --nuke` | Kills entire tmux server (DESTRUCTIVE - kills non-GT sessions too) |
| `gt shutdown` | "Done for the day" - stops agents AND removes worker worktrees/branches. Flags control aggressiveness (`--graceful`, `--force`, `--nuclear`, `--workers-only`, etc.) |

## Team Workspace Cleanup

| Command | What it does |
|---------|-------------|
| `gt team stop [name]` | Stops team tmux sessions |
| `gt team restart [name]` | Kills and restarts team fresh ("clean slate", no transfer mail) |
| `gt team remove <name>` | Removes workspace, closes agent bead |
| `gt team remove <name> --purge` | Full obliteration: deletes agent bead, unassigns beads, clears mail |
| `gt team pristine [name]` | Syncs workspaces with remote (`git pull`) |

## Ephemeral Data / Event Cleanup

| Command | What it does |
|---------|-------------|
| `gt compact` | TTL-based compaction: promotes/deletes ephemerals past their TTL |
| `gt krc prune` | Prunes expired events from the KRC event store |
| `gt krc config reset` | Resets KRC TTL configuration to defaults |
| `gt krc decay` | Shows forensic value decay report (pruning guidance) |

## Dolt Database Cleanup

| Command | What it does |
|---------|-------------|
| `gt dolt cleanup` | Removes orphaned databases from `.dolt-data/` |
| `gt dolt stop` | Stops the Dolt SQL server |
| `gt dolt rollback [backup-dir]` | Restores `.beads` from backup, resets metadata |

## Bead / Hook Cleanup

| Command | What it does |
|---------|-------------|
| `gt close <bead-id>` | Closes beads (lifecycle termination) |
| `gt unassign` / `gt unhook` | Removes work from agent's hook, resets bead status to "open" |
| `gt assignment clear` | Alias for unassign |

## Helper (Infrastructure Worker) Cleanup

| Command | What it does |
|---------|-------------|
| `gt helper remove <name>` | Removes worktrees and helper directory |
| `gt helper remove --all` | Removes all helpers |
| `gt helper clear <name>` | Resets stuck helper to idle state |
| `gt helper done [name]` | Marks helper as done, clears work field |

## Batch Cleanup

| Command | What it does |
|---------|-------------|
| `gt batch close <id>` | Closes a batch bead |
| `gt batch land <id>` | Closes batch, cleans up worker worktrees, sends completion notifications |

## Mail Cleanup

| Command | What it does |
|---------|-------------|
| `gt mail delete <msg-id>` | Deletes specific messages |
| `gt mail archive <msg-id>` | Archives messages (`--stale` for stale ones) |
| `gt mail clear [target]` | Deletes all messages from an inbox (workspace quiescence) |

## Misc State Cleanup

| Command | What it does |
|---------|-------------|
| `gt namepool reset` | Releases all claimed worker names |
| `gt checkpoint clear` | Removes checkpoint file |
| `gt issue clear` | Clears issue from tmux status line |
| `gt doctor --fix` | Auto-fixes: orphan sessions, ephemeral GC, stale redirects, worktree validity |

## System-Level Cleanup

| Command | What it does |
|---------|-------------|
| `gt disable --clean` | Disables gastown + removes shell integration |
| `gt shell remove` | Removes shell integration from RC files |
| `gt config agent remove <name>` | Removes custom agent definition |
| `gt uninstall` | Full removal: shell integration, wrapper scripts, state/config/cache dirs |
| `make clean` | Removes compiled `gt` binary |

## Scripts

| Command | What it does |
|---------|-------------|
| `scripts/migration-test/reset-vm.sh` | Restores VM to pristine v0.5.0 state (test environments) |

## Internal (Automatic / Side-Effect)

| Function | Where | What it does |
|----------|-------|-------------|
| `cleanupOrphanedProcesses()` | `worker.go` | Auto-runs after nuke/stale cleanup |
| `selfNukeWorker()` | `done.go` | Self-destructs worktree during `gt done` |
| `selfKillSession()` | `done.go` | Self-terminates tmux session |
| `rollbackDispatchArtifacts()` | `dispatch.go` | Cleans up partial dispatch failures |
| `cleanStaleHookedBeads()` | `unassign.go` | Repairs beads stuck in "assigned" state |
| `gt signal stop` | `signal_stop.go` | Clears stop-state temp files at turn boundaries |
| `make install` | `Makefile` | Removes stale `~/go/bin/gt` and `~/bin/gt` binaries |

---

## Cleanup Layers (Low to High Severity)

| Layer | Scope | Key Commands |
|-------|-------|-------------|
| **L0** | Ephemeral data | `gt compact`, `gt krc prune` (TTL-based lifecycle) |
| **L1** | Processes | `gt cleanup`, `gt lost-work procs kill`, `gt supervisor cleanup-orphans` |
| **L2** | Git artifacts | `gt prune-branches`, `gt worker gc`, `gt lost-work kill` |
| **L3** | Agents/sessions | `gt worker nuke`, `gt done`, `gt shutdown`, `gt down` |
| **L4** | Workspace | `gt project reset`, `gt doctor --fix`, `gt dolt cleanup` |
| **L5** | System | `gt uninstall`, `gt disable --clean` |

**Total: ~62 commands/functions** across the cleanup ecosystem.
