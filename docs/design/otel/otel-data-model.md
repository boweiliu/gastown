# OpenTelemetry Data Model

Complete schema of all telemetry events emitted by Gas Town. Each event consists of:

1. **Log record** (→ any OTLP v1.x+ backend, defaults to VictoriaLogs) with full structured attributes
2. **Metric counter** (→ any OTLP v1.x+ backend, defaults to VictoriaMetrics) for aggregation

> **`run.id` correlation**: automatic `run.id` injection into all log records is implemented in
> PR #2199 (`otel-p0-work-context`), not yet on main. On main, correlation is possible only via
> resource attributes (`gt.role`, `gt.project`, `gt.agent`, `gt.actor`).

---

## Event Index

| Event | Category | Status |
|-------|----------|--------|
| `session.start` | Session | ✅ Main |
| `session.stop` | Session | ✅ Main |
| `agent.event` | Agent | 🔲 PR #2199 |
| `agent.usage` | Agent | 🔲 PR #2199 |
| `agent.state_change` | Agent | ✅ Main |
| `bd.call` | Work | ✅ Main |
| `mail` | Work | ✅ Main |
| `prime` | Workflow | ✅ Main |
| `prime.context` | Workflow | ✅ Main |
| `prompt.send` | Workflow | ✅ Main |
| `message` | Workflow | ✅ Main |
| `dispatch` | Workflow | ✅ Main |
| `done` | Workflow | ✅ Main |
| `worker.spawn` | Lifecycle | ✅ Main |
| `worker.remove` | Lifecycle | ✅ Main |
| `daemon.restart` | Lifecycle | ✅ Main |
| `pane.read` | Internal | ✅ Main |
| `pane.output` | Internal | ✅ Main |
| `template.instantiate` | Workflow | ✅ Main |
| `batch.create` | Workflow | ✅ Main |
| `agent.instantiate` | Session | ❌ Roadmap |
| `mol.cook` | Workflow | ❌ Roadmap |
| `mol.ephemeral` | Workflow | ❌ Roadmap |
| `mol.squash` | Workflow | ❌ Roadmap |
| `mol.burn` | Workflow | ❌ Roadmap |
| `bead.create` | Workflow | ❌ Roadmap |

---

## 1. Identity hierarchy

### 1.1 Instance

The outermost grouping. Derived at agent spawn time from the machine hostname
and the workspace root directory basename.

| Attribute | Type | Description |
|---|---|---|
| `instance` | string | `hostname:basename(town_root)` — e.g. `"laptop:gt"` |
| `town_root` | string | absolute path to the workspace root — e.g. `"/Users/pa/gt"` |

### 1.2 Run

Resource attributes set at process start via `OTEL_RESOURCE_ATTRIBUTES` (populated by
`buildGTResourceAttrs()` in `internal/telemetry/subprocess.go`).

| Attribute | Type | Source | Notes |
|---|---|---|---|
| `gt.role` | string | `GT_ROLE` env var | e.g. `"gastown/workers/Toast"` |
| `gt.project` | string | `GT_RIG` env var | e.g. `"gastown"` |
| `gt.actor` | string | `BD_ACTOR` env var | bd actor identity |
| `gt.agent` | string | `GT_worker` or `GT_team` env var | agent name |
| `gt.session` | string | `GT_SESSION` env var | tmux session name — **PR #2199** |
| `gt.run_id` | string | `GT_RUN` env var | correlation key — **PR #2199** |
| `gt.work_rig` | string | `GT_WORK_RIG` env var | work project at last `gt prime` — **PR #2199** |
| `gt.work_bead` | string | `GT_WORK_BEAD` env var | assigned bead at last `gt prime` — **PR #2199** |
| `gt.work_mol` | string | `GT_WORK_MOL` env var | workflow step at last `gt prime` — **PR #2199** |

> Attributes marked **PR #2199** are only set after `otel-p0-work-context` merges.
> On main, only `gt.role`, `gt.project`, `gt.actor`, `gt.agent` are set.

---

## 2. Events

### `session.start` / `session.stop`

tmux session lifecycle events.

| Attribute | Type | Description |
|---|---|---|
| `session_id` | string | tmux pane name |
| `role` | string | gastown role |
| `status` | string | `"ok"` · `"error"` |
| `error` | string | error message; empty when `"ok"` |

---

### `prime`

Emitted on each `gt prime` invocation. The rendered template is emitted
separately as `prime.context` (same attributes plus `template`).

| Attribute | Type | Description |
|---|---|---|
| `role` | string | gastown role |
| `hook_mode` | bool | true when invoked from a hook |
| `status` | string | `"ok"` · `"error"` |
| `error` | string | error message; empty when `"ok"` |
| `work_rig` | string | ⚠️ **PR #2199** — project whose bead is on the hook |
| `work_bead` | string | ⚠️ **PR #2199** — bead ID currently assigned |
| `work_mol` | string | ⚠️ **PR #2199** — workflow ID if the bead is a workflow step; empty otherwise |

---

### `prime.context`

Companion to `prime`, emitted in the same invocation. Carries the full rendered template text.

| Attribute | Type | Description |
|---|---|---|
| `role` | string | gastown role |
| `hook_mode` | bool | true when invoked from a hook |
| `template` | string | full rendered template text |
| `status` | string | `"ok"` · `"error"` |

---

### `prompt.send`

Each `gt sendkeys` dispatch to an agent's tmux pane. Prompt content is **not** logged —
only the length is recorded.

| Attribute | Type | Description |
|---|---|---|
| `session` | string | tmux pane name |
| `keys_len` | int | prompt length in bytes |
| `debounce_ms` | int | applied debounce delay |
| `status` | string | `"ok"` · `"error"` |
| `error` | string | error message; empty when `"ok"` |

---

### `agent.event`

> **Status: PR #2199 (`otel-p0-work-context`)** — not on main. Requires `GT_LOG_AGENT_OUTPUT=true` and `GT_OTEL_LOGS_URL`.

One record per content block in the agent's conversation log. Full content, no truncation.

| Attribute | Type | Description |
|---|---|---|
| `session` | string | tmux pane name |
| `native_session_id` | string | agent-native session UUID (Claude Code JSONL filename UUID) |
| `agent_type` | string | adapter name (`"claudecode"`, `"opencode"`) |
| `event_type` | string | `"text"` · `"tool_use"` · `"tool_result"` · `"thinking"` |
| `role` | string | `"assistant"` · `"user"` |
| `content` | string | full content — LLM text, tool JSON input, tool output |

For `tool_use`: `content = "<tool_name>: <full_json_input>"`
For `tool_result`: `content = <full tool output>`

---

### `agent.usage`

> **Status: PR #2199 (`otel-p0-work-context`)** — not on main. Requires `GT_LOG_AGENT_OUTPUT=true`.

One record per assistant turn (not per content block, to avoid
double-counting).

| Attribute | Type | Description |
|---|---|---|
| `session` | string | tmux pane name |
| `native_session_id` | string | agent-native session UUID |
| `input_tokens` | int | `input_tokens` from the API usage field |
| `output_tokens` | int | `output_tokens` from the API usage field |
| `cache_read_tokens` | int | `cache_read_input_tokens` |
| `cache_creation_tokens` | int | `cache_creation_input_tokens` |

---

### `bd.call`

Each invocation of the `bd` CLI, whether by the Go daemon or by the agent
in a shell.

| Attribute | Type | Description |
|---|---|---|
| `subcommand` | string | bd subcommand (`"ready"`, `"update"`, `"create"`, …) |
| `args` | string | full argument list |
| `duration_ms` | float | wall-clock duration in milliseconds |
| `stdout` | string | full stdout (opt-in: `GT_LOG_BD_OUTPUT=true`) |
| `stderr` | string | full stderr (opt-in: `GT_LOG_BD_OUTPUT=true`) |
| `status` | string | `"ok"` · `"error"` |
| `error` | string | error message; empty when `"ok"` |

---

### `mail`

All operations on the gastown mail system. Carries operation and result only;
message payload attributes are not recorded.

| Attribute | Type | Description |
|---|---|---|
| `operation` | string | `"send"` · `"read"` · `"archive"` · `"list"` · `"delete"` · … |
| `status` | string | `"ok"` · `"error"` |
| `error` | string | error message; empty when `"ok"` |

Call `RecordMail(ctx, operation, err)` for all mail operations.

---

### `agent.state_change`

Emitted whenever an agent transitions to a new state (idle → working, etc.).

| Attribute | Type | Description |
|---|---|---|
| `agent_id` | string | agent identifier |
| `new_state` | string | new state (`"idle"`, `"working"`, `"done"`, …) |
| `has_assignment_bead` | bool | `true` when the agent has a non-empty bead on its hook |
| `status` | string | `"ok"` · `"error"` |
| `error` | string | error message; empty when `"ok"` |

> Note: the attribute is `has_assignment_bead` (bool), not `assignment_bead` (string).
> The bead ID itself is not recorded in the state change event.

---

### `pane.read`

Each tmux `CapturePane` call to read agent output.

| Attribute | Type | Description |
|---|---|---|
| `session` | string | tmux pane name |
| `lines_requested` | int | number of lines requested |
| `content_len` | int | byte length of captured content |
| `status` | string | `"ok"` · `"error"` |
| `error` | string | error message; empty when `"ok"` |

---

### `pane.output`

Raw pane output chunks emitted to VictoriaLogs (streaming tail of agent output).

| Attribute | Type | Description |
|---|---|---|
| `session` | string | tmux pane name |
| `content` | string | captured pane content chunk |

---

### Other events

All carry `status` and `error` fields.

| Event body | Key attributes | Metric |
|---|---|---|
| `dispatch` | `bead`, `target`, `status`, `error` | `gastown.dispatch.dispatches.total` |
| `message` | `target`, `status`, `error` | `gastown.message.total` |
| `done` | `exit_type` (`COMPLETED` · `ESCALATED` · `DEFERRED`), `status`, `error` | `gastown.done.total` |
| `worker.spawn` | `name`, `status`, `error` | `gastown.worker.spawns.total` |
| `worker.remove` | `name`, `status`, `error` | `gastown.worker.removes.total` |
| `template.instantiate` | `template_name`, `bead_id`, `status`, `error` | `gastown.template.instantiations.total` |
| `batch.create` | `bead_id`, `status`, `error` | `gastown.batch.creates.total` |
| `daemon.restart` | `agent_type` | `gastown.daemon.agent_restarts.total` |

---

## 3. Roadmap Events (not yet implemented)

The following events have no corresponding `Record*` function in `internal/telemetry/recorder.go`.
They are listed here to document intended design.

### `agent.instantiate` *(roadmap)*

Intended to anchor all subsequent events for a run. One span per agent spawn.

| Attribute | Type | Description |
|---|---|---|
| `agent_type` | string | `"claudecode"` · `"opencode"` · … |
| `role` | string | gastown role |
| `agent_name` | string | agent name |
| `session_id` | string | tmux pane name |
| `project` | string | allocation project (empty for generic workers) |
| `issue_id` | string | bead ID passed at spawn via `--issue`; empty if none |
| `git_branch` | string | git branch of the working directory at spawn time |
| `git_commit` | string | HEAD SHA of the working directory at spawn time |

### `mol.cook` / `mol.ephemeral` / `mol.squash` / `mol.burn` *(roadmap)*

Workflow lifecycle events. No `RecordMol*` functions exist yet.

### `bead.create` *(roadmap)*

Per-child-bead event during workflow instantiation. No `RecordBeadCreate` function exists yet.

---

## 4. Metrics Reference

| Metric | Type | Labels | Status |
|--------|------|--------|--------|
| `gastown.session.starts.total` | Counter | `status`, `role` | ✅ Main |
| `gastown.session.stops.total` | Counter | `status` | ✅ Main |
| `gastown.agent.state_changes.total` | Counter | `status`, `new_state` | ✅ Main |
| `gastown.bd.calls.total` | Counter | `status`, `subcommand` | ✅ Main |
| `gastown.bd.duration_ms` | Histogram | `subcommand` | ✅ Main |
| `gastown.mail.operations.total` | Counter | `status`, `operation` | ✅ Main |
| `gastown.prime.total` | Counter | `status`, `role`, `hook_mode` | ✅ Main |
| `gastown.prompt.sends.total` | Counter | `status` | ✅ Main |
| `gastown.pane.reads.total` | Counter | `status` | ✅ Main |
| `gastown.pane.output.total` | Counter | `session` | ✅ Main |
| `gastown.message.total` | Counter | `status` | ✅ Main |
| `gastown.dispatch.dispatches.total` | Counter | `status` | ✅ Main |
| `gastown.done.total` | Counter | `status`, `exit_type` | ✅ Main |
| `gastown.worker.spawns.total` | Counter | `status` | ✅ Main |
| `gastown.worker.removes.total` | Counter | `status` | ✅ Main |
| `gastown.daemon.agent_restarts.total` | Counter | `agent_type` | ✅ Main |
| `gastown.template.instantiations.total` | Counter | `status`, `template` | ✅ Main |
| `gastown.batch.creates.total` | Counter | `status` | ✅ Main |
| `gastown.agent.events.total` | Counter | `session`, `event_type`, `role` | 🔲 PR #2199 |

---

## 5. Recommended indexed attributes

```
gt.role, gt.project, gt.actor, gt.agent, session_id, event_type, subcommand,
operation, new_state, exit_type
```

---

## 6. Environment variables

| Variable | Set by | Description |
|---|---|---|
| `GT_OTEL_LOGS_URL` | daemon startup | OTLP logs endpoint URL |
| `GT_OTEL_METRICS_URL` | daemon startup | OTLP metrics endpoint URL |
| `GT_LOG_BD_OUTPUT` | operator | Set to `true` to include bd stdout/stderr in `bd.call` log records |
| `GT_LOG_AGENT_OUTPUT` | operator | **PR #2199** — set to `true` to enable agent conversation event streaming. Requires `GT_OTEL_LOGS_URL`. |
| `GT_RUN` | tmux session / subprocess | **PR #2199** — run UUID; correlation key across all events |

---

## 7. Status Field Semantics

All events include a `status` field:

| Value | Meaning |
|-------|---------|
| "ok" | Operation completed successfully |
| "error" | Operation failed |

When status is "error", the `error` field contains the error message. When status is "ok", `error` is an empty string.

---

## 8. Backend Compatibility

This data model is **backend-agnostic** — any OTLP v1.x+ compatible backend can consume these events:

- **VictoriaMetrics/VictoriaLogs** — Default for local development. Override with `GT_OTEL_METRICS_URL`/`GT_OTEL_LOGS_URL` to use any OTLP-compatible backend.
- **Prometheus** — Via remote_write receiver
- **Grafana Mimir** — Via write endpoint
- **OpenTelemetry Collector** — Universal forwarder to any backend

The schema uses standard OpenTelemetry Protocol (OTLP) with protobuf encoding, which is universally supported.

---

## Appendix: Source Reference Audit

Audited against `origin/main` @ `2d8d71ee35fafda3bbdf353683692bfcc9165476`

### Metrics (`internal/telemetry/recorder.go`)

| Claim | Source |
|-------|--------|
| `initInstruments()` function | `recorder.go:59` |
| `gastown.bd.calls.total` Counter | `recorder.go:64` |
| `gastown.session.starts.total` Counter | `recorder.go:67` |
| `gastown.session.stops.total` Counter | `recorder.go:70` |
| `gastown.prompt.sends.total` Counter | `recorder.go:73` |
| `gastown.pane.reads.total` Counter | `recorder.go:76` |
| `gastown.pane.output.total` Counter | `recorder.go:79` |
| `gastown.prime.total` Counter | `recorder.go:82` |
| `gastown.agent.state_changes.total` Counter | `recorder.go:85` |
| `gastown.worker.spawns.total` Counter | `recorder.go:88` |
| `gastown.worker.removes.total` Counter | `recorder.go:91` |
| `gastown.dispatch.dispatches.total` Counter | `recorder.go:94` |
| `gastown.mail.operations.total` Counter | `recorder.go:97` |
| `gastown.message.total` Counter | `recorder.go:100` |
| `gastown.done.total` Counter | `recorder.go:103` |
| `gastown.daemon.agent_restarts.total` Counter | `recorder.go:106` |
| `gastown.template.instantiations.total` Counter | `recorder.go:109` |
| `gastown.batch.creates.total` Counter | `recorder.go:112` |
| `gastown.bd.duration_ms` Histogram | `recorder.go:117` |

### Log events (`internal/telemetry/recorder.go`)

| Event | Function | Key attributes | Source |
|-------|----------|----------------|--------|
| `bd.call` | `RecordBDCall` | `subcommand`, `args`, `duration_ms`, `status`, `error`, `stdout`/`stderr` (opt-in) | `recorder.go:187`, emit at `recorder.go:214` |
| `session.start` | `RecordSessionStart` | `session_id`, `role`, `status`, `error` | `recorder.go:218`, emit at `recorder.go:227` |
| `session.stop` | `RecordSessionStop` | `session_id`, `status`, `error` | `recorder.go:236`, emit at `recorder.go:242` |
| `prompt.send` | `RecordPromptSend` | `session`, `keys_len`, `debounce_ms`, `status`, `error` | `recorder.go:250`, emit at `recorder.go:256` |
| `pane.read` | `RecordPaneRead` | `session`, `lines_requested`, `content_len`, `status`, `error` | `recorder.go:266`, emit at `recorder.go:272` |
| `prime` | `RecordPrime` | `role`, `hook_mode`, `status`, `error` | `recorder.go:282`, emit at `recorder.go:292` |
| `prime.context` | `RecordPrimeContext` | `role`, `hook_mode`, `template` | `recorder.go:305`, emit at `recorder.go:310` |
| `agent.state_change` | `RecordAgentStateChange` | `agent_id`, `new_state`, `has_assignment_bead` (bool), `status`, `error` | `recorder.go:318`, emit at `recorder.go:328` |
| `worker.spawn` | `RecordWorkerSpawn` | `name`, `status`, `error` | `recorder.go:338`, emit at `recorder.go:344` |
| `worker.remove` | `RecordWorkerRemove` | `name`, `status`, `error` | `recorder.go:352`, emit at `recorder.go:358` |
| `dispatch` | `RecordDispatch` | `bead`, `target`, `status`, `error` | `recorder.go:366`, emit at `recorder.go:372` |
| `mail` | `RecordMail` | `operation`, `status`, `error` | `recorder.go:381`, emit at `recorder.go:390` |
| `message` | `RecordNudge` | `target`, `status`, `error` | `recorder.go:398`, emit at `recorder.go:404` |
| `done` | `RecordDone` | `exit_type`, `status`, `error` | `recorder.go:413`, emit at `recorder.go:422` |
| `daemon.restart` | `RecordDaemonRestart` | `agent_type` | `recorder.go:431`, emit at `recorder.go:436` |
| `template.instantiate` | `RecordTemplateInstantiate` | `template_name`, `bead_id`, `status`, `error` | `recorder.go:442`, emit at `recorder.go:451` |
| `batch.create` | `RecordBatchCreate` | `bead_id`, `status`, `error` | `recorder.go:460`, emit at `recorder.go:466` |
| `pane.output` | `RecordPaneOutput` | `session`, `content` | `recorder.go:477`, emit at `recorder.go:482` |

### `prompt.send`: `keys` attribute absent (confirmed)

`RecordPromptSend` passes `keys string` but only emits `keys_len` (`int64(len(keys))`). The prompt content is deliberately not logged. `recorder.go:256–263`.

### `agent.state_change`: `has_assignment_bead` is bool, not string

`assignmentBead *string` pointer is converted to bool: `hasAssignmentBead := assignmentBead != nil && *assignmentBead != ""`. Emitted as `has_assignment_bead` bool at `recorder.go:321,328`.

### `mail`: no `msg.*` attributes

`RecordMail(ctx, operation, err)` at `recorder.go:381` only emits `operation`, `status`, `error`. No `msg.id`, `msg.from`, `msg.to`, etc. No `RecordMailMessage` function exists — grep `recorder.go` for `RecordMailMessage` → zero matches.

### GT_LOG_BD_OUTPUT

`recorder.go:208` — `os.Getenv("GT_LOG_BD_OUTPUT") == "true"` gates `stdout`/`stderr` logging.

### Absent events (confirmed by grep)

| Claim | Verification |
|-------|-------------|
| `agent.instantiate` — does not exist | `grep -r "agent.instantiate" internal/ → zero matches` |
| `RecordAgentInstantiate` — does not exist | `grep -r "RecordAgentInstantiate" internal/ → zero matches` |
| `mol.cook/ephemeral/squash/burn` — do not exist | `grep -r "mol\.cook\|mol\.ephemeral\|mol\.squash\|mol\.burn" internal/ → zero matches` |
| `bead.create` — does not exist | `grep -r "bead\.create\|RecordBeadCreate" internal/ → zero matches` |
| `RecordMailMessage` — does not exist | `grep -r "RecordMailMessage\|MailMessageInfo" internal/ → zero matches` |
| `gastown.agent.instantiations.total` — not in `initInstruments()` | `grep -r "agent.instantiations" internal/ → zero matches` |
| `gastown.mol.cooks.total` etc. — not in `initInstruments()` | `grep -r "mol\.cooks\|mol\.ephemerals\|mol\.squashes\|mol\.burns" internal/ → zero matches` |
| `gastown.bead.creates.total` — not in `initInstruments()` | `grep -r "bead\.creates" internal/ → zero matches` |

### PR #2199 additions (in `otel-p0-work-context`, not yet on main)

| Claim | Source (commit `8b88de15`) |
|-------|---------------------------|
| `RecordAgentEvent` / `agent.event` | `recorder.go` (added in `8b88de15`) |
| `RecordAgentTokenUsage` / `agent.usage` | `recorder.go` (added in `8b88de15`) |
| `gastown.agent.events.total` Counter | `recorder.go` (added in `8b88de15`) |
| `WithRunID(ctx, runID)` / `RunIDFromCtx(ctx)` | `recorder.go` (added in `8b88de15`) |
| `addRunID(ctx, *record)` — injects `run.id` into all emit calls | `recorder.go` (added in `8b88de15`) |
| `gt.session` in `OTEL_RESOURCE_ATTRIBUTES` | `subprocess.go` (updated in `8b88de15`) |
| `gt.run_id` in `OTEL_RESOURCE_ATTRIBUTES` | `subprocess.go` (updated in `8b88de15`) |
| `gt.work_rig/bead/mol` in `OTEL_RESOURCE_ATTRIBUTES` | `subprocess.go` (updated in `8b88de15`) |
| `GT_RUN` propagation to subprocesses | `subprocess.go` (updated in `8b88de15`) |
| `work_rig`, `work_bead`, `work_mol` on `prime` event | `recorder.go` (updated in `8b88de15`) |
| `internal/agentlog/` package | new package in `8b88de15` |
| `internal/cmd/agent_log.go` | new file in `8b88de15` |
| `internal/session/agent_logging_unix.go` | new file in `8b88de15` |
| `GT_LOG_AGENT_OUTPUT` env var | new in `8b88de15` |
| `telemetry.IsActive()` | `telemetry.go` (added in `8b88de15`) |
