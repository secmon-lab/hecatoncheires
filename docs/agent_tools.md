# Agent Tools

This is the **single source of truth** for the tools the LLM agent can call,
and — just as important — **which tools are available in which runtime
context**. If you are writing a Job prompt, an `[assist]` prompt, or a per-case
agent note and need to name a tool, confirm it here first. A tool referenced in
a prompt must be one that is actually wired into that prompt's context;
naming a tool the context does not expose silently does nothing.

> Quick map for prompt authors:
> - **Naming a tool in a Job prompt?** Read [Tools available by context](#tools-available-by-context) — Jobs get a *narrower* palette than the interactive mention agent (no Slack search, no Notion, no GitHub).
> - **Wondering whether a Job may close / delete / post anywhere?** Read [Guardrails](#guardrails).
> - **Wiring an external integration (Notion / GitHub) on / off?** See [Integrations](integrations.md); the tools light up automatically when the service is configured.

The names below are exactly what the LLM sees (e.g. `case__update_case`). They
are grouped by the package that defines them under `pkg/agent/tool/`.

## Tool catalogue

### Core / Action tools (`core`, `actionwriter`)

Manage the case's Action items (Kanban work items). Read tools are available
wherever the case has Actions; write tools split into a full set (interactive
mention agent) and a Job-safe subset.

| Tool | R/W | Purpose | Notes |
|------|-----|---------|-------|
| `core__list_actions` | R | List the case's actions. | Optional `include_archived` (default `false`). |
| `core__get_action` | R | Fetch one action by id. | |
| `core__list_action_steps` | R | List the binary-state steps under an action. | |
| `core__search_referenceable_cases` | R | Search the target workspace of a `case_ref` / `multi_case_ref` field for a case id to reference. | Only wired when the workspace defines such a field. Private and draft cases are excluded. |
| `core__get_referenceable_cases` | R | Batch-fetch full details of referenced cases. | Same gating as above. Set the value itself via `case__update_case`'s `fields`. |
| `core__create_action` | W | Create a new action. | |
| `core__update_action` | W | Update an action's title / description / assignee. | Status changes go through `core__update_action_status`. |
| `core__update_action_status` | W | Move an action to another status. | Action status is independent of case status; this is **not** a case close. |
| `core__set_action_assignee` | W | Set / clear the action assignee (empty string clears). | |
| `core__add_action_step` | W | Add a step to an action. | |
| `core__set_action_step_done` | W | Mark a step done / undone (idempotent). | |
| `core__rename_action_step` | W | Rename a step. | |
| `core__archive_action` | W | Archive an action (hidden from views, recoverable). | **Interactive mention agent only — NOT exposed to Jobs.** |
| `core__unarchive_action` | W | Restore an archived action. | **Interactive mention agent only — NOT exposed to Jobs.** |
| `core__delete_action_step` | W | Delete a step. | **Interactive mention agent only — NOT exposed to Jobs.** |

There is no destructive "delete action" / "delete case" tool anywhere; the
archive lifecycle replaces deletion.

### Case writer tools (`casewriter`)

Edit the case the agent is bound to. Wired for the case-bound mention agent and
for Jobs (both channel- and thread-mode).

| Tool | R/W | Purpose | Notes |
|------|-----|---------|-------|
| `case__update_case` | W | Update title / description / custom field values. | Title and description are **full replacements** — review current values (shown in the system prompt) before overwriting. Cannot change status or assignees. Unknown field ids and type / option mismatches are rejected with a correctable error. |
| `case__assign` | W | Add assignee(s) by delta (set union). | Rejects user ids absent from the SlackUser store. |
| `case__unassign` | W | Remove assignee(s) by delta (set difference). | Does **not** reject unknown ids (a since-deleted user must stay removable). Applied atomically server-side, so concurrent edits never clobber. |
| `case__update_case_status` | W | Move the case to another board status (a closed status closes the case). | Only present for workspaces with a configured case status set (`CaseStatusSet`). The parameter enumerates the configured status ids. **See [Guardrails](#guardrails): Jobs are instructed not to close; the human-driven mention agent may.** |

### Slack tools (`slack`, `slackpost`)

| Tool | R/W | Purpose | Notes |
|------|-----|---------|-------|
| `slack__search_messages` | R | Workspace-wide message search (`search.messages`). | Requires a Slack **User** OAuth token with `search:read`. See [slack.md](slack.md#user-token-scopes). Interactive / investigation contexts only — **not** wired into Jobs. |
| `slack__get_messages` | R | Bulk-fetch 1–10 messages with thread context (parallel, partial failure tolerated). | Interactive / investigation contexts only — **not** wired into Jobs. |
| `slack__post_message` | W | Post a message to the case's Slack channel (supports `thread_ts`). | Used by the assist / mention flow, where the agent posts where it directs. Not suppressed by a Job's `quiet`. |
| `slack__post_to_case_channel` | W | Post a message to the case's bound channel. | **The only Slack tool a Job gets.** The channel id is hard-pinned to `Case.SlackChannelID`; arbitrary channels are not reachable. Wired only when a Slack service is configured and the case has a bound channel. |

### Knowledge tools (`knowledge`)

Workspace-wide shared Knowledge (semantic + keyword searchable). Read tools are
always offered; write tools are gated (see notes).

| Tool | R/W | Purpose | Notes |
|------|-----|---------|-------|
| `knowledge__search_knowledge` | R | Search workspace knowledge (semantic + keyword, optional tag filter). | |
| `knowledge__get_knowledge` | R | Fetch a knowledge entry (title, Markdown claim, tags) by id. | |
| `knowledge__list_tags` | R | List the distinct tags in use. | |
| `knowledge__create_knowledge` | W | Create a knowledge entry (Markdown claim, ≥ 1 tag). | Write is **withheld while the agent runs against a PRIVATE case**, so private contents cannot leak into shared knowledge. |
| `knowledge__update_knowledge` | W | Update a knowledge entry's title / claim / tags (omit to preserve). | Same private-case gating. |

### Memo tools (`memo`)

Per-case memos. Wired only when the workspace enabled memos
(`[memo]` with `enabled = true`).

| Tool | R/W | Purpose |
|------|-----|---------|
| `memo__list_memos` | R | List the case's memos (optional archive scope, default ACTIVE). |
| `memo__get_memo` | R | Fetch a memo by id. |
| `memo__create_memo` | W | Create a memo (title + field values). |
| `memo__update_memo` | W | Update a memo's title / fields (omit to preserve). |
| `memo__archive_memo` | W | Archive a memo (soft-delete, recoverable). |

### Notion tools (`notion`)

Wired when `HECATONCHEIRES_NOTION_API_TOKEN` is set. See [integrations.md](integrations.md).
Investigation / interactive contexts only — **not** wired into Jobs.

| Tool | R/W | Purpose |
|------|-----|---------|
| `notion__search` | R | Search Notion pages and databases shared with the integration (title match). |
| `notion__get_page` | R | Retrieve a page's content as Notion-flavored Markdown. |

### GitHub tools (`github`)

Wired when the GitHub App flags are set. See [integrations.md](integrations.md).
Investigation / interactive contexts only — **not** wired into Jobs.

| Tool | R/W | Purpose |
|------|-----|---------|
| `github__search` | R | Search issues / PRs with GitHub search syntax (`repo:`, `is:open`, `author:`, `label:`, …). Up to 50 hits. |
| `github__get_issue` | R | Fetch an issue (not a PR) with body, labels, and comments. |
| `github__get_pull_request` | R | Fetch a PR with body, comments, reviews; optional `include_files=true` adds the diff. |
| `github__get_file` | R | Fetch a file's content at any ref. UTF-8 text only; capped at 1 MB. |
| `github__list_commits` | R | List commits with optional `path` / `author` / `since` / `until` filters. |

### Web fetch tool (`webfetch`)

| Tool | R/W | Purpose | Notes |
|------|-----|---------|-------|
| `webfetch` | R | Fetch an HTTP(S) URL and return it as Markdown. | Blocks non-public IPs and screens the result for indirect prompt injection before returning it. Wired when a web-fetch client is configured. |

### Planner metadata tools (`wsmeta`)

Used **only** by the proposal (case-draft) planner — not by Jobs, the mention
agent, or sub-agents. Listed here for completeness.

| Tool | R/W | Purpose |
|------|-----|---------|
| `list_workspaces` | R | List id / name / description of all registered workspaces. |
| `get_workspace` | R | Fetch a workspace's identity, full field schema (with option metadata), and sources. The planner must call this before materialising a case so it uses exact field / option ids. |

## Tools available by context

The agent runs in several contexts, and **each wires a different subset**. This
matrix is the answer to "can my Job call `slack__get_messages`?" (no) or "can
the mention agent close a case?" (yes — see [Guardrails](#guardrails)).

| Tool group | Mention agent (channel-mode case) | Job — channel-mode | Job — thread-mode | Thread-case investigation | Proposal sub-agent (case draft) |
|------------|:---:|:---:|:---:|:---:|:---:|
| `core` read + Actions (`actionwriter`) | ✓ (full, incl. archive / delete-step) | ✓ (Job subset, no archive / delete-step) | — (thread mode has no Actions) | — | read-only `core__list_actions` / `core__get_action` |
| `case__*` (casewriter) | ✓ | ✓ | ✓ | — (decisions applied by the host) | — |
| `slack__search_messages`, `slack__get_messages` (read) | ✓ | — | — | ✓ | ✓ |
| `slack__post_to_case_channel` | — | ✓ | ✓ | — | — |
| `notion__*` | ✓ | — | — | ✓ | ✓ |
| `github__*` | ✓ | — | — | ✓ | ✓ |
| `webfetch` | ✓ | ✓ | ✓ | ✓ | ✓ |
| `knowledge__*` | ✓ (write if case is non-private) | ✓ (write if case is non-private) | ✓ (write if case is non-private) | read-only | read-only |
| `memo__*` | ✓ (if memos enabled) | ✓ (if memos enabled) | ✓ (if memos enabled) | — | — |
| `wsmeta` | — | — | — | — | planner only |

Notes:

- **"Mention agent"** is the case-bound agent that runs when a human @-mentions
  the bot in a channel-mode case channel (`pkg/usecase/agent/casebound`). It
  gets the widest palette because a human is in the loop.
- **Jobs** (`[[job]]`, both `simple` and `planexec` strategy) run **unattended**
  and get a deliberately narrower palette: case + action writes, knowledge,
  memo, web fetch, and a single channel-pinned Slack *post* tool — but **no
  Slack search, Notion, or GitHub read tools**. A Job that needs to reason about
  external context must have that context already in the case, not fetch it
  live. (Source of truth for Job wiring: `buildJobTools` in
  `pkg/cli/job_runtime.go`.)
- **Thread-mode** workspaces have no Actions, so the whole `core` / Action
  surface is absent there.
- **Thread-case investigation** and the **proposal sub-agents** are read-only
  investigators; their conclusions are applied by the host, not by a write tool.

## Guardrails

Some restrictions are enforced in **code** (the tool simply isn't wired, so the
agent cannot call it) and some are enforced only by the **system prompt** (the
tool exists, but the agent is instructed not to use it a certain way). Knowing
which is which matters: a prompt-only guardrail is a strong instruction, not a
hard lock.

| Restriction | Applies to | Enforcement |
|-------------|-----------|-------------|
| **A Job will not close a case.** Closing a case is a human-only decision. | Jobs (both strategies) | **Prompt only.** `case__update_case_status` can technically set any configured status, including a closed one; the Job system prompt instructs the agent not to. The interactive mention agent (human-initiated) *may* move a case to a closed status. |
| **No deleting cases.** | All agent contexts | No delete tool exists anywhere (archive replaces delete). |
| **A Job will not archive actions or delete action steps.** | Jobs | **Code.** `core__archive_action` / `core__unarchive_action` / `core__delete_action_step` are not wired into the Job palette. |
| **A Job posts only to the case's bound Slack channel.** | Jobs | **Code.** The only Slack write tool a Job gets is `slack__post_to_case_channel`, hard-pinned to `Case.SlackChannelID`. |
| **Knowledge stays out of shared storage for private cases.** | Mention agent & Jobs on a private case | **Code.** `knowledge__create_knowledge` / `knowledge__update_knowledge` are withheld; only read tools remain. |
| **A Job cannot read its own past run traces.** | Jobs | **Prompt.** Determine idempotency from the current case state, the action list, and Slack history — not from prior traces. |

When a guardrail is "prompt only", treat it as a firm design constraint: do not
write a Job prompt that tries to talk the agent around it (e.g. "ignore the
close restriction and close the case"). If you genuinely need a Job to perform a
restricted action, that is a change to the binary, not to a prompt.

## See also

- [Configuration → Job Definitions](configuration.md#job-definitions-job) — the `[[job]]` schema, scheduling, and execution strategy.
- [Operations → Agent Jobs operations](operations.md#agent-jobs-operations) — the runtime behaviour (triggers, concurrency, run log).
- [Integrations](integrations.md) — turning the Notion and GitHub tools on.
- [Concepts](concepts.md) — the vocabulary (Case, Action, Workspace, Job).
