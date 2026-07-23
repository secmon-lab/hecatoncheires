# BigQuery Export

The `export` subcommand writes the current data of every configured workspace to
BigQuery for analysis. Each run is a **full refresh** ("洗替"): every table is
truncated and reloaded with the latest snapshot, so BigQuery always mirrors the
current state rather than accumulating history.

- One **BigQuery dataset per workspace** (schemas differ per workspace because
  custom fields differ, and dataset-level IAM keeps access separated).
- One **table per entity** within each dataset: `cases`, `actions`, `memos`,
  `knowledge`, `tags`.
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
| `knowledge` | Knowledge | workspace-level; embedding vector excluded |
| `tags` | Tags | workspace-level |

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
