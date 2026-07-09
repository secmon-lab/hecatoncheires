# Eval Harness (`hecatoncheires eval`)

The eval harness runs LLM-based agent workflows against pre-defined **scenario
files** and evaluates the produced artifact with an LLM-as-a-judge checklist.

Supported workflows (`meta.workflow`):

- **`thread_mode_initial`** — a monitored-channel post creates a case, the agent
  investigates and materializes it, and the resulting case is judged.
- **`job`** — a workspace Job (`[[job]]`) runs against a seeded case
  through the real JobRunner (simple / planexec strategy), and the job's outcome
  (success/failure + summary), the case state after the run, the actions it
  created, and its tool-call trajectory are judged.

The harness is designed to grow: new workflow types plug in by registering a
driver, and the scenario schema / judging model stay the same.

> **The final OK/NG decision is a human's.** The judge only produces per-check
> verdicts (pass/fail + reason) and an informational pass ratio. The harness
> never auto-gates on check results; it exits non-zero only on execution
> errors. For failing scenarios it writes a diagnostic dump you can hand to a
> separate session to analyze "what to fix".

## Quick start

```sh
# Validate scenario files (no LLM / tools / network):
hecatoncheires eval --dryrun scenarios/

# Run scenarios (requires an LLM provider, shared by all roles):
hecatoncheires eval \
  --llm-provider=claude --llm-claude-api-key=... \
  --report out.json \
  scenarios/

# List the tools usable in scenarios:
hecatoncheires eval --list-tools
```

`eval` accepts one or more **files or directories** (directories are scanned
recursively for `*.toml`).

## Flags

| Flag | Description |
|------|-------------|
| `--dryrun` | Validate scenario files only; no LLM/tools/network. |
| `--report <file>` | Write a machine-readable JSON report. |
| `--concurrency <n>` | Scenarios run in parallel (default 2). |
| `--quiet` | One-line-per-scenario summary. |
| `--verbose` | Expand transcript / tool-call detail. |
| `--dump-dir <dir>` | Diagnostic dump root (default `tmp/eval`). |
| `--dump-all` | Dump every scenario, not just those with failing checks. |
| `--lang <en\|ja>` | Output language for judge reasons / `analysis.md` (default from `HECATONCHEIRES_DEFAULT_LANG`). Distinct from the agent's conversation language (`meta.language`). |
| `--list-tools` | Print the tool catalog and exit. |

LLM / Slack / GitHub / Notion credentials reuse the **same flags and
environment variables as `serve`** (`--llm-*` / `HECATONCHEIRES_LLM_*`,
`--slack-user-oauth-token`, `--github-*`, `--notion-api-token`). A single LLM
client is shared by the agent, the judge, and the simulators.

Exit codes: `0` on completed runs (regardless of check verdicts), `2` on an
execution error (parse failure, LLM/tool error, etc.).

## Scenario schema

A scenario file is a single TOML file that contains **both** the
system-under-test workspace configuration (authored with the same top-level
layout as a normal workspace config file) **and** the eval-specific tables. The
workspace part is extracted by the existing config loader, which ignores the
eval-only keys.

```toml
[meta]
id          = "thread-initial-login-issue"   # required, unique
description = "Portal login 503 should be materialized into a case"
workflow    = "thread_mode_initial"          # required, a registered driver kind
language    = "en"                            # optional: the agent's conversation language

# --- system-under-test workspace (standard workspace-config layout) ---
[workspace]
id   = "support"
name = "Support"

[[fields]]
id       = "severity"
name     = "Severity"
type     = "select"
required = true
options  = [ {id = "high", name = "High"}, {id = "low", name = "Low"} ]

[slack]
mode    = "thread"
channel = "C0123456789"      # monitored channel (must be a Slack channel ID)

[[case.status]]
id   = "triage"
name = "Triage"
[[case.status]]
id   = "done"
name = "Done"
[case]
initial = "triage"
closed  = ["done"]

# --- eval input: the first top-level post that triggers case creation ---
[input]
text = "Cannot log in to the portal, I keep getting a 503 error since this morning."
# reporter / channel / thread_ts are synthesized; set `reporter` only if a
# specific posting user matters.

# --- optional: prior cases seeded into the memory repository (future search) ---
[[cases]]
title        = "Portal outage 2026-05"
board_status = "done"
  [cases.fields]
  severity = "high"

# --- optional: workspace data sources seeded into the repo ---
# Real Source entities (repo.Source().Create), readable by source-aware tools
# (e.g. the workspace-metadata tool) and source-consuming workflows (Jobs).
# Exactly one type config block must match `type`.
[[sources]]
name = "Incident runbooks"
type = "notion_db"            # notion_db | notion_page | slack | github
description = "Runbook database"
enabled = true               # default true
  [sources.notion_db]
  database_id    = "11112222333344445555666677778888"
  database_title = "Runbooks"
  database_url   = "https://notion.so/runbooks"
[[sources]]
name = "Incident channel"
type = "slack"
  [sources.slack]
  channels = [ {id = "C0123456789", name = "incidents"} ]
# notion_page: [sources.notion_page] page_id=... [recursive, max_depth]
# github:      [sources.github] repositories = [ {owner="o", repo="r"} ]

# --- tool behavior (optional). Default: simulated. ---
# Describe the data a tool can see in `background`; when the agent calls it,
# the ToolSimulator LLM generates a realistic response. Set `live = true` to
# hit the real API instead (background ignored). Tools with no background in
# sim mode return nothing.
[tools.slack_search]
background = """
#incidents has two threads reporting portal 503 errors since this morning.
"""
# [tools.github_search]
# live = true

# --- persona: who answers the agent's clarifying questions, and what they know ---
[persona]
description      = "A non-technical employee reporting an issue."
knowledge        = """
- The error started around 9:00 this morning.
- It affects the web portal; the mobile app works fine.
"""
max_answer_turns = 3       # cap on the question/answer loop

# --- expectations: a checklist of yes/no questions the judge evaluates ---
# Each check's verdict + reason and a pass ratio are reported; the human
# decides the final OK/NG. Checks can target the case content OR tool usage.
[[expect.checks]]
id       = "title-identifies-login-failure"
question = "Does the case title concisely identify that this is a portal login failure?"

[[expect.checks]]
id       = "severity-high"
question = "Is the 'severity' field set to 'high'?"

[[expect.checks]]
id       = "searched-for-context"
question = "Did the agent call a search tool to gather context before materializing the case?"
```

## `job` workflow

For evaluating a Job, the scenario:

- declares the Job in the workspace config (`[[job]]`, the normal job layout),
- seeds the target case under `[[cases]]`,
- selects which job to run with `[run_job]` (the key is `run_job`, not `job`, to
  avoid colliding with the workspace's `[[job]]` array):

```toml
[[job]]
id       = "triage_summary"
name     = "Triage summary"
prompt   = "Summarize the case titled {{.Case.Title}} and note next steps."
strategy = "simple"            # simple | planexec
  [job.events.case]
  on = ["created"]

[[cases]]
title       = "Cannot log in to portal (503)"
description = "Users report 503 on login since this morning."

[run_job]
id          = "triage_summary"
target_case = "Cannot log in to portal (503)"   # optional; defaults to the first seeded case

[[expect.checks]]
id       = "job-succeeded"
question = "Did the job run complete successfully (outcome SUCCESS)?"
[[expect.checks]]
id       = "created-followup-action"
question = "Did the job create a follow-up action for the login failure?"
```

The job runs through the real `JobRunner` with read-only + action-writer tools
wired, so checks can assert on the run outcome, the resulting case, and any
actions the job created. Seeded `[[sources]]` reach the job via its system
prompt (`resolveSources`). (v1 omits the case-field-writer and slack-post tools
from the eval job tool set — action creation is the primary observable.)

## Tools

`eval --list-tools` prints the catalog. v1:

| Tool | Mode | Notes |
|------|------|-------|
| `slack_search` | sim + live | |
| `notion_search` | sim + live | |
| `github_search` | **live-only** | simulating it needs a production interface extraction, deferred |
| `jira_search` | **live-only** | wraps the external gollem-dev/tools/jira ToolSet, which has no simulatable interface seam either |
| `webfetch` | **live-only** | real HTTP GET + LLM injection screening; the eval LLM does the screening |
| `knowledge__create_tag` | sim | Create a knowledge tag; must call `knowledge__list_tags` first to avoid duplicates. Returns the new tag id. |
| `knowledge__update_tag` | sim | Rename an existing knowledge tag by id. |
| `knowledge__delete_tag` | sim | Delete a knowledge tag by id; succeeds only when no knowledge entry references it. |

`sim` tools generate responses from their `background`; `live` tools call the
real API (require the matching credentials). Every tool call — sim or live — is
recorded so checks can verify tool usage and dumps can show the trajectory.

The catalog enumerates **external-service** tools and **knowledge tag management**
tools the harness can simulate or declare. Other in-process, always-present tools
that operate on the local repository — `core__*` (actions), `memo__*` (memos), and
the remaining `knowledge__*` tools (search, get, list_tags, create_knowledge,
update_knowledge, which now accept/return `tag_ids` referencing first-class Tag
entities) — are not listed in the catalog and are not scenario-configurable; the
eval agent is wired with them directly (knowledge write is withheld for private
cases, as in production).

## Diagnostic dumps

Scenarios with a failing check (or all scenarios with `--dump-all`) are dumped
to `<dump-dir>/<scenario-id>/<eval-id>/`:

```
trace.json     planner / sub-agent / tool execution trace
run.json       transcript, tool calls (with results), final case, verdicts
analysis.md    a ready-to-hand-off analysis request for a separate session
```

`eval-id` is a UUIDv7 per run (also in the summary and JSON report), so reruns
keep history. Hand `analysis.md` to a separate session to get a "what to fix"
analysis.

## Authoring scenarios

Use the `hecatoncheires-build-scenario` skill to author scenarios
interactively; it knows this schema and validates the result with
`eval --dryrun`. The skill is published through the repo's plugin marketplace
(`.claude-plugin/marketplace.json`), so it can be installed via
`/plugin install hecatoncheires-build-scenario@hecatoncheires` after adding this
repo as a marketplace. Good checks come from **real failures** — write atomic yes/no
questions, balance positive and negative cases, and cover both the case content
and tool usage.

## Design notes & limitations

- Binary checklist (no 1–5 score), outcome-graded with the trajectory captured
  for verification/diagnosis, one judge call per scenario, simulated user for
  multi-turn — these follow current LLM-eval practice.
- The judge shares the agent's LLM provider (simplicity), so self-preference
  bias is a known limitation.
- Not yet implemented (structured to add later): G-Eval-style evidence-first
  judging, repeated runs with `pass^k` / variance, judge-vs-human calibration,
  and per-check isolated judge calls.
