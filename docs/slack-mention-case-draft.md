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

1. **Workspace estimation** — the user's accessible workspaces are fetched
   (currently all registered workspaces). When more than one is accessible,
   the workspace where they reported a Case most recently in the past 30 days
   is preferred; otherwise the first registered workspace.
2. **Message collection** —
   - In a thread: latest 64 thread messages.
   - Outside a thread: messages within the last 3 hours, capped at 64.
3. **AI materialization** — `DraftMaterializer` is given the raw messages,
   the user's mention text, and the selected workspace's `FieldSchema`. It
   produces `Title`, `Description`, and a `custom_field_values` map covering
   the schema. Fields the LLM cannot confidently fill are omitted; required
   fields left empty surface during Submit (the user is routed to the Edit
   modal to complete them).
4. **Preview ephemeral** — posted via `chat.postEphemeral` with full
   per-field display.
5. **User actions** —
   - `Submit` → Case is created with the materialization and a thread reply
     with the new Case link is posted in the originating thread (or as a new
     thread reply to the mention if the mention was outside a thread).
   - `Edit` → opens a dynamic modal whose blocks come from the workspace's
     `FieldSchema`; on submission, the Case is created from the modal values.
   - `Cancel` → ephemeral is deleted and the draft is removed.
   - `Workspace selector` → the ephemeral is locked (context block "推論中…",
     selector disabled, action buttons removed) **before** any AI call,
     `InferenceInProgress` is set on the persisted draft as a server-side
     guard, the LLM is re-run for the new workspace's schema, the draft's
     materialization is overwritten, and the preview is re-rendered.

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

## Recovery from a wrong workspace pick

The estimation is intentionally cheap and may pick the wrong workspace. The
user can switch to the correct workspace **before** submitting, in which
case the entire materialization is regenerated for the new schema. After
Submit there is no built-in switch flow; the user closes the wrongly placed
Case and re-runs the mention (the source material is in Slack, not in the
deleted draft).

## Configuration

This feature is enabled automatically when both an LLM client and a Slack
service are configured. No extra environment variables are required.

| Constant (Go) | Value | Meaning |
|---|---|---|
| `model.CaseDraftTTL` | 24h | Draft expiry. |
| `slacksvc.MaxCollectedMessages` | 64 | Per-mention message cap. |
| `slacksvc.ChannelLookbackWindow` | 3h | Time window for non-thread mentions. |

## Failure modes

- **LLM unavailable** — the Materializer falls back to a deterministic payload
  (`Title` = mention text excerpt, `Description` = transcript, no custom
  fields). The user can still Edit the modal to fill in fields manually.
- **Permalink fetch fails** — the affected message is included with an empty
  `Permalink`; the failure is logged via `errutil.Handle`.
- **No accessible workspace** — an ephemeral error message is shown to the
  user and no draft is created.
