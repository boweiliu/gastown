# Installing Gas Town

Complete setup guide for Gas Town multi-agent orchestrator.

## Prerequisites

### Required

| Tool | Version | Check | Install |
|------|---------|-------|---------|
| **Go** | 1.24+ | `go version` | See [golang.org](https://go.dev/doc/install) |
| **Git** | 2.20+ | `git --version` | See below |
| **Dolt** | >= 1.82.4 | `dolt version` | macOS: `brew install dolt`; other platforms: see [dolthub/dolt](https://github.com/dolthub/dolt?tab=readme-ov-file#installation) |
| **Beads** | >= 0.55.4 | `bd version` | Installed by `brew install gastown`, or from source with `go install github.com/steveyegge/beads/cmd/bd@latest` |

### Optional (for Full Stack Mode)

| Tool | Version | Check | Install |
|------|---------|-------|---------|
| **tmux** | 3.0+ | `tmux -V` | See below |
| **Claude Code** (default) | >= 2.0.20 | `claude --version` | See [claude.ai/claude-code](https://claude.ai/claude-code) |
| **Codex CLI** (optional) | latest | `codex --version` | See [developers.openai.com/codex/cli](https://developers.openai.com/codex/cli) |
| **OpenCode CLI** (optional) | latest | `opencode --version` | See [opencode.ai](https://opencode.ai) |
| **GitHub Copilot CLI** (optional) | latest | `copilot --version` | See [cli.github.com](https://cli.github.com) (requires Copilot seat) |

## Installing Prerequisites

### macOS

```bash
# Install Homebrew if needed
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Required
brew install go git dolt

# Optional (for full stack mode)
brew install tmux
```

### Linux (Debian/Ubuntu)

```bash
# Required
sudo apt update
sudo apt install -y git

# Install Go (apt version may be outdated, use official installer)
wget https://go.dev/dl/go1.24.12.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.24.12.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> ~/.bashrc
source ~/.bashrc

# Install Dolt: see https://github.com/dolthub/dolt?tab=readme-ov-file#installation

# Optional (for full stack mode)
sudo apt install -y tmux
```

### Linux (Fedora/RHEL)

```bash
# Required
sudo dnf install -y git golang
# Install Dolt: see https://github.com/dolthub/dolt?tab=readme-ov-file#installation

# Optional
sudo dnf install -y tmux
```

### Verify Prerequisites

```bash
# Check all prerequisites
go version        # Should show go1.24 or higher
git --version     # Should show 2.20 or higher
dolt version      # Should show 1.82.4 or higher
tmux -V           # (Optional) Should show 3.0 or higher
```

## Installing Gas Town

### Step 1: Install the Binaries

```bash
# Install Gas Town CLI
brew install gastown

# Verify installation
gt version
bd version
dolt version
```

Homebrew installs the runtime dependencies declared by the core template. The
`gastownhall/gastown` tap is reserved for emergency updates. If you build from
source instead, install `dolt` first, install `bd` with Go, and ensure
`$GOPATH/bin` (usually `~/go/bin`) is in your PATH. On macOS, do not install
`gt` with `go install`: unsigned binaries may be killed by the OS. Clone the
repository and use `make` instead.

```bash
brew install dolt
go install github.com/steveyegge/beads/cmd/bd@latest
export PATH="$PATH:$HOME/go/bin"
git clone https://github.com/steveyegge/gastown.git
cd gastown
make build
mv gt "$HOME/go/bin/"
```

### Step 2: Create Your Workspace

```bash
# Create a Gas Town workspace (HQ)
gt install ~/gt --shell

# This creates:
#   ~/gt/
#   ├── CLAUDE.md          # Identity anchor (run gt prime)
#   ├── coordinator/             # Coordinator config and state
#   ├── projects/              # Project containers (initially empty)
#   └── .beads/            # Workspace-level issue tracking
```

### Step 3: Add a Project (Project)

```bash
# Add your first project
gt project add myproject https://github.com/you/repo.git

# This clones the repo and sets up:
#   ~/gt/myproject/
#   ├── .beads/            # Project issue tracking
#   ├── coordinator/project/         # Coordinator's clone (canonical)
#   ├── merger/project/      # Merge queue processor
#   ├── watcher/           # Worker monitor
#   └── workers/          # Worker clones (created on demand)
```

### Step 4: Verify Installation

```bash
cd ~/gt

gt enable              # enable Gas Town system-wide
gt git-init            # initialize a git repo for your HQ
gt up                  # Start all services. Use gt down or gt shutdown for stopping. 

gt doctor              # Run health checks
gt status              # Show workspace status
```

### Step 5: Configure Agents (Optional)

Gas Town supports built-in runtimes (`claude`, `gemini`, `codex`, `cursor`, `auggie`, `amp`, `opencode`, `copilot`) plus custom agent aliases.

```bash
# List available agents
gt config agent list

# Create an alias (aliases can encode model/thinking flags)
gt config agent set codex-low "codex --thinking low"
gt config agent set claude-haiku "claude --model haiku --dangerously-skip-permissions"

# Set the workspace default agent (used when a project doesn't specify one)
gt config default-agent codex-low
```

You can also override the agent per command without changing defaults:

```bash
gt start --agent codex-low
gt dispatch gt-abc12 myproject --agent claude-haiku
```

## Minimal Mode vs Full Stack Mode

Gas Town supports two operational modes:

### Minimal Mode (No Daemon)

Run individual runtime instances manually. Gas Town only tracks state.

```bash
# Create and assign work
gt batch create "Fix bugs" gt-abc12
gt dispatch gt-abc12 myproject

# Run runtime manually
cd ~/gt/myproject/workers/<worker>
claude --resume          # Claude Code
# or: codex              # Codex CLI

# Check progress
gt batch list
```

**When to use**: Testing, simple workflows, or when you prefer manual control.

### Full Stack Mode (With Daemon)

Agents run in tmux sessions. Daemon manages lifecycle automatically.

```bash
# Start the daemon
gt daemon start

# Create and assign work (workers spawn automatically)
gt batch create "Feature X" gt-abc12 gt-def34
gt dispatch gt-abc12 myproject
gt dispatch gt-def34 myproject

# Monitor on dashboard
gt batch list

# Attach to any agent session
gt coordinator attach
gt watcher attach myproject
```

**When to use**: Production workflows with multiple concurrent agents.

### Choosing Roles

Gas Town is modular. Enable only what you need:

| Configuration | Roles | Use Case |
|--------------|-------|----------|
| **Workers only** | Workers | Manual spawning, no monitoring |
| **+ Watcher** | + Monitor | Automatic lifecycle, stuck detection |
| **+ Merger** | + Merge queue | MR review, code integration |
| **+ Coordinator** | + Coordinator | Cross-project coordination |

## Troubleshooting

### `gt: command not found`

Your Go bin directory is not in PATH:

```bash
# Add to your shell config (~/.bashrc, ~/.zshrc)
export PATH="$PATH:$HOME/go/bin"
source ~/.bashrc  # or restart terminal
```

### `bd: command not found`

Beads CLI not installed:

```bash
go install github.com/steveyegge/beads/cmd/bd@latest
```

### `gt doctor` shows errors

Run with `--fix` to auto-repair common issues:

```bash
gt doctor --fix
```

For persistent issues, check specific errors:

```bash
gt doctor --verbose
```

### Daemon not starting

Check if tmux is installed and working:

```bash
tmux -V                    # Should show version
tmux new-session -d -s test && tmux kill-session -t test  # Quick test
```

### Git authentication issues

Ensure SSH keys or credentials are configured:

```bash
# Test SSH access
ssh -T git@github.com

# Or configure credential helper
git config --global credential.helper cache
```

### Beads issues

If experiencing beads problems:

```bash
cd ~/gt/myproject/coordinator/project
bd status                  # Check database health
bd doctor                  # Run beads health check
```

## Updating

To update Gas Town and Beads:

```bash
go install github.com/steveyegge/gastown/cmd/gt@latest
go install github.com/steveyegge/beads/cmd/bd@latest
gt doctor --fix            # Fix any post-update issues
```

## Uninstalling

```bash
# Remove binaries
rm $(which gt) $(which bd)

# Remove workspace (CAUTION: deletes all work)
rm -rf ~/gt
```

## Next Steps

After installation:

1. **Read the README** - Core concepts and workflows
2. **Try a simple workflow** - `bd create "Test task"` then `gt batch create "Test" <bead-id>`
3. **Explore docs** - `docs/reference.md` for command reference
4. **Run doctor regularly** - `gt doctor` catches problems early
5. **Join the Archive** - `gt archive join hop/wl-commons` to browse and claim federated work (see [ARCHIVE.md](ARCHIVE.md))
