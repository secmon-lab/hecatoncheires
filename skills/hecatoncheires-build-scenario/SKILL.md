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

## Decisions to make

Author the scenario by making these decisions in order. For the exact TOML keys
of each table, follow `docs/eval.md` (eval tables) and `docs/configuration.md`
(workspace config) — do not reproduce the schema from memory.

1. **Workflow.** `thread_mode_initial` (a post creates + materializes a case) or
   `job` (a workspace Job runs against a seeded case). This dictates which
   trigger you author in step 3.
2. **Workspace config** — the system under test. Author it like a real config
   file.
3. **Trigger.** For `thread_mode_initial`, the first monitored-channel post.
   For `job`, the seeded target case plus the job selection.
4. **Simulated world.** What can each tool see, and which data sources exist?
   Describe only what the scenario needs. `hecatoncheires eval --list-tools`
   lists the valid tool names.
5. **Persona** — who answers the agent's clarifying questions, and what they
   know.
6. **Checklist** — the yes/no questions the judge evaluates. This is the heart
   of the scenario; see "Writing good checks" below.
7. **Validate** with `hecatoncheires eval --dryrun <file>` and fix every error
   before considering it done.

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

For a complete, annotated scenario `.toml` (every table filled in), see the
scenario schema example in `docs/eval.md`.
