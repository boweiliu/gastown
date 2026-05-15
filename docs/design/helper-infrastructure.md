# Helper Infrastructure: Watchdog Chain & Pool Architecture

> Autonomous health monitoring, recovery, and concurrent shutdown dances in Gas Town.

## Overview

Gas Town uses a three-tier watchdog chain for autonomous health monitoring:

```
Daemon (Go process)          <- Dumb transport, 3-min heartbeat
    |
    +-> Boot (AI agent)       <- Intelligent triage, fresh each tick
            |
            +-> Supervisor (AI agent)  <- Continuous sweep, long-running
                    |
                    +-> Watchers & Mergers  <- Per-project agents
```

**Key insight**: The daemon is mechanical (can't reason), but health decisions need
intelligence (is the agent stuck or just thinking?). Boot bridges this gap.

## Design Rationale: Why Two Agents?

### The Problem

The daemon needs to ensure the Supervisor is healthy, but:

1. **Daemon can't reason** - It's Go code following the ZFC principle (don't reason
   about other agents). It can check "is session alive?" but not "is agent stuck?"

2. **Waking costs context** - Each time you spawn an AI agent, you consume context
   tokens. In idle workspaces, waking Supervisor every 3 minutes wastes resources.

3. **Observation requires intelligence** - Distinguishing "agent composing large
   artifact" from "agent hung on tool prompt" requires reasoning.

### The Solution: Boot as Triage

Boot is a narrow, ephemeral AI agent that:
- Runs fresh each daemon tick (no accumulated context debt)
- Makes a single decision: should Supervisor wake?
- Exits immediately after deciding

This gives us intelligent triage without the cost of keeping a full AI running.

### Why Not Merge Boot into Supervisor?

We could have Supervisor handle its own "should I be awake?" logic, but:

1. **Supervisor can't observe itself** - A hung Supervisor can't detect it's hung
2. **Context accumulation** - Supervisor runs continuously; Boot restarts fresh
3. **Cost in idle workspaces** - Boot only costs tokens when it runs; Supervisor costs
   tokens constantly if kept alive

## Session Ownership

| Agent | Session Name | Location | Lifecycle |
|-------|--------------|----------|-----------|
| Daemon | (Go process) | `~/gt/daemon/` | Persistent, auto-restart |
| Boot | `gt-boot` | `~/gt/supervisor/helpers/boot/` | Ephemeral, fresh each tick |
| Supervisor | `hq-supervisor` | `~/gt/supervisor/` | Long-running, transfer loop |

**Critical**: Boot runs in `gt-boot`, NOT `hq-supervisor`. This prevents Boot
from conflicting with a running Supervisor session.

## Heartbeat Mechanics

### Daemon Heartbeat (3 minutes)

The daemon runs a heartbeat tick every 3 minutes:

```go
func (d *Daemon) heartbeatTick() {
    d.ensureBootRunning()           // 1. Spawn Boot for triage
    d.checkSupervisorHeartbeat()        // 2. Belt-and-suspenders fallback
    d.ensureWatcheresRunning()      // 3. Watcher health (checks tmux directly)
    d.ensureMergersRunning()     // 4. Merger health (checks tmux directly)
    d.processLifecycleRequests()    // 5. Cycle/restart requests
    // Agent state derived from tmux, not recorded in beads (gt-zecmc)
}
```

### Supervisor Heartbeat (continuous)

The Supervisor updates `~/gt/supervisor/heartbeat.json` at the start of each sweep cycle:

```json
{
  "timestamp": "2026-01-02T18:30:00Z",
  "cycle": 42,
  "last_action": "health-scan",
  "healthy_agents": 3,
  "unhealthy_agents": 0
}
```

### Heartbeat Freshness

| Age | State | Boot Action |
|-----|-------|-------------|
| < 5 min | Fresh | Nothing (Supervisor active) |
| 5-15 min | Stale | Message if pending mail |
| > 15 min | Very stale | Wake (Supervisor may be stuck) |

## Boot Decision Matrix

When Boot runs, it observes:
- Is Supervisor session alive?
- How old is Supervisor's heartbeat?
- Is there pending mail for Supervisor?
- What's in Supervisor's tmux pane?

Then decides:

| Condition | Action | Command |
|-----------|--------|---------|
| Session dead | START | Exit; daemon calls `ensureSupervisorRunning()` |
| Heartbeat > 15 min | WAKE | `gt message supervisor "Boot wake: check your inbox"` |
| Heartbeat 5-15 min + mail | MESSAGE | `gt message supervisor "Boot check-in: pending work"` |
| Heartbeat fresh | NOTHING | Exit silently |

## Transfer Flow

### Supervisor Transfer

The Supervisor runs continuous sweep cycles. After N cycles or high context:

```
End of sweep cycle:
    |
    +- Squash ephemeral to digest (ephemeral -> permanent)
    +- Write summary to workflow state
    +- gt transfer -s "Routine cycle" -m "Details"
        |
        +- Creates mail for next session
```

Next daemon tick:
```
Daemon -> ensureSupervisorRunning()
    |
    +- Spawns fresh Supervisor in gt-supervisor
        |
        +- SessionStart hook: gt mail check --inject
            |
            +- Previous transfer mail injected
                |
                +- Supervisor reads and continues
```

### Boot Transfer (Rare)

Boot is ephemeral - it exits after each tick. No persistent transfer needed.

However, Boot uses a marker file to prevent double-spawning:
- Marker: `~/gt/supervisor/helpers/boot/.boot-running` (TTL: 5 minutes)
- Status: `~/gt/supervisor/helpers/boot/.boot-status.json` (last action/result)

If the marker exists and is recent, daemon skips Boot spawn for that tick.

## Degraded Mode

When tmux is unavailable, Gas Town enters degraded mode:

| Capability | Normal | Degraded |
|------------|--------|----------|
| Boot runs | As AI in tmux | As Go code (mechanical) |
| Observe panes | Yes | No |
| Message agents | Yes | No |
| Start agents | tmux sessions | Direct spawn |

Degraded Boot triage is purely mechanical:
- Session dead -> start
- Heartbeat stale -> restart
- No reasoning, just thresholds

## Fallback Chain

Multiple layers ensure recovery:

1. **Boot triage** - Intelligent observation, first line
2. **Daemon checkSupervisorHeartbeat()** - Belt-and-suspenders if Boot fails
3. **Tmux-based discovery** - Daemon checks tmux sessions directly (no bead state)
4. **Human escalation** - Mail to overseer for unrecoverable states

---

## Helper Pool Architecture

Boot needs to run multiple shutdown-dance workflows concurrently when multiple death
warrants are issued. All warrants need concurrent tracking, independent timeouts, and
separate outcomes.

### Design Decision: Lightweight State Machines

The shutdown-dance does NOT need Claude sessions. The dance is a deterministic
state machine:

```
WARRANT -> INTERROGATE -> EVALUATE -> PARDON|EXECUTE
```

Each step is mechanical:
1. Send a tmux message (no LLM needed)
2. Wait for timeout or response (timer)
3. Check tmux output for ALIVE keyword (string match)
4. Repeat or terminate

**Decision**: Helpers are lightweight Go routines, not Claude sessions.

### Architecture Overview

```
+-----------------------------------------------------------------+
|                             BOOT                                |
|                     (Claude session in tmux)                    |
|                                                                 |
|  +-----------------------------------------------------------+ |
|  |                      Helper Manager                           | |
|  |                                                            | |
|  |   Pool: [Dog1, Dog2, Dog3, ...]  (goroutines + state)     | |
|  |                                                            | |
|  |   allocate() -> Helper                                        | |
|  |   release(Helper)                                             | |
|  |   status() -> []DogStatus                                  | |
|  +-----------------------------------------------------------+ |
|                                                                 |
|  Boot's job:                                                    |
|  - Watch for warrants (file or event)                           |
|  - Allocate helper from pool                                       |
|  - Monitor helper progress                                         |
|  - Handle helper completion/failure                                |
|  - Report results                                               |
+-----------------------------------------------------------------+
```

### Helper Structure

```go
// Helper represents a shutdown-dance executor
type Helper struct {
    ID        string            // Unique ID (e.g., "helper-1704567890123")
    Warrant   *Warrant          // The death warrant being processed
    State     ShutdownDanceState
    Attempt   int               // Current interrogation attempt (1-3)
    StartedAt time.Time
    StateFile string            // Persistent state: ~/gt/supervisor/helpers/active/<id>.json
}

type ShutdownDanceState string

const (
    StateIdle          ShutdownDanceState = "idle"
    StateInterrogating ShutdownDanceState = "interrogating"  // Sent message, waiting
    StateEvaluating    ShutdownDanceState = "evaluating"     // Checking response
    StatePardoned      ShutdownDanceState = "pardoned"       // Session responded
    StateExecuting     ShutdownDanceState = "executing"      // Killing session
    StateComplete      ShutdownDanceState = "complete"       // Done, ready for cleanup
    StateFailed        ShutdownDanceState = "failed"         // Helper crashed/errored
)

type Warrant struct {
    ID        string    // Bead ID for the warrant
    Target    string    // Session to interrogate (e.g., "gt-gastown-Toast")
    Reason    string    // Why warrant was issued
    Requester string    // Who filed the warrant
    FiledAt   time.Time
}
```

### Pool Design

**Decision**: Fixed pool of 5 helpers, configurable via environment (`GT_helper_POOL_SIZE`).

Rationale:
- Dynamic sizing adds complexity without clear benefit
- 5 concurrent shutdown dances handles worst-case scenarios
- If pool exhausted, warrants queue (better than infinite helper spawning)
- Memory footprint is negligible (goroutines + small state files)

```go
const (
    DefaultPoolSize = 5
    MaxPoolSize     = 20
)

type DogPool struct {
    mu       sync.Mutex
    helpers     []*Helper           // All helpers in pool
    idle     chan *Helper        // Channel of available helpers
    active   map[string]*Helper  // ID -> Helper for active helpers
    stateDir string           // ~/gt/supervisor/helpers/active/
}
```

### Shutdown Dance State Machine

```
                    +------------------------------------------+
                    |                                          |
                    v                                          |
    +----------------------------+                            |
    |     INTERROGATING          |                            |
    |                            |                            |
    |  1. Send health check      |                            |
    |  2. Start timeout timer    |                            |
    +-------------+--------------+                            |
                  |                                            |
                  | timeout or response                        |
                  v                                            |
    +----------------------------+                            |
    |      EVALUATING            |                            |
    |                            |                            |
    |  Check tmux output for     |                            |
    |  ALIVE keyword             |                            |
    +-------------+--------------+                            |
                  |                                            |
          +-------+-------+                                   |
          |               |                                   |
          v               v                                   |
     [ALIVE found]   [No ALIVE]                              |
          |               |                                   |
          |               | attempt < 3?                      |
          |               +-----------------------------------+
          |               | yes: attempt++, longer timeout
          |               |
          |               | no: attempt == 3
          v               v
      +---------+    +-----------+
      | PARDONED|    | EXECUTING |
      |         |    |           |
      | Cancel  |    | Kill tmux |
      | warrant |    | session   |
      +----+----+    +-----+-----+
           |               |
           +-------+-------+
                   |
                   v
          +----------------+
          |    COMPLETE    |
          |                |
          |  Write result  |
          |  Release helper   |
          +----------------+
```

### Timeout Gates

| Attempt | Timeout | Cumulative Wait |
|---------|---------|-----------------|
| 1       | 60s     | 60s             |
| 2       | 120s    | 180s (3 min)    |
| 3       | 240s    | 420s (7 min)    |

### Health Check Message

```
[DOG] HEALTH CHECK: Session {target}, respond ALIVE within {timeout}s or face termination.
Warrant reason: {reason}
Filed by: {requester}
Attempt: {attempt}/3
```

### Integration with Existing Helpers

The existing `helper` package (`internal/helper/`) manages Supervisor's multi-project helper helpers.
Those are different from shutdown-dance helpers:

| Aspect          | Helper Helpers (existing)      | Dance Helpers (new)           |
|-----------------|-----------------------------|-----------------------------|
| Purpose         | Cross-project infrastructure    | Shutdown dance execution    |
| Sessions        | Claude sessions             | Goroutines (no Claude)      |
| Worktrees       | One per project                 | None                        |
| Lifecycle       | Long-lived, reusable        | Ephemeral per warrant       |
| State           | idle/working                | Dance state machine         |

**Recommendation**: Use different package to avoid confusion:
- `internal/helper/` - existing helper helpers
- `internal/shutdown/` - shutdown dance pool

## Failure Handling

### Helper Crashes Mid-Dance

If a helper crashes (Boot process restarts, system crash):

1. State files persist in `~/gt/supervisor/helpers/active/`
2. On Boot restart, scan for orphaned state files
3. Resume or restart based on state:

| State            | Recovery Action                    |
|------------------|------------------------------------|
| interrogating    | Restart from current attempt       |
| evaluating       | Check response, continue           |
| executing        | Verify kill, mark complete         |
| pardoned/complete| Already done, clean up             |

```go
func (p *DogPool) RecoverOrphans() error {
    files, _ := filepath.Glob(p.stateDir + "/*.json")
    for _, f := range files {
        state := loadDogState(f)
        if state.State != StateComplete && state.State != StatePardoned {
            helper := p.allocateForRecovery(state)
            go helper.Resume()
        }
    }
    return nil
}
```

### Handling Pool Exhaustion

If all helpers are busy when a new warrant arrives, the warrant is queued for
later processing. When a helper completes and is released, the queue is checked
for pending warrants.

## Directory Structure

```
~/gt/
├── daemon/
│   ├── daemon.log              # Daemon activity log
│   └── daemon.pid              # Daemon process ID
├── supervisor/
│   ├── heartbeat.json          # Supervisor freshness (updated each sweep cycle)
│   ├── health-check-state.json # Agent health tracking (gt supervisor health-check)
│   └── helpers/
│       ├── boot/               # Boot's working directory
│       │   ├── CLAUDE.md       # Boot context
│       │   ├── .boot-running   # Boot in-progress marker (TTL: 5 min)
│       │   └── .boot-status.json # Boot last action/result
│       ├── active/             # Active helper state files
│       │   ├── helper-123.json
│       │   └── ...
│       ├── completed/          # Completed dance records (for audit)
│       │   └── helper-789.json
│       └── warrants/           # Pending warrant queue
│           └── warrant-abc.json
```

## Debugging

```bash
# Check Supervisor heartbeat
cat ~/gt/supervisor/heartbeat.json | jq .

# Check Boot status
cat ~/gt/supervisor/helpers/boot/.boot-status.json | jq .

# View daemon log
tail -f ~/gt/daemon/daemon.log

# Manual Boot run
gt watchdog triage

# Manual Supervisor health check
gt supervisor health-check

# Helper pool status
gt helper pool status

# View active shutdown dances
gt helper dances

# View warrant queue
gt helper warrants
```

## Common Issues

### Boot Spawns in Wrong Session

**Symptom**: Boot runs in `hq-supervisor` instead of `gt-boot`
**Cause**: Session name confusion in spawn code
**Fix**: Ensure `gt watchdog triage` specifies `--session=gt-boot`

### Zombie Sessions Block Restart

**Symptom**: tmux session exists but Claude is dead
**Cause**: Daemon checks session existence, not process health
**Fix**: Kill zombie sessions before recreating: `gt session kill hq-supervisor`

### Status Shows Wrong State

**Symptom**: `gt status` shows wrong state for agents
**Cause**: Previously bead state and tmux state could diverge
**Fix**: As of gt-zecmc, status derives state from tmux directly (no bead state for
observable conditions like running/stopped). Non-observable states (stuck, awaiting-gate)
are still stored in beads.

## Summary

The watchdog chain provides autonomous recovery:

- **Daemon**: Mechanical heartbeat, spawns Boot
- **Boot**: Intelligent triage, decides Supervisor fate
- **Supervisor**: Continuous sweep, monitors workers

Boot exists because the daemon can't reason and Supervisor can't observe itself.
The separation costs complexity but enables:

1. **Intelligent triage** without constant AI cost
2. **Fresh context** for each triage decision
3. **Graceful degradation** when tmux unavailable
4. **Multiple fallback** layers for reliability

The helper pool extends this with concurrent shutdown dances -- lightweight
Go state machines that execute warrants without consuming Claude sessions.
