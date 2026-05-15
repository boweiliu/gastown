# Gas Town Glossary

Gas Town is an agentic development environment for managing multiple Claude Code instances simultaneously using the `gt` and `bd` (Beads) binaries, coordinated with tmux in git-managed directories.

## Core Principles

### Work Decomposition (Molecular Expression of Work)
Breaking large goals into detailed instructions for agents. Supported by Beads, Epics, Templates, and Workflows. Work Decomposition ensures work is decomposed into trackable, atomic units that agents can execute autonomously.

### Auto-Execute Rule (Gas Town Universal Propulsion Principle)
"If there is work on your Hook, YOU MUST RUN IT." This principle ensures agents autonomously proceed with available work without waiting for external input. Auto-Execute Rule is the heartbeat of autonomous operation.

### Eventual Completion (Nondeterministic Idempotence)
The overarching goal ensuring useful outcomes through orchestration of potentially unreliable processes. Persistent Beads and oversight agents (Watcher, Supervisor) guarantee eventual workflow completion even when individual operations may fail or produce varying results.

## Environments

### Workspace
The management headquarters (e.g., `~/gt/`). The Workspace coordinates all workers across multiple Projects and houses workspace-level agents like Coordinator and Supervisor.

### Project
A project-specific Git repository under Gas Town management. Each Project has its own Workers, Merger, Watcher, and Team members. Projects are where actual development work happens.

## Workspace-Level Roles

### Coordinator
Chief-of-staff agent responsible for initiating Batches, coordinating work distribution, and notifying users of important events. The Coordinator operates from the workspace level and has visibility across all Projects.

### Supervisor
Daemon beacon running continuous Sweep cycles. The Supervisor ensures worker activity, monitors system health, and triggers recovery when agents become unresponsive. Think of the Supervisor as the system's watchdog.

### Helpers
The Supervisor's team of maintenance agents handling background tasks like cleanup, health checks, and system maintenance.

### Boot (the Helper)
A special Helper that checks the Supervisor every 5 minutes, ensuring the watchdog itself is still watching. This creates a chain of accountability.

## Project-Level Roles

### Worker
Worker agents with persistent identity but ephemeral sessions. Each worker has a permanent agent bead, CV chain, and work history that accumulates across assignments. Sessions and sandboxes are ephemeral — spawned for specific tasks, cleaned up on completion — but the identity persists. They work in isolated git worktrees to avoid conflicts.

### Merger
Manages the Merge Queue for a Project. The Merger intelligently merges changes from Workers, handling conflicts and ensuring code quality before changes reach the main branch.

### Watcher
Sweep agent that oversees Workers and the Merger within a Project. The Watcher monitors progress, detects stuck agents, and can trigger recovery actions.

### Team
Long-lived, named agents for persistent collaboration. Unlike ephemeral Workers, Team members maintain context across sessions and are ideal for ongoing work relationships.

## Work Units

### Bead
Git-backed atomic work unit stored in Dolt. Beads are the fundamental unit of work tracking in Gas Town. They can represent issues, tasks, epics, or any trackable work item.

### Template
TOML-based workflow source template. Templates define reusable patterns for common operations like sweep cycles, code review, or deployment.

### WorkflowTemplate
A template class for instantiating Workflows. ProtoWorkflows define the structure and steps of a workflow without being tied to specific work items.

### Workflow
Durable chained Bead workflows. Workflows represent multi-step processes where each step is tracked as a Bead. They survive agent restarts and ensure complex workflows complete.

### Ephemeral
Ephemeral Beads destroyed after runs. Ephemerals are lightweight work items used for transient operations that don't need permanent tracking.

### Hook
A special pinned Bead for each agent. The Hook is an agent's primary work queue - when work appears on your Hook, Auto-Execute Rule dictates you must run it.

## Workflow Commands

### Batch
Primary work-order wrapping related Beads. Batches group related tasks together and can be assigned to multiple workers. Created with `gt batch create`.

### dispatching
Assigning work to agents via `gt dispatch`. When you dispatch work to a Worker or Team member, you're putting it on their Hook for execution.

### Nudging
Real-time messaging between agents with `gt message`. Messages allow immediate communication without going through the mail system.

### Transfer
Agent session refresh via `/transfer`. When context gets full or an agent needs a fresh start, transfer transfers work state to a new session.

### Recall
Communicating with previous sessions via `gt recall`. Allows agents to query their predecessors for context and decisions from earlier work.

### Sweep
Ephemeral loop maintaining system heartbeat. Sweep agents (Supervisor, Watcher) continuously cycle through health checks and trigger actions as needed.

---

*This glossary was contributed by [Clay Shirky](https://github.com/cshirky) in [Issue #80](https://github.com/steveyegge/gastown/issues/80).*
