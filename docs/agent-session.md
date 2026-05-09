# Agent Thread Session

The agent that responds to `@mention` in Slack threads treats each thread as
a long-running **Session** (`pkg/domain/model.Session`). The session ties a
Slack thread to either a Case (case-bound mode, when the channel is bound to
an existing Case) or to a draft-in-progress (open mode, when the bot is
mentioned in an unbound channel). It persists the gollem conversation history
so follow-up mentions can pick up where the previous turn left off, and
writes a Trace blob for every turn for diagnostics.

A per-thread **turn lock** (CAS-backed in Firestore, mutex-backed in memory)
prevents two turns from running concurrently on the same thread. A heartbeat
goroutine refreshes the lock every 10s; if the holder dies, the next caller
reclaims the stale lock after the staleness window (default 30s).

## Lifecycle

1. A user `@mention`s the bot in a channel that is bound to a Case.
2. The agent looks up an existing AgentSession by
   `(workspaceID, caseID, threadTS)`. If none exists, it creates a new one
   with a fresh UUIDv7 ID.
3. For new sessions, the full thread context is folded into the system
   prompt. For continuing sessions, only **unprocessed** thread messages
   (those with `ts > LastMentionTS` and `userID != botUserID`) are
   surfaced to the agent as user input.
4. The agent runs against gollem with `WithHistoryRepository` so each LLM
   turn auto-persists to Cloud Storage. A trace.Recorder is also attached
   so the per-turn execution graph (LLM calls, tool calls, sub-agents) is
   captured.
5. After the response is posted, `LastMentionTS` is updated to the current
   mention's TS so the next mention only ingests truly new chatter.

If the mention thread happens to live under an Action notification message
(matched via `Action.SlackMessageTS`), the session records the `ActionID`.

## Storage layout

Configurable via two CLI flags / environment variables:

| Flag | Env | Required | Purpose |
| --- | --- | --- | --- |
| `--cloud-storage-bucket` | `HECATONCHEIRES_CLOUD_STORAGE_BUCKET` | **yes** | Bucket holding History/Trace blobs |
| `--cloud-storage-prefix` | `HECATONCHEIRES_CLOUD_STORAGE_PREFIX` | no | Optional path prefix within the bucket |

Object layout under the bucket:

```
{prefix}/v1/sessions/{sessionID}/history.json
{prefix}/v1/traces/{sessionID}/{traceID}.json
```

- `sessionID` = `Session.ID` (UUIDv7).
- `traceID` = the `ts` of the mention message that triggered the turn —
  one trace per mention.

The `serve` command refuses to start when the bucket flag is unset.

Session metadata (workspace, case, thread TS, action linkage, last mention
TS, turn-lock fields, optional draft binding) is stored in Firestore keyed
by Slack channel + thread TS:

```
slack_channels/{channelID}/sessions/{threadTS}
```

The same Session row is used by both modes — case-bound mention agent
(`pkg/usecase/agent/casebound`) and open-mode draft agent
(`pkg/usecase/agent/draft`). Mode is discriminated at lookup time:
`Session.IsCaseBound()` returns true when `CaseID != 0`.

No new Firestore composite indexes are required; lookups are direct
document fetches.

## Required IAM

The service account that runs the application needs read/write access to
the configured Cloud Storage bucket. The least-privilege role is
**Storage Object Admin** scoped to the bucket (or the prefix if you split
buckets across environments). `Storage Object Viewer` alone is
insufficient — Save mutates objects on every LLM turn.

## Reading the artifacts

History blobs are gollem `History` JSON (`pkg/m-mizutani/gollem` v0.24+
format, version 3). They can be loaded back into a Go process via
`gollem.HistoryRepository.Load(ctx, sessionID)`.

Trace blobs are gollem `trace.Trace` JSON. The `metadata.labels` map
includes:

- `session_id` — `AgentSession.ID`
- `workspace_id`, `case_id`, `thread_ts`, `action_id` — domain identifiers
- `trigger_mention_ts` — the Slack TS that triggered this turn

Use these labels to slice traces in any downstream observability tool.

## Available agent tools

The mention agent loads tools from several namespaces. Each is gated on its
own configuration; missing config silently disables that namespace's tools
(no startup failure).

| Namespace | Tools | Gate |
| --- | --- | --- |
| `core__*` | Action read/create/update/archive | Always on |
| `slack__*` | Workspace search (read-only), bulk message fetch | Slack bot token; search additionally requires the User OAuth token with `search:read` |
| `notion__*` | Page/database search, page Markdown fetch | `--notion-api-token` |
| `github__*` | Issue/PR search, single Issue/PR fetch, file content, commit history | All three `--github-app-*` flags |

The mention flow uses the **read-only** Slack tool set (no `post_message` —
the trace UI handles outbound messages). The `assist` flow uses the full
Slack tool set including `slack__post_message`.

GitHub tools (`github__search`, `github__get_issue`, `github__get_pull_request`,
`github__get_file`, `github__list_commits`) are described in detail in
[docs/config.md](config.md#agent-tools). They share the same GitHub App
installation as the Source pipeline.
