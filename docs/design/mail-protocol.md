# Gas Town Mail Protocol

> Reference for inter-agent mail communication in Gas Town

## Overview

Gas Town agents coordinate via mail messages routed through the beads system.
Mail uses `type=message` beads with routing handled by `gt mail`.

## Message Types

### worker_done

**Route**: Worker → Watcher

**Purpose**: Signal work completion, trigger cleanup flow.

**Subject format**: `worker_done <worker-name>`

**Body format**:
```
Exit: MERGED|ESCALATED|DEFERRED
Issue: <issue-id>
MR: <mr-id>          # if exit=MERGED
Branch: <branch>
```

**Trigger**: `gt done` command generates this automatically.

**Handler**: Watcher creates a cleanup ephemeral for the worker.

### MERGE_READY

**Route**: Watcher → Merger

**Purpose**: Signal a branch is ready for merge queue processing.

**Subject format**: `MERGE_READY <worker-name>`

**Body format**:
```
Branch: <branch>
Issue: <issue-id>
Worker: <worker-name>
Verified: clean git state, issue closed
```

**Trigger**: Watcher sends after verifying worker work is complete.

**Handler**: Merger adds to merge queue, processes when ready.

### MERGED

**Route**: Merger → Watcher

**Purpose**: Confirm branch was merged successfully, safe to nuke worker.

**Subject format**: `MERGED <worker-name>`

**Body format**:
```
Branch: <branch>
Issue: <issue-id>
Worker: <worker-name>
Project: <project>
Target: <target-branch>
Merged-At: <timestamp>
Merge-Commit: <sha>
```

**Trigger**: Merger sends after successful merge to main.

**Handler**: Watcher completes cleanup ephemeral, nukes worker worktree.

### MERGE_FAILED

**Route**: Merger → Watcher

**Purpose**: Notify that merge attempt failed (tests, build, or other non-conflict error).

**Subject format**: `MERGE_FAILED <worker-name>`

**Body format**:
```
Branch: <branch>
Issue: <issue-id>
Worker: <worker-name>
Project: <project>
Target: <target-branch>
Failed-At: <timestamp>
Failure-Type: <tests|build|push|other>
Error: <error-message>
```

**Trigger**: Merger sends when merge fails for non-conflict reasons.

**Handler**: Watcher notifies worker, assigns work back for rework.

### REWORK_REQUEST

**Route**: Merger → Watcher

**Purpose**: Request worker to rebase branch due to merge conflicts.

**Subject format**: `REWORK_REQUEST <worker-name>`

**Body format**:
```
Branch: <branch>
Issue: <issue-id>
Worker: <worker-name>
Project: <project>
Target: <target-branch>
Requested-At: <timestamp>
Conflict-Files: <file1>, <file2>, ...

Please rebase your changes onto <target-branch>:

  git fetch origin
  git rebase origin/<target-branch>
  # Resolve any conflicts
  git push -f

The Merger will retry the merge after rebase is complete.
```

**Trigger**: Merger sends when merge has conflicts with target branch.

**Handler**: Watcher notifies worker with rebase instructions.

### RECOVERED_BEAD

**Route**: Watcher → Supervisor

**Purpose**: Notify Supervisor that a dead worker's abandoned work has been recovered
and needs re-dispatch.

**Subject format**: `RECOVERED_BEAD <bead-id>`

**Body format**:
```
Recovered abandoned bead from dead worker.

Bead: <bead-id>
Worker: <project>/<worker-name>
Previous Status: <assigned|in_progress>

The bead has been reset to open with no assignee.
Please re-dispatch to an available worker.
```

**Trigger**: Watcher detects a zombie worker with work still assigned/in_progress.
The bead is reset to open status and this mail is sent for re-dispatch.

**Handler**: Supervisor runs `gt supervisor redispatch <bead-id>` which:
- Rate-limits re-dispatches (5-minute cooldown per bead)
- Tracks failure count (after 3 failures, escalates to Coordinator)
- Auto-detects target project from bead prefix
- Dispatches the bead to an available worker via `gt dispatch`

### RECOVERY_NEEDED

**Route**: Watcher → Supervisor

**Purpose**: Escalate a dirty worker that has unpushed/uncommitted work needing
manual recovery before cleanup.

**Subject format**: `RECOVERY_NEEDED <project>/<worker-name>`

**Body format**:
```
Worker: <project>/<worker-name>
Cleanup Status: <has_uncommitted|has_stash|has_unpushed>
Branch: <branch>
Issue: <issue-id>
Detected: <timestamp>
```

**Trigger**: Watcher detects zombie worker with dirty git state.

**Handler**: Supervisor coordinates recovery (push branch, save work) before
authorizing cleanup. Only escalates to Coordinator if Supervisor cannot resolve.

### HELP

**Route**: Any → escalation target (usually Coordinator)

**Purpose**: Request intervention for stuck/blocked work.

**Subject format**: `HELP: <brief-description>`

**Body format**:
```
Agent: <agent-id>
Issue: <issue-id>       # if applicable
Problem: <description>
Tried: <what was attempted>
```

**Trigger**: Agent unable to proceed, needs external help.

**Handler**: Escalation target assesses and intervenes.

### TRANSFER

**Route**: Agent → self (or successor)

**Purpose**: Session continuity across context limits/restarts.

**Subject format**: `🤝 TRANSFER: <brief-context>`

**Body format**:
```
attached_workflow: <workflow-id>   # if work in progress
attached_at: <timestamp>

## Context
<freeform notes for successor>

## Status
<where things stand>

## Next
<what successor should do>
```

**Trigger**: `gt transfer` command, or manual send before session end.

**Handler**: Next session reads transfer, continues from context.

## Format Conventions

### Subject Line

- **Type prefix**: Uppercase, identifies message type
- **Colon separator**: After type for structured info
- **Brief context**: Human-readable summary

Examples:
```
worker_done nux
MERGE_READY greenplace/nux
HELP: Worker stuck on test failures
🤝 TRANSFER: Schema work in progress
```

### Body Structure

- **Key-value pairs**: For structured data (one per line)
- **Blank line**: Separates structured data from freeform content
- **Markdown sections**: For freeform content (##, lists, code blocks)

### Addresses

Format: `<project>/<role>` or `<project>/<type>/<name>`

Examples:
```
greenplace/watcher       # Watcher for greenplace project
beads/merger           # Merger for beads project
greenplace/workers/nux  # Specific worker
coordinator/                # Workspace-level Coordinator
supervisor/               # Workspace-level Supervisor
```

## Protocol Flows

### Worker Completion Flow

```
Worker                    Watcher                    Merger
   │                          │                          │
   │ worker_done             │                          │
   │─────────────────────────>│                          │
   │                          │                          │
   │                    (verify clean)                   │
   │                          │                          │
   │                          │ MERGE_READY              │
   │                          │─────────────────────────>│
   │                          │                          │
   │                          │                    (merge attempt)
   │                          │                          │
   │                          │ MERGED (success)         │
   │                          │<─────────────────────────│
   │                          │                          │
   │                    (nuke worker)                   │
   │                          │                          │
```

### Merge Failure Flow

```
                           Watcher                    Merger
                              │                          │
                              │                    (merge fails)
                              │                          │
                              │ MERGE_FAILED             │
   ┌──────────────────────────│<─────────────────────────│
   │                          │                          │
   │ (failure notification)   │                          │
   │<─────────────────────────│                          │
   │                          │                          │
Worker (rework needed)
```

### Rebase Required Flow

```
                           Watcher                    Merger
                              │                          │
                              │                    (conflict detected)
                              │                          │
                              │ REWORK_REQUEST           │
   ┌──────────────────────────│<─────────────────────────│
   │                          │                          │
   │ (rebase instructions)    │                          │
   │<─────────────────────────│                          │
   │                          │                          │
Worker                       │                          │
   │                          │                          │
   │ (rebases, gt done)       │                          │
   │─────────────────────────>│ MERGE_READY              │
   │                          │─────────────────────────>│
   │                          │                    (retry merge)
```

### Abandoned Work Recovery Flow

```
Dead Worker               Watcher                    Supervisor
     │                        │                          │
     │ (session dies)         │                          │
     │                        │                          │
     │                  (detects zombie)                 │
     │                  (bead status=assigned)             │
     │                        │                          │
     │                  resetAbandonedBead()             │
     │                  bd update --status=open          │
     │                        │                          │
     │                        │ RECOVERED_BEAD           │
     │                        │─────────────────────────>│
     │                        │                          │
     │                        │                    gt supervisor redispatch
     │                        │                    gt dispatch <bead> <project>
     │                        │                          │
     │                        │                          ├──> New Worker
     │                        │                          │    (re-dispatched)
```

### Second-Order Monitoring

```
Watcher-1 ──┐
            │ (check agent bead last_activity)
Watcher-2 ──┼────────────────> Supervisor agent bead
            │
Watcher-N ──┘
                                 │
                          (if stale >5min)
                                 │
            ─────────────────────┘
            ALERT to Coordinator (mail only on failure)
```

## Communication Hygiene: Mail vs Message

Agents overuse mail for routine communication, generating permanent beads and
Dolt commits for messages that should be ephemeral. Every `gt mail send` creates
a ephemeral bead in Dolt -- a permanent record with its own commit in the git-like
history. This is a critical pollution source.

### The Two Channels

**`gt message` (ephemeral, preferred for routine comms)**
- Sends a message directly to an agent's tmux session
- No beads created. No Dolt commits. Zero storage cost.
- Message appears as a `<system-reminder>` in the agent's context
- Suitable for: health checks, status requests, simple instructions, "wake up" signals
- Limitation: if the target session is dead, the message is lost

**`gt mail send` (persistent, for structured protocol messages only)**
- Creates a bead (ephemeral) in the Dolt database
- Generates at least one Dolt commit (the write)
- Persists across session restarts -- survives agent death
- Suitable for: TRANSFER context, MERGE_READY/MERGED protocol, escalations, HELP
  requests, anything that MUST survive session death

### The Rule

**Default to `gt message`. Only use `gt mail send` when the message MUST survive
the recipient's session death.**

The litmus test: "If the recipient's session dies and restarts, do they need this
message?" If yes -> mail. If no -> message.

### Role-Specific Guidance

| Role | Mail Budget | When to Mail | When to Message |
|------|-------------|-------------|---------------|
| **Worker** | 0-1 per session | HELP/ESCALATE only (gt escalate preferred) | Everything else |
| **Watcher** | Protocol msgs only | MERGE_READY, RECOVERED_BEAD, RECOVERY_NEEDED, escalations to Coordinator | Worker health checks, status pings, message-and-observe |
| **Merger** | Protocol msgs only | MERGED, MERGE_FAILED, REWORK_REQUEST | Status updates to Watcher |
| **Supervisor** | Escalations only | Escalations to Coordinator, TRANSFER to self | TIMER callbacks, HEALTH_CHECK, lifecycle pokes |
| **Helpers** | Zero | Never (results go to event beads or logs) | Report completion to Supervisor via message |
| **Coordinator** | Strategic only | Cross-project coordination, TRANSFER to self | Instructions to Supervisor/Watcher |

### Why This Matters (The Commit Graph)

Dolt is git under the hood. Every mail creates a Dolt commit. Over a day of
normal operations:
- 4 agents x 15 sweep cycles x 2 mails per cycle = 120 commits just for routine chatter
- These commits live in the git history forever, even after mail rows are deleted
- Rebase can remove them, but prevention is always cheaper than cleanup

### Anti-Patterns

**helper_DONE as mail** -- Helpers should not mail their completion status. Use
`gt message supervisor/ "helper_DONE: plugin-name success"` instead.

**Duplicate escalations** -- Watchers sending 2+ mails about the same issue
minutes apart. Check inbox before sending: if you already sent about this topic,
don't send again.

**TRANSFER for routine cycles** -- Sweep agents (Watcher, Supervisor) doing routine
transfers should use minimal mail. If there's nothing extraordinary, just cycle --
the next session discovers state from beads, not from mail.

**Health check responses via mail** -- When Supervisor sends a health check message, do
NOT respond with mail. The Supervisor tracks health via session status, not mail
responses.

## Implementation

### Sending Mail

```bash
# Basic send
gt mail send <addr> -s "Subject" -m "Body"

# With structured body
gt mail send greenplace/watcher -s "MERGE_READY nux" -m "Branch: feature-xyz
Issue: gp-abc
Worker: nux
Verified: clean"
```

### Receiving Mail

```bash
# Check inbox
gt mail inbox

# Read specific message
gt mail read <msg-id>

# Mark as read
gt mail ack <msg-id>
```

### In Sweep Templates

Templates should:
1. Check inbox at start of each cycle
2. Parse subject prefix to route handling
3. Extract structured data from body
4. Take appropriate action
5. Mark mail as read after processing

## Extensibility

New message types follow the pattern:
1. Define subject prefix (TYPE: or TYPE_SUBTYPE)
2. Document body format (key-value pairs + freeform)
3. Specify route (sender → receiver)
4. Implement handlers in relevant sweep templates

The protocol is intentionally simple - structured enough for parsing,
flexible enough for human debugging.

## Beads-Native Messaging

Beyond direct agent-to-agent mail, the messaging system supports three bead-backed
primitives for group and broadcast communication. All use the `hq-` prefix
(workspace-level entities that span projects).

### Groups (`gt:group`)

Named collections of addresses for mail distribution. Sending to a group
delivers to all members.

**Bead ID format:** `hq-group-<name>`

**Member types:** direct addresses (`gastown/team/max`), wildcard patterns
(`*/watcher`, `gastown/team/*`), special patterns (`@workspace`, `@team`,
`@watchers`), or nested group names.

### Queues (`gt:queue`)

Work queues where each message goes to exactly one claimant (unlike groups).

**Bead ID format:** `hq-q-<name>` (workspace-level) or `gt-q-<name>` (project-level)

Fields: `status` (active/paused/closed), `max_concurrency`, `processing_order`
(fifo/priority), plus count fields (available, processing, completed, failed).

### Channels (`gt:channel`)

Pub/sub broadcast streams with configurable message retention.

**Bead ID format:** `hq-channel-<name>`

Fields: `subscribers`, `status` (active/closed), `retention_count`,
`retention_hours`.

### Group and Channel CLI Commands

```bash
# Groups
gt mail group list
gt mail group show <name>
gt mail group create <name> [members...]
gt mail group add <name> <member>
gt mail group remove <name> <member>
gt mail group delete <name>

# Channels
gt mail channel list
gt mail channel show <name>
gt mail channel create <name> [--retain-count=N] [--retain-hours=N]
gt mail channel delete <name>
```

### Sending to Groups, Queues, and Channels

```bash
gt mail send my-group -s "Subject" -m "Body"           # group (expands to members)
gt mail send queue:my-queue -s "Work item" -m "Details" # queue (single claimant)
gt mail send channel:alerts -s "Alert" -m "Content"     # channel (broadcast)
```

### Address Resolution Order

When sending mail, addresses are resolved in this order:

1. **Explicit prefix** -- `group:`, `queue:`, or `channel:` uses that type directly
2. **Contains `/`** -- Treat as agent address or pattern (direct delivery)
3. **Starts with `@`** -- Special pattern (`@workspace`, `@team`, etc.) or group
4. **Name lookup** -- Search group -> queue -> channel by name

If a name matches multiple types, the resolver returns an error requiring an
explicit prefix.

### Retention Policy

Channels support count-based (`--retain-count=N`) and time-based
(`--retain-hours=N`) retention. Retention is enforced on-write (after posting)
and on-sweep (Supervisor runs `PruneAllChannels()` with a 10% buffer to avoid
thrashing).

## Related Documents

- `docs/agent-as-bead.md` - Agent identity and slots
- `.beads/templates/wf-watcher-sweep.template.toml` - Watcher handling
- `internal/mail/` - Mail routing implementation
- `internal/protocol/` - Protocol handlers for Watcher-Merger communication
