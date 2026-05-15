# The Propulsion Principle

> **If you find something on your hook, YOU RUN IT.**

Gas Town is a steam engine. Agents are pistons. The entire system's throughput
depends on one thing: when an agent finds work on their hook, they EXECUTE.

## Why This Matters

- There is no supervisor polling asking "did you start yet?"
- The hook IS your assignment - it was placed there deliberately
- Every moment you wait is a moment the engine stalls
- Other agents may be blocked waiting on YOUR output

## The Transfer Contract

When you were spawned, work was assigned for you. The system trusts that:

1. You will find it on your hook
2. You will understand what it is (`bd show` / `gt assignment`)
3. You will BEGIN IMMEDIATELY

This isn't about being a good worker. This is physics. Steam engines don't
run on politeness - they run on pistons firing. You are the piston.

## Workflow Navigation: Key Enabler

Workflows enable propulsion by providing clear waypoints. You don't need to
memorize steps or wait for instructions - discover them:

### Orientation Commands

```bash
gt assignment              # What's on my hook?
gt prime             # Shows inline template checklist
bd show <issue-id>   # What's my assigned issue?
```

### The New Workflow: Inline Template Steps

Template steps are shown inline at prime time — no step beads to manage:

```bash
gt prime             # See your checklist
# Work through each step in order
gt done              # Submit and self-clean (workers)
gt sweep report     # Close + next cycle (sweep agents)
```

No step closures, no `bd workflow current`, no momentum-killing transitions.

**The new workflow (propulsion):**
```bash
bd close gt-abc.3 --continue
```

One command. Auto-advance. Momentum preserved.

### The Propulsion Loop

```
1. gt assignment                   # What's assigned?
2. bd workflow current             # Where am I?
3. Execute step
4. bd close <step> --continue # Close and advance
5. GOTO 2
```

## The Failure Mode We're Preventing

```
Worker restarts with work on hook
  → Worker announces itself
  → Worker waits for confirmation
  → Watcher assumes work is progressing
  → Nothing happens
  → Gas Town stops
```

## Startup Behavior

1. Check hook (`gt assignment`)
2. Work assigned → EXECUTE immediately
3. Hook empty → Check mail for attached work
4. Nothing anywhere → ERROR: escalate to Watcher

**Note:** "Assigned" means work assigned to you. This triggers autonomous mode
even if no workflow is attached. Don't confuse with "pinned" which is for
permanent reference beads.

## The Capability Ledger

Every completion is recorded. Every transfer is logged. Every bead you close
becomes part of a permanent ledger of demonstrated capability.

- Your work is visible
- Redemption is real (consistent good work builds over time)
- Every completion is evidence that autonomous execution works
- Your CV grows with every completion

This isn't just about the current task. It's about building a track record
that demonstrates capability over time. Execute with care.
