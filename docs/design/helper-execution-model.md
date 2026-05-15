# Helper Execution Model: Imperative vs Template Dispatch

## Status: Active Design Doc
Created: 2026-02-27

## Problem Statement

Gas Town helpers (daemon sweep routines) use two execution models:

1. **Imperative Go** (ticker fires → Go code runs): Doctor, Reaper, JSONL backup, Dolt backup
2. **Template-only** (ticker fires → workflow poured → ... nothing): Compactor (was stub), ~~Janitor~~ (removed)

The template-only helpers were broken because no agent interprets their workflows from
ticker context. The workflow system requires an idle helper to execute the template, but
the ticker fires regardless of helper availability.

After the Beads Flows work, the Compactor has been upgraded to imperative Go.
The Janitor helper was removed entirely — test infrastructure migrated from a
dedicated port-3308 Dolt test server to testcontainers-go (Docker), eliminating
the orphan test database problem at its source.

This document captures the target execution model going forward.

## Current State (Post Testcontainers Migration)

| Helper | Model | Works? | Notes |
|-----|-------|--------|-------|
| Doctor | Imperative Go (466 lines) | Yes | 7 health checks, GC, zombie kill |
| Reaper | Imperative Go (658 lines) | Yes | Close, purge, auto-close, mail purge |
| JSONL Backup | Imperative Go (619 lines) | Yes | Export, scrub, filter, spike detect, push |
| Dolt Backup | Imperative Go | Yes | Filesystem backup sync |
| Compactor | Imperative Go (new) | Yes | Flatten + GC when commits > threshold |

## Target Model

### Keep imperative Go for: reliability-critical helpers

Helpers that MUST run on schedule, unattended, with no agent dependency:

- **Doctor**: Health checks are the foundation. Must run even if all agents are dead.
- **Reaper**: Data hygiene can't depend on agent availability.
- **Compactor**: Compaction must run deterministically on its 24h schedule.
- **JSONL Backup**: Backup integrity can't be left to agent scheduling.
- **Dolt Backup**: Same as JSONL.

**Principle**: If the helper's failure would cause a Clown Show, it must be imperative Go.

### Migrate to plugin dispatch for: enhancement/opportunistic helpers

Helpers whose failure is merely inconvenient, not catastrophic:

- Future: cosmetic cleanup, metrics collection, log rotation.

### Plugin dispatch model

For plugin-dispatched helpers:

1. Remove dedicated ticker from daemon `Run()` loop
2. Create `plugins/<helper>/plugin.md` with cooldown gate
3. `handleDogs()` dispatches to idle helper when cooldown expires
4. Helper agent interprets the plugin template and executes

**Key constraint**: The `handleDogs()` dispatch path already exists and works.
The issue is that ticker-based helpers bypass it. Plugin helpers use it correctly.

## Migration Path

### Future helpers default to plugin
- New helpers should start as plugins unless reliability-critical
- Existing imperative helpers stay as Go (working, tested, reliable)

## Decision: Do NOT migrate working imperative helpers

The Doctor, Reaper, Compactor, and backup helpers work reliably as imperative Go.
Migrating them to template+agent would:

1. Add a dependency on agent availability
2. Introduce latency (agent startup, template interpretation)
3. Risk regression on critical paths
4. Gain nothing — they already work

**The only helpers that should use template dispatch are ones where agent intelligence
adds value** or where the helper's task is inherently non-critical.
