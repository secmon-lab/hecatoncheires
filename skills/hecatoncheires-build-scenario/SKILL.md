---
name: hecatoncheires-build-scenario
description: Author an eval scenario TOML for `hecatoncheires eval`. Use when the user wants to create or edit an eval scenario for an LLM workflow (thread_mode_initial or job) — defining the workspace config, the trigger (a post, or a Job + seeded case), simulated tool data / sources, the answering persona, and the checklist the judge evaluates.
---

# Write an eval scenario

Produce a single self-contained scenario `.toml` for `hecatoncheires eval`. A
scenario carries the system-under-test workspace config (standard top-level
layout) plus the eval tables (`meta` / `input` / `cases` / `tools` / `persona`
/ `expect`).

This skill describes only the principles of authoring a scenario. It
deliberately does **not** restate the concrete config schema, because that
evolves and would drift out of sync. Always defer to the authoritative,
always-current sources:

- **Workspace config portion** (`[workspace]`, `[[fields]]`, field types, id
  formats, `[slack]`, `[case]`, `[[job]]`, …) → `docs/configuration.md`.
- **Eval tables** (`meta` / `input` / `cases` / `tools` / `persona` / `expect`)
  → `docs/eval.md`.

## Steps

1. **Pick the workflow.** Registered kinds: `thread_mode_initial` (a post
   creates+materializes a case) and `job` (a workspace Job runs
   against a seeded case). Set `meta.id` (unique, kebab-case) and
   `meta.workflow`. Run `hecatoncheires eval --list-tools` to see valid tool
   names.
   - For `job`: declare the Job in the workspace config (`[[job]]`),
     seed the target case under `[[cases]]`, and select it with `[run_job]`
     (`id`, optional `target_case`). No `[input]` is needed. Checks assert on the
     job outcome, the case after the run, and any actions the job created.
2. **Workspace config.** Author the top-level tables (`[workspace]`,
   `[[fields]]`, `[slack]`, `[case]`, …) exactly like a real workspace config
   file. Do **not** re-derive the field types, id formats, option/status-set
   layout, or Slack options from memory — follow `docs/configuration.md`, the
   single source of truth for the config schema. Eval-specific constraint only:
   for `thread_mode_initial` the `[slack]` table needs `mode = "thread"` and a
   real `channel` id (e.g. `C0123456789`).
3. **Input.** `[input].text` is the first monitored-channel post. Omit
   reporter/channel/thread_ts (synthesized) unless a specific reporter matters.
3b. **Sources (optional).** Add `[[sources]]` for the workspace's data sources
   (`type` = `notion_db` / `notion_page` / `slack` / `github`, each with its
   matching config block). These are seeded as real Source entities and read by
   source-aware tools / workflows. (Note: the `thread_mode_initial` materialize
   agent does not consult Sources, so they are inert for that workflow today.)
4. **Tools (optional).** For each search tool the agent might use, add
   `[tools.<name>]` with a natural-language `background` describing what data it
   can see. The ToolSimulator LLM generates responses from that. Use
   `live = true` only when you intend to hit the real API.
5. **Persona.** `[persona]` describes who answers the agent's clarifying
   questions and what they know (`description`, `knowledge`,
   `max_answer_turns`).
6. **Checks.** Add `[[expect.checks]]` items — atomic yes/no `question`s the
   judge answers against the produced case (and its transcript / tool calls).
   See "Writing good checks".
7. **Validate.** Run `hecatoncheires eval --dryrun <file>` and fix any errors
   before considering it done.

## Available tools

The authoritative, always-current list is `hecatoncheires eval --list-tools`.
At time of writing:

- `slack_search` — search Slack messages (read-only; sim + live)
- `notion_search` — search Notion pages (read-only; sim + live)
- `github_search` — search GitHub issues/PRs (read-only; **live-only** in v1)
- `webfetch` — fetch a URL and return its content as Markdown, screened for prompt injection (read-only; **live-only** in v1)

> Keep this list in sync: when an agent tool is added, update it here and in the
> `eval` tool catalog (see CLAUDE.md). When unsure, trust `--list-tools`.

## Writing good checks

- **Derive from real failures.** The best checks come from things the agent
  actually got wrong. Start from observed bad outputs, not imagination.
- **One yes/no per check.** Keep each `question` atomic (no "and"). Compound
  questions reintroduce ambiguity. Do not use numeric scores.
- **Balance positive and negative.** Include checks that should pass *and*
  checks for behavior that should NOT happen (e.g. "Did the agent avoid calling
  any write tool?").
- **Cover both axes.** Check the outcome (case title / description / fields /
  board status) *and* tool usage / trajectory (e.g. "Did it search Slack before
  materializing?").
- **Decidable from state.** Phrase field/status/tool checks so they are
  answerable from the explicit snapshot (e.g. "Is the 'severity' field set to
  'high'?").
- **Treat checks as code.** They are the spec of expected behavior — version
  them, and don't let the system-under-test author its own checks.

The final OK/NG is decided by a human reviewing the per-check verdicts; the
harness never auto-gates.

## Minimal example

Illustrative only — it shows how the eval tables sit alongside a workspace
config. The workspace-config portion (`[workspace]` / `[[fields]]` / `[slack]` /
`[case]`) follows `docs/configuration.md`; check there if anything below looks
out of date.

```toml
[meta]
id       = "thread-initial-login-issue"
workflow = "thread_mode_initial"
language = "en"

[workspace]
id   = "support"
name = "Support"
[[fields]]
id = "severity"
name = "Severity"
type = "select"
required = true
options = [ {id = "high", name = "High"}, {id = "low", name = "Low"} ]
[slack]
mode = "thread"
channel = "C0123456789"
[[case.status]]
id = "triage"
name = "Triage"
[[case.status]]
id = "done"
name = "Done"
[case]
initial = "triage"
closed = ["done"]

[input]
text = "Cannot log in to the portal, 503 since this morning."

[tools.slack_search]
background = "#incidents has two threads about portal 503 since this morning."

[persona]
description = "A non-technical employee."
knowledge = "The error started at 9am; web portal only; severity feels high."
max_answer_turns = 2

[[expect.checks]]
id = "title-identifies-login-failure"
question = "Does the case title concisely identify a portal login failure?"
[[expect.checks]]
id = "severity-high"
question = "Is the 'severity' field set to 'high'?"
```
