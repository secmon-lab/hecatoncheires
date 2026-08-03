# BigQuery Export

The `export` subcommand writes the current data of every configured workspace to
BigQuery for analysis. Each run is a **full refresh** ("洗替"): every table is
truncated and reloaded with the latest snapshot, so BigQuery always mirrors the
current state rather than accumulating history.

- One **BigQuery dataset per workspace** (schemas differ per workspace because
  custom fields differ, and dataset-level IAM keeps access separated).
- One **table per entity** within each dataset: `cases`, `actions`, `memos`,
  `job_runs`, `job_run_logs`, `job_run_events`, `knowledge`, `tags`.
- Per-workspace **custom fields** are expanded into typed `field_<id>` columns.
- Schema changes are detected and applied **in place** (columns are added; the
  table is never dropped/recreated), so downstream views and column ACLs
  survive.

Writes use the BigQuery **Storage Write API** (`managedwriter`, pending stream).

## Usage

```
hecatoncheires export \
  --global-config ./global.toml \
  --config ./workspaces/ \
  --repository-backend firestore \
  --firestore-project-id my-firestore-project
```

The command reads existing flags only — there are **no new CLI flags or
environment variables**. The BigQuery destination and the workspace→dataset
mapping live in the `[export]` section of the `--global-config` file. Run it once
(e.g. from a scheduled job) to refresh every configured workspace.

Authentication uses Application Default Credentials (ADC), like the Firestore /
Cloud Storage integrations.

## Configuration (`[export]` in the global config)

The `[export]` section goes in a file passed via `--global-config`
(`HECATONCHEIRES_GLOBAL_CONFIG`) — the same deployment-wide file that holds
`[[workspace_group]]`. Only one global-config file may declare `[export]`.

```toml
[export]
# Default privacy policy for every workspace: whether to ALSO export private
# Cases (and their Actions / Memos). Default: false — private data is NOT
# exported unless you opt in. A workspace may override it (see below).
include_private = false

[export.bigquery]
# Destination GCP project (required).
project = "my-bq-project"
# BigQuery location, used only when a dataset must be created (optional).
location = "asia-northeast1"

# One mapping per workspace to export. A workspace not listed here is skipped.
# BigQuery dataset names forbid hyphens, so the dataset name is given explicitly
# rather than derived from the workspace id.
[[export.bigquery.workspace]]
id      = "sec-risk"          # workspace id (must exist in --config)
dataset = "hecato_sec_risk"   # BigQuery dataset name ([A-Za-z0-9_], <= 1024 chars)

[[export.bigquery.workspace]]
id      = "task-mgmt"
dataset = "hecato_task_mgmt"
include_private = true        # per-workspace override of the [export] default
```

`include_private` resolves per workspace: a mapping's own `include_private` wins
when set, otherwise the `[export].include_private` default applies (which itself
defaults to `false`). So private Cases are excluded everywhere unless a scope
explicitly opts in.

Validation (fails fast at startup): the BigQuery `project` must be set, every
`id` must exist in the workspace registry, each `dataset` must match
`^[A-Za-z0-9_]+$` and be at most 1024 characters, and both the ids and the
dataset names must be unique.

## What is exported

| Table | Source | Notes |
|-------|--------|-------|
| `cases` | non-draft Cases | drafts excluded; `field_<id>` per workspace field; private cases excluded unless `include_private` |
| `actions` | Actions (archived included) | only actions whose parent Case is exported |
| `memos` | Memos (archived included) | `field_<id>` per workspace memo field; only memos of exported cases |
| `job_runs` | Latest run state per (case, job) | only runs of exported cases |
| `job_run_logs` | One row per agent run against a case | only runs of exported cases; includes mention-triggered runs; carries the run's system prompt and its token / step totals |
| `job_run_events` | One row per LLM call, tool execution or run error | the full timeline of every exported run, payload bodies included |
| `knowledge` | Knowledge | workspace-level; embedding vector excluded |
| `tags` | Tags | workspace-level |

### Agent run tables

The three `job_run*` tables are Case-scoped and follow the same privacy rule as
Actions and Memos: a Case excluded from the export (a draft, or a private Case
when `include_private` is off) contributes no rows to any of them, so its
prompts, tool results, errors and token counts never reach BigQuery.

The keys nest:

- `job_runs` — `(workspace_id, case_id, job_id)`
- `job_run_logs` — `(workspace_id, case_id, job_id, run_id)`
- `job_run_events` — `(workspace_id, case_id, job_id, run_id, event_id)`

`job_run_logs` is not limited to TOML-configured Jobs — every case-scoped agent
run lands there, including Slack mentions handled by the case agent. The
`event_type` column discriminates them: mention runs carry `mention` (and a fresh
per-turn `job_id`), while Job runs carry their triggering event domain (`case`,
`scheduled`, `manual`).

#### Run totals on `job_run_logs`

`input_tokens` / `output_tokens` / `llm_call_count` / `tool_call_count` are the
run's totals across everything it did — planner, sub-agents and the reflection
pass included — accumulated while the run executes and stored on the run record,
so cost and size are available without touching `job_run_events`. A run's step
count is `llm_call_count + tool_call_count`. Interactive runs that paused for a
question accumulate across both turns. **Runs that finished before these columns
existed report zero; there is no backfill.**

Note that `input_tokens` and `output_tokens` also exist on `job_run_events`, where
they are one call's figures rather than the run's total. Do not sum the
`job_run_logs` columns across a run's events, or compare the two without
qualifying which table you mean.

`llm_call_count` counts every attempt to reach the model, including one that
failed before any response arrived (a provider that cannot open its stream reports
no call data). Such an attempt produces no `job_run_events` row, so
`llm_call_count` can exceed the number of `LLM_RESPONSE` events for the same run.
`tool_call_count` counts completed tool executions, successful and failed alike;
a tool that never returned is not counted.

#### The `job_run_events` timeline

One flat table holds all four event kinds so a run reads back as a single ordered
scan. `sequence` is the authoritative order within a run (document ids may
diverge under clock skew). Only the columns belonging to a row's `kind` are
populated; the rest are NULL.

| Columns | Populated for |
|---------|---------------|
| `model`, `messages_json`, `tools_json` | `LLM_REQUEST` |
| `model`, `texts_json`, `function_calls_json`, `input_tokens`, `output_tokens`, `duration_ms` | `LLM_RESPONSE` |
| `tool_name`, `tool_arguments_json`, `tool_result_json`, `tool_is_error`, `tool_error_message`, `tool_started_at`, `tool_ended_at` | `TOOL_CALL` |
| `error_stage`, `error_message` | `RUN_ERROR` |

**This table carries the agents' full conversation and tool payloads**, so it is
by far the largest thing the export writes, and it grows superlinearly per run:
each `LLM_REQUEST` holds the whole conversation as of that call, so a run with N
LLM calls stores roughly N²/2 messages. Every export is a full refresh, so all of
it is re-read from Firestore and re-written each time. Individual payload fields
are capped at 800 KiB by the trace layer (longer values are truncated from the
tail before they are persisted).

Reading it costs one Firestore query per run, on top of one subcollection scan
per case and one log query per (case, job) pair. Mention runs each get their own
`job_id`, so a busy Case adds one of each per mention turn. The queries run
serially.

`pending_interaction` (the question form of a run sitting at `AWAITING_INPUT`) is
deliberately not exported: it is transient state with no scalar representation.

#### Limits of the timeline

Three things the timeline does not tell you. None of them is a property of the
export — each is inherited from how the run was recorded.

**`parent_sequence` and `agent_label` are unreliable while sub-agents run in
parallel.** A `plan_execute` run drives several sub-agents concurrently through
one trace handler that keeps a single "most recent response" and a single active
label. When two sub-agents interleave, a `TOOL_CALL` can be attributed to another
agent's `LLM_RESPONSE`, and an event can carry a neighbouring agent's
`agent_label` (or none). Treat both columns as hints on parallel runs; `sequence`
and `run_id` remain exact.

**An empty payload is indistinguishable from an unrecorded one.** Both repository
backends decode a stored empty array back into a nil slice, so a response that
returned no text and a response whose text was never recorded both arrive as NULL
in `texts_json`. The event row and its scalar columns still identify the call.

**A run whose summary document was never written is missing entirely.** The export
walks `job_runs` first and reaches logs and events through it. A mention-triggered
run writes its log and events before its summary, so if that final write fails the
run is durably stored in Firestore yet never exported. Configured Jobs are not
affected: they create the summary when taking the run lease, before the log
exists.

Custom field column types: `text` / `markdown` / `url` / `select` / `user` /
`case_ref` → `STRING`; `number` → `FLOAT64`; `multi-*` → `ARRAY<STRING>`;
`date` → `STRING` (stored dates are a heterogeneous mix of RFC3339 and
`YYYY-MM-DD`, kept verbatim rather than forced into one temporal type).

## Full-refresh semantics (important)

The Storage Write API is append-only and has no truncate mode, so each table is
refreshed as `TRUNCATE TABLE` followed by an append. These are two separate
operations, so during a refresh there is a brief window where the table is empty
or partially written — **the refresh is not atomic**. If the append fails after
the truncate, the table can be left empty until the next run. For a periodic
analytics export this is an accepted trade-off (chosen for simplicity over a
staging-table swap).

**Run `export` as a single instance at a time.** Because the refresh is not
atomic, two overlapping `export` runs against the same dataset can interleave
their truncate/append and leave duplicated rows (both snapshots committed). The
command is intended to be run singly (a manual invocation or a scheduled job with
no overlap); it does **not** take a distributed lock. If you schedule it, ensure
the schedule interval exceeds the run time, or gate it so a new run does not start
while the previous one is still going.

Schema evolution note: after a schema change (a new custom field), the Storage
Write backend can take several minutes to accept the new columns; the export
retries the append under a bounded backoff (up to 15 minutes) to absorb that
delay.

## Required IAM

The identity running `export` needs, on the destination project/datasets, roughly
`roles/bigquery.dataEditor` plus `roles/bigquery.jobUser`:

- create datasets / tables (`bigquery.datasets.create`, `bigquery.tables.create`)
- update table schemas (`bigquery.tables.update`)
- run jobs and read/write table data — including the Storage Write API and the
  `TRUNCATE` query (`bigquery.jobs.create`, `bigquery.tables.getData`,
  `bigquery.tables.updateData`)

## Live tests

The end-to-end tests write to a real BigQuery dataset and are gated on
environment variables (skipped when unset). Point them at a throwaway dataset —
the tests create and drop their own tables.

```
TEST_BIGQUERY_PROJECT_ID   # gate; the destination project
TEST_BIGQUERY_DATASET_ID   # a dedicated test dataset (tables are created/dropped)
TEST_BIGQUERY_LOCATION     # optional; used only if a dataset must be created
```

Run:

```
TEST_BIGQUERY_PROJECT_ID=my-proj TEST_BIGQUERY_DATASET_ID=export_test \
  go test ./pkg/usecase/export/...
```

## Future

A Cloud Storage sink is anticipated: the exporter writes through a generic
`Sink` interface, so a second sink can be added without touching the
read/normalize logic. Only BigQuery is implemented today.
