# Slack Mention → Case Draft

When a user mentions the Hecatoncheires bot (`@hecatoncheires`) in a Slack
channel that is **not** already bound to an existing Case, the bot collects
surrounding context, asks an LLM to produce a Case payload tailored to a
selected workspace's `FieldSchema`, and shows the user an ephemeral preview
with workspace selector + Submit / Edit / Cancel buttons.

## Behavior

| Where the bot is mentioned | What happens |
|---|---|
| In a Case-bound channel | The existing `AgentUseCase` flow runs (no change). |
| In any other channel | The Mention-Draft flow runs (this feature). |

### Mention dispatch

1. The bot receives `app_mention`.
2. `SlackUseCases.HandleSlackEvent` checks every registered workspace for a
   Case bound to the mentioning channel. If found → existing Agent path.
3. Otherwise → `MentionDraftUseCase.HandleAppMention` is invoked.

### Mention-Draft flow

1. **Accessible workspaces** — the user's accessible workspaces are fetched
   (currently all registered workspaces). The host no longer pre-selects one;
   the planner picks the workspace itself via the `list_workspaces` /
   `get_workspace` tools (see step 3).
2. **Message collection** —
   - In a thread: latest 64 thread messages.
   - Outside a thread: messages within the last 3 hours, capped at 64.
   - The originating channel's descriptor (name, topic, purpose,
     privacy, member count, archive / shared flags, creator, created
     time) is fetched once via `conversations.info` and included as a
     dedicated `# Channel context` block at the top of the planner's
     first user message. This gives the planner a workspace-inference
     anchor without spending a tool call on it. The lookup is
     non-fatal: a failure is funneled through `errutil.Handle` and the
     section is omitted; the rest of the prompt still renders.
3. **Planner-driven turn** — the open-mode `draft.UseCase` (in
   `pkg/usecase/agent/draft`) acquires a per-thread turn lock on the
   Session, then runs a planner LLM round-trip against the conversation
   history. The planner agent is **tool-enabled**: the system prompt
   carries only the workspace identity tier (id / name / description), and
   the planner pulls the field schema and source list per turn via the
   `pkg/agent/tool/wsmeta` tools (`list_workspaces`, `get_workspace`).
   Each round, the planner emits a JSON plan with one of three actions:
   `investigate` (parallel sub-agent fan-out under read-only tool sets),
   `question`, or `materialize`. The terminal action for a normal mention
   is `materialize`, which produces `Title`, `Description`, and a
   `custom_field_values` map for the **planner-selected** workspace's
   `FieldSchema`. Loop budgets (planner / sub-agent / sub-agent inner)
   bound runaway turns; when exhausted, the runtime returns
   `StatusFallback` and the host posts a system fallback message.

   **Per-message trace UI** — every progress event renders as its own
   Slack thread reply rather than as a row inside a single growing
   context block. Concretely:

   - **Phase trace** (planner round start, action selections, retry
     notices, the `investigate.message` phase prelude) — each
     `Handler.Trace` call posts a **fresh thread reply**. Lines never
     accumulate inside a single message that grows over time.
   - **Per-task trace** — when the planner picks `investigate`, the
     runtime calls `Handler.RegisterTasks` once with all sub-agent
     task IDs + titles BEFORE any sub-agent goroutine starts. The
     host posts **one fresh thread reply per task** at that moment,
     so each task block is anchored at its own position in the
     thread. Sub-agents then update their own task message in place
     via `Handler.TraceTask`; they never post fresh Slack messages.
     Within each sub-agent, a gollem `ContentBlockMiddleware`
     surfaces per-iteration progress: the middleware turns the
     LLM's accompanying thought into a one-line excerpt, and
     overrides it with `🛠 calling <tool>` when the same response
     carries a tool call. Terminal `done` / `failed` lines replace
     the running text once the sub-agent returns.

   The initial `processing…` placeholder posted at mention time is
   reserved for one specific transition: at `materialize`, that
   message is updated in place with the rendered preview blocks. Trace
   lines never reuse this TS.
4. **Preview thread reply** — once the planner emits `materialize`, the
   `slackDraftHandler` (host adapter) updates the in-place "processing…"
   message with the rendered preview blocks.
5. **User actions** —
   - `Submit` → Case is created with the materialization and a thread reply
     with the new Case link is posted in the originating thread (or as a new
     thread reply to the mention if the mention was outside a thread).
   - `Edit` → opens a dynamic modal whose blocks come from the workspace's
     `FieldSchema`; on submission, the Case is created from the modal values.
   - `Cancel` → ephemeral is deleted and the draft is removed.
   - `Workspace selector` → the preview is locked (`InferenceInProgress`
     set on the persisted draft as a server-side guard) and the same
     `draft.UseCase` is re-invoked with `TriggerWSSwitch`. The planner
     re-materialises against the new workspace's schema using the
     existing conversation history, and the preview is re-rendered.

## Storage

Drafts are persisted in the **workspace-agnostic** Firestore collection
`case_drafts`. They have a 24-hour TTL (`ExpiresAt` field). The Firestore
implementation rejects expired records on `Get`. The collection is **not**
nested under a workspace because the draft does not yet belong to one.

There is no per-workspace materialization cache: switching workspaces always
re-runs the LLM and overwrites the single `Materialization` slot in the
draft.

## Required Slack OAuth scopes

The flow uses these scopes in addition to existing ones:

- `chat:write` — post the preview ephemeral and the thread reply (existing).
- `channels:history`, `groups:history` — read messages in public/private
  channels for context collection.
- `chat:postEphemeral` is implied by `chat:write` for ephemerals scoped to
  the channel the bot is in.
- `commands` is **not** required (we trigger via `app_mention`, not slash
  commands).

The bot must be a member of the channel where the mention happens, otherwise
no `app_mention` event is delivered and message collection has no source.

### Thread-reply resume (post_question)

When the planner ends a turn on `post_question`, the user can answer either
by `@mention`-ing the bot again or by replying in the same thread without
a mention. The dispatcher subscribes to:

- `app_mention` event (existing) — covers re-mention.
- `message.channels` event (existing in public channels) — covers
  no-mention reply in public channels.
- `message.groups` event — required only if you want no-mention reply
  resume to work in **private** channels. Adding this scope/subscription
  triggers a Slack app re-install.

The dispatcher then runs the F1-F8 filter chain (see `pkg/usecase/slack.go`
`shouldResumeOnReply`) to drop bot/duplicate/un-tracked messages. F5
(`<@botUserID>` substring check) ensures `app_mention` and
`message.channels` duplicates do not trigger the planner twice.

## Recovery from a wrong workspace pick

The planner picks the workspace from the registered list and may still pick
the wrong one when the conversation is ambiguous. The user can switch to the
correct workspace **before** submitting, in which case the entire
materialization is regenerated for the new schema (the synthetic
`TriggerWSSwitch` user message names the new workspace explicitly so the
planner re-materialises against it without re-running its own selection).
After Submit there is no built-in switch flow; the user closes the wrongly
placed Case and re-runs the mention (the source material is in Slack, not in
the deleted draft).

## Configuration

This feature is enabled automatically when both an LLM client and a Slack
service are configured. No extra environment variables are required.

| Constant (Go) | Value | Meaning |
|---|---|---|
| `model.CaseDraftTTL` | 24h | Draft expiry. |
| `slacksvc.MaxCollectedMessages` | 64 | Per-mention message cap. |
| `slacksvc.ChannelLookbackWindow` | 3h | Time window for non-thread mentions. |

## Failure modes

- **LLM unavailable / planner budget exhausted** — the runtime returns
  `StatusFallback` with a non-empty reason; the host renders a system
  fallback message asking the user to re-mention with more context. The
  draft row is still persisted (without a materialisation) so a subsequent
  ws-switch or thread reply can resume.
- **Permalink fetch fails** — the affected message is included with an empty
  `Permalink`; the failure is logged via `errutil.Handle`.
- **No accessible workspace** — an ephemeral error message is shown to the
  user and no draft is created.
- **Concurrent turn on the same thread** — the per-thread turn lock
  rejects the new trigger; the host posts the i18n busy notice and the
  duplicate trigger is dropped (`StatusBusy` / `StatusIdempotent`).
