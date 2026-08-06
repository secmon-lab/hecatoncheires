# Operations Guide

This guide collects the day-2 runbook material for operating Hecatoncheires:
observability and error reporting, agent Job operations, scheduled-sweep
wiring, data migrations, the one-shot diagnosis jobs, and backup guidance for
the Firestore and Cloud Storage state.

The commands themselves (flags, env vars) are documented in the
[CLI Reference](./cli.md); this guide focuses on the operational use of those
commands. Declarative configuration (workspace TOML, the `[[job]]` schema,
system prompt) lives in the [Configuration Guide](./configuration.md).

## Observability (Sentry)

The server can forward errors to [Sentry](https://sentry.io/) in addition to
the structured log. Sentry is opt-in: leaving `HECATONCHEIRES_SENTRY_DSN`
empty keeps the SDK uninitialized and the integration becomes a cheap no-op
(one atomic flag check per error).

### Environment variables

| Env Var | Default | Description |
|---------|---------|-------------|
| `HECATONCHEIRES_SENTRY_DSN` | - | Sentry DSN. **Setting this enables Sentry.** |
| `HECATONCHEIRES_SENTRY_ENV` | - | Environment tag (`production`, `staging`, etc.) |
| `HECATONCHEIRES_SENTRY_RELEASE` | - | Release identifier — set to the commit SHA in CI |

### What gets reported

Every call to `errutil.Handle` (and the HTTP variant) feeds the error to
Sentry's `CaptureException`. `goerr` values attached to the error appear as
the **`goerr_values`** Sentry context, so structured fields such as
`slack_error`, `query`, or `case_id` show up alongside the exception
without per-call site changes.

For HTTP requests, Sentry middleware sits right after `RequestID` so
captures inside a handler automatically carry the request URL, method, and
headers. Panics propagate through the middleware (`Repanic: true`) so chi's
`Recoverer` still produces a `500` response after Sentry has captured the
event.

### Operational troubleshoot

- **Slack `missing_scope` even after adding the scope**: re-install the
  Slack App to the workspace. Adding scopes in the App Manifest does not
  re-issue existing tokens; the `xoxp-...` you had before the change still
  carries the old scope set. Re-install (Install App → Reinstall to
  Workspace) and replace the token in `HECATONCHEIRES_SLACK_USER_OAUTH_TOKEN`.
  See the [Slack Integration Guide](./slack.md) for the full re-install flow.
- **Confirming what scope Slack actually wants**: when a Slack call fails,
  the wrapped error carries the `slack_error` / `slack_response_messages` /
  `slack_response_warnings` fields. Both the structured log and the Sentry
  `goerr_values` context include these — search for them to find the
  Slack-side error code without parsing free-form error strings.

## Agent Jobs operations

Agent Jobs run LLM-powered automation against Case lifecycle events and
periodic ticks. The declarative definition — the `[[job]]` TOML schema,
execution strategy, system prompt, per-case customisation, and the tool
palette — is documented in the [Configuration Guide](./configuration.md).
This chapter covers the operator-facing runtime behaviour: triggers,
concurrency, failure handling, the run-log event trail, and scheduling.

### When a Job runs

Two event domains can trigger a Job:

| Domain      | When it fires |
|-------------|---------------|
| `case`      | Case lifecycle transitions (`created`, `closed`). Fired by `CaseUseCase` immediately after persistence. |
| `scheduled` | A duration (`every`) or cron expression (`cron`) elapsed since the last successful run. Fired by the `hecatoncheires tick` CLI or the `POST /hooks/tick` endpoint. |

A Job may subscribe to multiple domains; the runtime fires one
invocation per matching `(job, case)` tuple. The full subscription syntax
(`events.case` / `events.scheduled`) is documented with the
[`[[job]]` schema](./configuration.md).

### Tools available to a Job

The exact tool palette a Job gets — and how it differs from the interactive
mention agent — is documented once, in
**[Agent Tools → Tools available by context](agent_tools.md#tools-available-by-context)**.
The short version, because it surprises people:

- A Job gets **case edits** (`case__update_case`, `case__assign` /
  `case__unassign`, and `case__update_case_status` where a case status set
  exists), **action management** in channel-mode workspaces, **workspace
  knowledge**, **memos** (when enabled), **web fetch**, and a single
  channel-pinned Slack poster (`slack__post_to_case_channel`).
- A Job does **NOT** get the Slack *search* tools, Notion, or GitHub. Those are
  wired only into the interactive / investigation contexts. A Job that needs
  external context must already have it in the case — it cannot fetch it live.

Writer mutations run as `model.SystemActorID` (`"@system"`). The
`CaseUseCase.UpdateCase` path skips Slack user-token permission checks
when the actor is the system identifier.

### Lifecycle of an invocation

```mermaid
flowchart LR
  EV[Lifecycle event\nor scheduled tick] --> PUB[JobUseCase.Publish]
  PUB -->|matching jobs| DISP[async.Dispatch]
  DISP --> RUN[JobRunner.Run]
  RUN --> LEASE[Lease via JobRunRepository]
  LEASE -->|busy| SKIP[silent skip]
  LEASE --> SLOT[Concurrency slot\nscheduled runs only]
  SLOT -->|slots full| SKIP
  SLOT --> EXEC[SingleLoopJobExecutor]
  EXEC --> LLM[gollem agent]
  LLM <-->|tool calls| TOOLS[(read-only + writer tools)]
  EXEC --> REC[RecordRun]
```

#### Event matching

Matching depends on the event's domain, and the two domains behave
differently on purpose:

- **`case`** — a fan-out. Every Job whose `events.case.on` contains the
  published lifecycle fires, so several Jobs can react to one
  transition.
- **`scheduled`** — not a fan-out. The sweep decides due-ness per Job
  (each Job has its own `every` / `cron`), so the event names the Job it
  was raised for and dispatch runs that Job only. An event reaching
  `Publish` without a Job id — which no current publisher produces — is
  reported through `errutil.Handle` and dropped rather than broadcast to
  every scheduled Job in the workspace.

These log lines make the matching auditable without reading run history:

| Message | Fields | Read it as |
|---------|--------|------------|
| `scheduled sweep completed` (INFO, one per workspace per sweep) | `workspace_id`, `scheduled_jobs`, `open_cases`, `due_published` | How many `(job, case)` pairs this sweep found due. `due_published` is normally far below `scheduled_jobs × open_cases`. |
| `job event dispatched` (INFO, one per published event) | `domain`, `workspace_id`, `case_id`, `event_job_id`, `dispatched_job_ids`, `dispatched` | Which Jobs the event was dispatched to. For `domain=scheduled`, `dispatched_job_ids` must be exactly `[event_job_id]`; more than that means the event fanned out to Jobs that were not due. |
| `job: invalid event dropped` (ERROR) | `error`, `values` | An event failed validation and fired nothing — e.g. a scheduled event carrying no `job_id`. |

The dispatch line is written when the run is handed to `async.Dispatch`, before
the run takes its lease — so it reports what the event was addressed to, not
that a run executed. A dispatched run can still be skipped by the lease (see
Concurrency below); confirm actual executions in the run history on the Case
agent page.

#### Concurrency

The `JobRunRepository` provides a per-(workspace, case, job) lease. A
second invocation that arrives while the first holds the lease is
**silently skipped** — duplicate triggers from rapid lifecycle toggles
or two scheduler ticks landing close together are absorbed safely.

The default lease is 10 minutes. `RecordRun` clears the lease on
completion; if the runner crashes the lease times out on its own.

#### Deployment-wide concurrency limit

The lease above serialises one `(workspace, case, job)` tuple; it does
**not** bound how many Jobs run at once. A tick can make hundreds of
`(job, case)` pairs due simultaneously, and every one of them would
otherwise start its own LLM run — enough to exceed the provider's rate
limit. `--job-max-concurrency` (`HECATONCHEIRES_JOB_MAX_CONCURRENCY`,
**default `1`**) caps the number of concurrently executing **scheduled**
Job runs across every instance of the deployment.

How it works:

- The limit `N` is stored as `N` slot documents (`jobSlots/{index}`, a
  top-level Firestore collection — the limit spans every workspace
  because it protects one shared LLM quota). An occupied slot is a
  document whose `ExpiresAt` is in the future; a free slot has no
  document at all, so the number of stored documents *is* the number of
  in-flight runs. No counter, nothing to drift.
- `JobRunner.Run` takes a slot after the lease and suspension checks and
  before it builds any prompt, then releases it in a defer. While the run
  executes, a heartbeat pushes `ExpiresAt` forward every 10 seconds
  (TTL 30 seconds), so a crashed instance's slot frees itself within
  ~30 seconds without any cleanup sweep. A hold stops renewing after
  2 hours as a backstop against a leaked slot; the run itself continues.
- **When no slot is free the run is skipped, not queued.** It records
  nothing, so `LastRunAt` is untouched and the next tick finds the Job
  due again — the effect is a postponement to a later tick, not a lost
  run. The skip is logged at `INFO`
  (`job run skipped: concurrency slots full`) and is not reported to
  Sentry.
- If the slot state cannot be read (Firestore error) the run is
  **refused**, not started: with the state unknown, starting anyway
  invites the very rate-limit blowout the gate prevents. The error is
  reported via `errutil.Handle`, and the next tick retries.
- An interactive Job that suspends to ask the user releases its slot
  immediately — a human wait never pins the limit. The resume is not
  gated.

Scope and operational notes:

- **Only the scheduled domain is gated.** Case lifecycle events, the web
  UI's manual Run button, and interactive resumes are single
  user-visible actions with no retry path, so they run regardless of how
  many slots are occupied.
- **Set the same value on every instance**, including the `tick`
  entry point (`serve` and `tick` both dispatch Job runs). An instance
  configured with a larger value will admit up to that larger number.
- **`0` disables the limit** (the pre-1.x behaviour: unbounded fan-out).
- During a rolling deploy, instances still running the older build do
  not consult the gate, so the effective concurrency can exceed the
  limit until the rollout completes.
- With `--repository-backend=memory` the slots live in process memory,
  so the limit binds only within a single instance. Production uses
  Firestore.

#### Loop suppression

Mutations a Job's tool performs run with a context-marker carrying the
originating job id (`job.JobActorMarker`). `JobUseCase.Publish` skips
only the job whose id matches that marker, so a Job that touches
`case__update_case` cannot re-fire itself — but any *other* Job
listening on the resulting lifecycle event still fires. This is what
lets an on-created Job that closes the case trigger the on-closed Job.

Note that per-JobID suppression does **not** make cross-Job loops
structurally impossible. In thread mode a case can be reopened and
re-closed via the status tool, so `closed` is not a one-way edge; two
Jobs that both listen on the same lifecycle and whose agents reopen and
re-close the case could ping-pong indefinitely. Loop-freedom relies on
agents not performing such reopen cycles — typical read-and-post Jobs
(summarisers, notifiers) do not — rather than on the trigger graph's
topology. If a future Job configuration genuinely needs to reopen cases,
add a topology-independent safety valve (per-(case, lifecycle) dispatch
cap or depth guard) before relying on it.

### Running scheduled Jobs

There are two entry points for the time-driven sweep. Both end in the
same `ScheduledScanner.Scan` call.

#### CLI: one-shot sweep

```sh
$ hecatoncheires tick --config /etc/hecatoncheires/workspaces/
```

Suitable for `cron`, GitHub Actions, or any external timer. The command
exits when the sweep and every dispatched Job goroutine finish.

#### HTTP: `POST /hooks/tick`

Available on the `hecatoncheires serve` HTTP server. Wire to Cloud
Scheduler / Eventarc / your preferred scheduler. The endpoint:

- Is **unauthenticated by design.** Deploy behind IAP / Cloud Run
  internal-only ingress / private networking. Do NOT expose to the
  public internet.
- Responds `200` immediately; the sweep runs in a background goroutine.
- Ignores the request body.

### Failure handling

- Job validation errors (TOML schema, unknown lifecycle value, bad cron):
  loud failure at config load — startup aborts.
- LLM errors / tool errors during a run: recorded via `errutil.Handle`
  (Sentry-bound) and persisted to `JobRunRepository` as `FAILED`. A
  matching `RUN_ERROR` entry is appended to the per-Run event log (see
  next section).
- Workspace / case loading failures inside the runner: recorded as
  `FAILED` on the JobRun lock doc; the lease is released so a retry can
  pick up. No `JobRunLog` is written for these *prepare-stage* failures
  because no RunID has been allocated yet.

### Measuring runs from the logs

The Firestore records below reconstruct *one* run in detail. To answer
"is a single run slow, or is capacity going unused?" across a whole
sweep, three aggregate log lines are emitted at INFO. They are designed
to be read together and contain no prompt, model output, tool argument
or tool result — only identifiers, counters and durations.

#### `job run finished` — one line per run attempt

Emitted by `JobRunner.Run` and `Resume` on **every** exit, including the
skips that perform no work. `outcome` is the discriminator, so there is
no need to match message strings:

| `outcome` | Meaning |
|-----------|---------|
| `completed` | Reached the executor and finished without error. |
| `failed` | Any failure, including a prepare-stage one that never reached the executor. |
| `suspended` | An interactive run asked the user and paused. |
| `skipped_lease` | Another runner held the `(workspace, case, job)` lease. |
| `skipped_suspended` | Stepped aside for a genuinely open question. |
| `skipped_slots_full` | Refused by the deployment-wide concurrency gate. |
| `skipped_stale` | A resume whose run was no longer awaiting input. |

Fields: `workspace_id`, `case_id`, `job_id`, `run_id` (empty when the
attempt never reached the executor — that absence is what keeps it out
of `started` below), `domain` (`scheduled` / `case` / `manual`),
`strategy`, `resumed`, `outcome`, `elapsed_ms`, and the stage split
`admit_ms` (lease + suspension check + slot admission) / `prepare_ms`
(entity loads, prompt build, run log, tools) / `execute_ms` (the agent
loop) / `finish_ms` (terminal record + notifications + reflection) /
`reflect_ms` (reflection's share of `finish_ms`, broken out because it
is a whole extra agent pass).

Call aggregates: `llm_calls`, `llm_ms`, `tool_calls`, `tool_ms`, and
`tool_ms_by_name` (per-tool `Calls` / `DurationMs`). **`llm_ms` and
`tool_ms` are sums of per-call spans, so they can exceed `execute_ms`**
— `planexec` runs sub-agents in parallel and they share one trace
handler. Keys in `tool_ms_by_name` are restricted to the tool names the
model was actually offered; anything else (including a name from a lost
span) is bucketed under `unregistered`, so model output can never become
a log field name.

When the concurrency gate applies, the line also carries
`slot_observed` (slots held at the admission attempt), `slot_limit` and
`slot_hold_ms`. With the gate disabled the three are omitted rather than
reported as zero.

#### `job concurrency slot released` — one line per held slot

`slot_index`, `slot_limit`, `slot_hold_ms`. Emitted at release so a run
that dies before its own summary still records how long it occupied
capacity.

#### `job tick summary` — one line per sweep

Emitted by `ScheduledScanner.Scan` after it waits for the runs it
dispatched (15 minutes by default; a sweep that fails before finishing
its walk returns without a summary instead of waiting).

`due_total` counts the `(job, case)` pairs the sweep raised an event
for. `started` counts the attempts that reached the executor.
`completed` / `failed` / `suspended` / `skipped_slots_full` /
`skipped_lease` / `skipped_suspended` are always present, zero
included. `elapsed_ms` covers the whole sweep; `settled` is `false`
when the wait was given up on, which marks the counts as partial.

With the gate enabled the line adds `slot_limit`, `slot_busy_ms` (the
holds this instance took during this sweep) and `slot_idle_ms`
(`slot_limit × elapsed_ms − slot_busy_ms`, floored at zero).
**`slot_idle_ms` is an upper bound on the real idle time**: a hold left
over from an earlier sweep, or one taken by another instance, is not
subtracted. Capacity going unused shows up as a large `slot_idle_ms`
*together with* a large `skipped_slots_full` — a sweep that refused
almost everything while its slots sat free.

Only scheduled runs are counted. A Job agent that mutates its case
publishes a lifecycle event from inside the run; counting that run would
push the outcome totals past `due_total`.

#### Startup lines

`Agent Job runtime configured` reports `job_max_concurrency` from both
`serve` and `tick`. `job concurrency limiter enabled` reports the
compiled-in slot timings (`slot_ttl`, `slot_renew_interval`,
`slot_max_hold`) so a `slot_hold_ms` can be read against the TTL that
would have expired it; with the limit set to `0` the line is
`job concurrency limiter disabled` instead.

#### Per-call detail

`appended job run event` (DEBUG) carries `duration_ms` per LLM response
and tool call, plus `tool_name`. There is deliberately no INFO line per
call: one run makes tens to hundreds of them.

### Run logs and event trail

Each invocation of a Job (= one *Run*) writes a structured log so the
agent's behaviour can be reconstructed after the fact. The log is
designed for rough flow tracing — "what was asked, what did the model
say, which tools were called with what arguments" — not exact byte-for-
byte reproduction.

#### Web UI: per-Run detail page

Each Run has a detail page in the web console at
`/ws/{WorkspaceID}/cases/{CaseID}/agent/runs/{RunID}`, rendering the
metadata, system prompt, and the full event timeline. The page header
carries a **Download JSON** button that exports the entire record —
the `JobRunLog` metadata plus every `JobRunEvent` (in `Sequence` order,
with each `payload` decoded to nested JSON) — as a single
`jobrun-{CaseID}-{RunID}.json` file. The export runs entirely in the
browser from data already fetched for the page (no extra API call) and
mirrors the field names / shapes described below.

#### Identifiers

| ID | Scope | Generated by | Where it appears |
|----|-------|-------------|------------------|
| `RunID`   | One Run | `JobRunner.Run` (UUIDv7) | Doc ID of the `JobRunLog`; flat field on every `JobRunEvent`. |
| `TraceID` | One gollem trace | `JobRunner.Run` (UUIDv7) | Field on `JobRunLog` and every `JobRunEvent`. Logically distinct from `RunID` so a future plan-execute runtime can group multiple sub-agent traces under one Run. |

Both IDs are also mirrored onto the `JobRun` lock doc as `LastRunID` /
`LastTraceID` so a single read of the lock doc points at the latest log
without scanning the subcollection.

#### Firestore layout

```
workspaces/{WorkspaceID}/cases/{CaseID}/jobRuns/{JobID}                     ← lock doc + last-run summary
workspaces/{WorkspaceID}/cases/{CaseID}/jobRuns/{JobID}/logs/{RunID}        ← JobRunLog (one per Run)
workspaces/{WorkspaceID}/cases/{CaseID}/jobRuns/{JobID}/logs/{RunID}/events/{Sequence}
                                                                            ← JobRunEvent (one per LLM call or tool call)
jobSlots/{index}                                                            ← execution slot of the deployment-wide
                                                                              concurrency limit (deleted when released)
```

`Sequence` is a 20-digit zero-padded `uint64` so doc IDs sort
lexicographically the same way they sort numerically.

#### `JobRunLog` fields (per Run)

- Identifiers: `WorkspaceID`, `CaseID`, `JobID`, `RunID`, `TraceID`
  (all top-level scalars, BigQuery-friendly).
- Lifecycle: `Stage` (`RUNNING` / `SUCCESS` / `FAILED`), `StartedAt`,
  `EndedAt`, `Error`.
- Runtime: `ExecutorKind` (`"single_loop"` for `simple`, `"plan_execute"`
  for `planexec`), `ExecutorVersion`.
- Provenance: `EventType` (e.g. `case`, `scheduled`), `EventTriggerAt`.
- `SystemPrompt`: the full system prompt, truncated from the tail at
  ~800 KiB. Held once per Run rather than duplicated on every LLM call.

#### `JobRunEvent` kinds

Each event is one of:

| Kind            | When it appears |
|-----------------|-----------------|
| `LLM_REQUEST`   | Emitted at every LLM API call. Captures `Model`, the full message history sent, and the advertised tool list. |
| `LLM_RESPONSE`  | Paired with `LLM_REQUEST`. Captures `Texts`, `FunctionCalls`, `InputTokens`, `OutputTokens`, `CacheCreationInputTokens`, `CacheReadInputTokens`, `DurationMs`. `InputTokens` is the provider's total and already includes the two prompt-cache figures. |
| `TOOL_CALL`     | One per tool execution. `ParentSequence` points at the LLM_RESPONSE whose tool_use spawned it. Captures `ToolName`, `ArgumentsJSON`, `ResultJSON`, `IsError`, `ErrorMessage`, `StartedAt`, `EndedAt`. |
| `RUN_ERROR`     | Emitted by `JobRunner.Run` when the agent loop fails. Captures `Stage` (`prepare` / `execute` / `finish`) and `Message`. |

**`OccurredAt` is a completion timestamp, not a call start.** Both the
`LLM_REQUEST` and the `LLM_RESPONSE` of one call carry the same value —
the moment the handler observed the call finish — so the gap between the
two rows measures only how long the two writes took. The measured
latency is `LLM_RESPONSE.DurationMs`, and for a tool the difference
between `TOOL_CALL.StartedAt` and `EndedAt`. Deriving a call's duration
from `OccurredAt` will silently under-report it.

#### Truncation policy

Single text or JSON fields longer than `model.MaxInlineBytes` (800 KiB)
are silently truncated from the tail. There is no truncation flag — the
goal is "you can read what happened roughly", not exact reproduction.
If you need full fidelity for a particular Job, consider a custom trace
backend; the public `trace.Handler` interface is `gollem.WithTrace`-able
from any executor.

#### Strategy coverage

Both Job strategies populate the event timeline:

- `simple` (single-loop) records the one gollem agent's `LLM_REQUEST` /
  `LLM_RESPONSE` / `TOOL_CALL` events.
- `planexec` records events from **every** agent the run drives — the
  planner rounds, each parallel investigation sub-agent, the round-1
  direct reply (when taken), and the final-response synthesis — so a
  multi-step investigation shows its whole trail, not just a summary.
  (The Job's per-event handler is wired into all of them alongside the
  separate trace archive recorder; before this was wired, `planexec`
  Jobs showed an empty timeline despite succeeding.)

Two fields on every `JobRunEvent` carry attribution but are currently
coarse:

- `Phase` — always `"execute"` (or `"reflection"` for the optional
  post-run reflection pass). The finer `"plan"` / `"investigate"` /
  `"final"` labelling for `planexec` is not emitted yet.
- `AgentLabel` — always `""`. `planexec` sub-agents are independent
  root agents (not gollem-internal sub-agents), so the per-agent label
  hook does not fire; events are ordered by `Sequence` but not attributed
  to a named sub-agent.

Because `planexec` sub-agents run in parallel (up to the plan's per-phase
fan-out), a `TOOL_CALL`'s `ParentSequence` is best-effort under
concurrency — it points at the most recent `LLM_RESPONSE` the shared
handler observed, which may belong to a sibling sub-agent. The timeline
remains complete and `Sequence`-ordered; only the parent linkage and
per-agent attribution are approximate. `simple` Jobs are single-threaded
and therefore exact.

Combined with `JobRunLog.ExecutorKind`, downstream consumers can filter
by runtime without a Firestore schema change.

#### BigQuery export

The Firestore documents use Go field names directly (no `firestore:`
struct tags), so a Datastream / `gcloud firestore export` to BigQuery
produces tables with PascalCase columns: `SELECT * FROM events WHERE
WorkspaceID = 'X' AND CaseID = 42`, or `events JOIN logs USING (RunID)`.
Every record carries `WorkspaceID`, `CaseID`, `JobID`, `RunID`,
`TraceID` at the top level, so JOIN-friendly queries are flat.

### Operational tips

- Treat the `prompt` as the only place to encode Job-specific behaviour;
  everything else is fixed by the runtime.
- For high-frequency scheduled Jobs, set `every` to a value greater than
  your expected sweep cadence so the duration-since-last-run check absorbs
  scheduler jitter.
- Use the `JobRunRepository.List` API (over `workspaceID`) to surface
  per-Job state in an observability dashboard.

## `tick` scheduling

Scheduled Jobs are time-driven by an external sweep — Hecatoncheires does
not run its own internal cron. You must wire one of the two entry points
described in [Running scheduled Jobs](#running-scheduled-jobs) to a
scheduler:

- **`hecatoncheires tick`** — a one-shot CLI sweep. Run it from `cron`,
  GitHub Actions, a Kubernetes CronJob, or any external timer. The process
  exits once the sweep and every dispatched Job goroutine finish, so it is
  safe to invoke on a fixed interval. Flags are documented in the
  [CLI Reference](./cli.md).
- **`POST /hooks/tick`** — the HTTP entry point on the `serve` server, for
  push-style schedulers (Cloud Scheduler / Eventarc). It is
  **unauthenticated by design** — deploy it behind IAP, Cloud Run
  internal-only ingress, or private networking, and never expose it to the
  public internet. It responds `200` immediately and runs the sweep in a
  background goroutine.

Set each Job's `every` interval larger than your sweep cadence so the
duration-since-last-run check absorbs scheduler jitter. Overlapping ticks
are safe: the per-(workspace, case, job) lease silently skips a second
invocation that arrives while the first still holds the lease (see
[Concurrency](#concurrency)).

## `migrate` operations

The `migrate` command (alias: `m`) manages Firestore indexes. It targets a
Firestore project / database via `--firestore-project-id` and
`--firestore-database-id`; the full flag reference is in the
[CLI Reference](./cli.md).

Run `migrate` with `--dry-run` first to preview the migration changes
without applying them, then re-run without the flag to apply. Note the
project's standing policy that **Firestore index changes are avoided in
principle** — most features are designed to work against the existing
indexes, so a `migrate` run that wants to add an index should be reviewed
with the team before it is applied in production.

## `diagnosis` usage

The `diagnosis` command groups one-shot data inspection / repair jobs. Each
sub-subcommand is a self-contained job; the umbrella itself takes no flags.
The flag reference for each job is in the [CLI Reference](./cli.md).

### `diagnosis fix-unsent-action`

Re-posts Slack messages for Actions whose initial Slack post never reached
Slack. The job sweeps every workspace in the registry, finds Actions with an
empty `SlackMessageTS`, and replays the post via the unified
`ActionUseCase.PostSlackMessageToAction` entry point. Repeat runs are safe:
already-posted Actions are skipped.

The job logs a final summary line:

```
fix-unsent-action complete total=N fixed=X skipped=Y failed=Z
```

- `Total` — Actions found with an empty `SlackMessageTS`
- `Fixed` — Successfully posted; timestamp persisted
- `Skipped` — Documented skip conditions (parent Case has no Slack channel,
  the Action was already posted by a concurrent run, or the row was deleted
  during the sweep)
- `Failed` — Unexpected errors. Each is reported via `errutil.Handle` so it
  reaches the configured error sink (Sentry / log); the sweep continues
  past failures so a single bad row never blocks the rest

## Backup & data migration

Hecatoncheires keeps persistent state in two backends, and a backup plan
should cover both:

- **Firestore** — the system of record for domain data: Cases, Actions,
  action steps, Knowledge, Slack message linkage, agent Session metadata,
  and the agent Job run logs. Job run logs live under
  `workspaces/{WorkspaceID}/cases/{CaseID}/jobRuns/{JobID}/...` (see
  [Firestore layout](#firestore-layout)); agent Session metadata is keyed
  by Slack channel + thread TS under `slack_channels/{channelID}/sessions/{threadTS}`.
  Use `gcloud firestore export` for backups; the same export feeds the
  BigQuery analytics path described in [BigQuery export](#bigquery-export).
- **Cloud Storage** — the agent conversation History and execution Trace
  blobs (one per session / per turn), written by the Slack-mention agent.
  Object layout under the configured bucket:

  ```
  {prefix}/v1/sessions/{sessionID}/history.json
  {prefix}/v1/traces/{sessionID}/{traceID}.json
  ```

  The bucket and optional prefix are set via
  `HECATONCHEIRES_CLOUD_STORAGE_BUCKET` /
  `HECATONCHEIRES_CLOUD_STORAGE_PREFIX`. The service account needs
  **Storage Object Admin** on the bucket — `Storage Object Viewer` alone
  is insufficient because every LLM turn mutates objects. Back the bucket
  up with object versioning or a scheduled bucket copy.

For the full Cloud Storage object model, History/Trace formats, and the IAM
details, see [agent session implementation in the Architecture Guide](./develop/architecture.md)
and the [User Guide](./user_guide.md) for the user-facing session behaviour.
No new Firestore composite indexes are required for these lookups; they are
direct document fetches.

## See Also

- [CLI Reference](./cli.md) — flags and env vars for `serve`, `tick`, `migrate`, and `diagnosis`
- [Configuration Guide](./configuration.md) — workspace TOML and the `[[job]]` schema, system prompt, and per-case agent settings
- [Architecture Guide](./develop/architecture.md) — agent session internals, Cloud Storage object model, and dataloader design
