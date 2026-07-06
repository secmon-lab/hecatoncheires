# Architecture (internals)

This document explains the "why" and "how" of Hecatoncheires' internal design for human contributors. It complements — but does not replace — the machine-enforced rules in [`.claude/rules/`](../../.claude/rules/) and the project context in [`CLAUDE.md`](../../CLAUDE.md). For the developer documentation index, see [develop/README.md](./README.md).

## GraphQL DataLoader

The GraphQL layer uses a request-scoped DataLoader pattern (via
[`github.com/graph-gophers/dataloader/v7`](https://github.com/graph-gophers/dataloader))
to collapse N+1 fetches that arise when a list query renders sub-resolvers
for each row.

### Where it lives

- `pkg/controller/graphql/dataloader.go` — loader definitions, batch
  functions, request-context plumbing
- `pkg/cli/serve.go` — middleware that instantiates one
  `*DataLoaders` per HTTP request before invoking the gqlgen handler
- `pkg/controller/http/graphql_test.go` — the same per-request
  wiring on the test side, so resolver tests exercise the real
  batching path

### Why request-scoped

- The internal cache MUST NOT survive across requests. A loader that
  outlives one request would leak one user's view to another (private
  cases, restricted assignees) and break the multi-instance safety
  invariant in CLAUDE.md.
- `dataloader.NewBatchedLoader` is cheap; constructing seven loaders
  per request (`SlackUser`, `SlackChannelName`, `Action`, `Case`,
  three `ActionsByCase` scopes) is below noise on every CPU profile.
- Batching only collapses calls *inside* one batch tick anyway — the
  graph-gophers wait window is 16 ms by default — so a per-request
  loader is the longest meaningful scope.

### What gets batched

| Loader | Batch source | Solves N+1 on |
|---|---|---|
| `SlackUser` | `repo.SlackUser().GetByIDs(ctx, ids)` | `Case.reporter`, `Case.assignees`, `Case.channelUsers`, `Action.assignee`, `ActionEvent.actor` |
| `SlackChannelName` | `slackSvc.GetChannelNames(ctx, ids)` | `Case.slackChannelName` (the original Cases-page hotspot) |
| `Action` | `repo.Action().GetByIDs(ctx, ids)` | future Action sub-resolvers |
| `Case` | `repo.Case().GetByIDs(ctx, ids)` | `Action.case`, `Action.steps`, `Action.events`, `Action.messages`, `Action.stepProgress` |
| `ActiveActionsByCaseLoader` / `Archived` / `All` | `repo.Action().GetByCases(ctx, caseIDs, opts)` | `Case.actions` |

`SlackUser`, `Action`, and `Case` repositories all expose `GetByIDs`
returning a `map[K]*V`; missing IDs are silently absent (callers
distinguish "missing" from "found" themselves). The DataLoader batch
function fans those map results back out into per-key `Result` entries
in the order the dataloader supplied keys.

### Calling convention from resolvers

```go
// single load
user, err := loaders.SlackUser.Load(ctx, *obj.ReporterID)()

// many loads (returns []V, []error — per-key parallel arrays)
users, errs := loaders.SlackUser.LoadMany(ctx, obj.AssigneeIDs)()

// composite-key load (Case, Action, ActionsByCase)
c, err := loaders.Case.Load(ctx, MakeCaseKey(workspaceID, caseID))()
```

Each `Load` returns a `Thunk[V]`; calling the thunk is what actually
waits for the batch to fire. gqlgen runs sub-resolvers concurrently, so
the parallel `Load` calls all enqueue within the wait window and the
first thunk to be called triggers the single `GetByIDs` / batch fetch.

#### Handling missing keys

- `*SlackUser`: missing IDs return `Data: nil` (no error). The
  resolver decides: `Case.reporter` returns a field-level
  `ErrSlackUserNotInRepo` because the empty cell is the original
  bug; `Case.assignees` filters nils because `[SlackUser!]!`
  requires non-null elements; `Action.assignee` returns nil
  directly because the schema field is nullable.
- `*SlackChannelName`: returns `Data: nil` for IDs that the Slack
  service did not resolve. The resolver passes that through as a
  null GraphQL field.
- `Case` / `Action`: missing IDs return `Data: nil`. Resolvers that
  treat absence as "not visible to this requester" check for nil
  and return empty results (e.g. access-denied paths).

### Adding a new loader

1. Add a batch method on the relevant repository / service that
   takes a `[]K` and returns a `map[K]*V` (preferred — easier to
   reorder than a slice).
2. In `dataloader.go`:
   - Add a field on `DataLoaders`.
   - Wire it in `NewDataLoaders`.
   - Write a `buildXxxBatch` closure that:
     - Dedupes / normalises keys
     - Calls the repository once
     - Emits one `*dataloader.Result[V]` per input key in order,
       using `Data: nil` for legitimate "not found"
3. Replace per-row repository calls inside resolvers with
   `loaders.Xxx.Load(ctx, key)()` / `LoadMany(ctx, keys)()`.
4. Add (or extend) the regression test in
   `pkg/controller/graphql/dataloader_test.go` that wraps the real
   repository with a call counter and asserts the batch ran exactly
   once for the workload.

### Why we didn't keep the old "fake DataLoader"

Before this rewrite, `pkg/controller/graphql/dataloader.go` exposed
types named `SlackUserLoader`, `ActionLoader` etc. but each was just a
batch-fetch helper: `Load(ctx, ids)` made one repository call and
returned. Resolvers called it per row (`Load(ctx, []string{singleID})`)
because there was no debounce layer, so a 20-case list page issued 20
`SlackUser.GetByIDs` calls for reporter, 20 more for assignees, and 20
Slack API calls for channel names — even with caching on top. The
graph-gophers loader collapses each of those to one call per request.

## Agent thread session (internals)

The agent that responds to `@mention` in Slack threads treats each thread as
a long-running **Session** (`pkg/domain/model.Session`). The session ties a
Slack thread to either a Case (case-bound mode, when the channel is bound to
an existing Case) or to a draft-in-progress (open mode, when the bot is
mentioned in an unbound channel). It persists the gollem conversation history
so follow-up mentions can pick up where the previous turn left off, and
writes a Trace blob for every turn for diagnostics.

In case-bound mode the agent can edit the bound Case directly via the
`case__update_case` (title / description / custom fields), `case__assign` /
`case__unassign` (delta assignee changes), and a mode-specific "mark done" tool
— `case__update_case_status` for thread-mode workspaces (move to a closed board
status) or `case__close_case` for channel-mode cases (close OPEN -> CLOSED) —
the same tools the
event-driven Agent Jobs use. They funnel through `CaseUseCase.UpdateCase` /
`AssignCase` / `UnassignCase` / `UpdateCaseStatus` / `CloseCase`, so every entry point (Web
GraphQL, Slack modal, Job, mention agent) enforces the same validation,
including the SlackUser existence check on newly assigned users and user-typed
field values. Assignees are mutated only through the delta `AssignCase` /
`UnassignCase` path (never as a full-list replace on `UpdateCase`), so
concurrent edits cannot clobber one another.

A per-thread **turn lock** (CAS-backed in Firestore, mutex-backed in memory)
prevents two turns from running concurrently on the same thread. A heartbeat
goroutine refreshes the lock every 10s; if the holder dies, the next caller
reclaims the stale lock after the staleness window (default 30s).

#### Case-mode invariants (enforced at the usecase boundary)

A Case is either **channel-mode** (`SlackThreadTS == ""`; dedicated channel) or
**thread-mode** (`SlackThreadTS != ""`; bound to a Slack thread, tracked by a
configurable board status / Kanban). Two invariants follow from this split and
are enforced at the **usecase boundary** — not just by withholding agent tools —
so every entry point (GraphQL, Slack, agent tools, eval) is covered uniformly:

- **Lifecycle path is mode-specific.** Thread-mode cases change lifecycle only
  by moving their board status (`UpdateCaseStatus`, which keeps `BoardStatus` and
  `Status` in sync); `CloseCase` / `ReopenCase` reject thread-mode cases
  (`ErrCaseThreadModeUseStatus`). Symmetrically, `UpdateCaseStatus` rejects
  channel-mode cases (no board status). The Web UI mirrors this: the
  close/reopen button shows only for channel-mode, the Kanban only for
  thread-mode.
- **Actions belong to channel-mode only.** Thread-mode cases have no Actions
  (the configurable status attaches to the Case itself there, not to Actions).
  `ActionUseCase.CreateAction` and `UpdateAction`'s reparent path reject a
  thread-mode parent / target (`ErrCaseThreadModeNoActions`). The agent tool
  wiring additionally withholds the action (`core__*`) tools for thread-mode in
  all three hosts (Job runtime, case-bound mention agent, eval env) so the LLM
  is never offered a tool that can only error.
- **Thread-mode Jobs embed the thread's recent messages in the system prompt.**
  Because a thread-mode Job has no Actions to anchor on, `JobRunner` loads the
  Case's recent Slack messages (`CaseMessageRepository.List`, bounded to the
  newest `recentMessageMaxCount` = 32 within `recentMessageWindow` = 24h of the
  run start, ordered oldest-first) and renders them in a dedicated system-prompt
  section. Each body is rune-truncated to `recentMessageTruncateRunes` = 140 with
  the original character count annotated when elided. Channel-mode Jobs skip this
  read entirely (the section is absent from their prompt).

### State persistence across turns

The turn lifecycle persists several pieces of state so that a follow-up
mention resumes where the previous turn left off:

- For new sessions, the full thread context is folded into the system
  prompt. For continuing sessions, only **unprocessed** thread messages
  (those with `ts > LastMentionTS` and `userID != botUserID`) are
  surfaced to the agent as user input.
- The agent runs against gollem with `WithHistoryRepository` so each LLM
  turn auto-persists to Cloud Storage. A trace.Recorder is also attached
  so the per-turn execution graph (LLM calls, tool calls, sub-agents) is
  captured.
- After the response is posted, `LastMentionTS` is updated to the current
  mention's TS so the next mention only ingests truly new chatter.

If the mention thread happens to live under an Action notification message
(matched via `Action.SlackMessageTS`), the session records the `ActionID`.

### Thread-mode case initialization (deferred, agent-driven)

In thread mode, case creation is initiated **only** by a post at the channel
root (a top-level message in the monitored channel). A **human** root post
always qualifies. A **bot-authored** root post qualifies only when the workspace
opts in via `[slack] accept_bot` (default off) — otherwise a
channel would spawn a Case for every bot notification. `isThreadCaseCreationTrigger`
rejects replies, edits, system events, and our own bot's posts; a
`bot_message`/`bot_id` post additionally requires the opt-in flag. This is
deliberate: in opted-in channels the case-creating signal is often an intake-form
app's relayed request.
The reporter is, as a rule, the post's author; only when the author is a bot
does `HandleThreadCaseCreation` fall back to resolving it from the first Slack
user mention in the body (the requester named in the form). When none is
present the reporter stays empty: thread-mode Cases are exempt from the
mandatory-reporter rule (`model.Case.ValidateNew` requires `ReporterID` only for
channel-mode Cases), so creation still proceeds and the GraphQL `reporter` field
resolves to null. A mention or a reply inside a thread that is not bound to a Case is
ignored — activity inside an arbitrary thread never starts a Case. A
channel-root post does **not** create a Case immediately, though:
`HandleThreadCaseCreation` runs the `threadcase`
plan-and-execute agent in
`ModeCreate`: it investigates (read-only search tools), may ask the reporter a
question (terminal `question` action → the turn ends and waits), and only
commits a Case once it produces a final `create` decision that passes full
field validation. The commit happens **inside the planner loop** via the
planexec `OnFinalize` hook (host callback `Handler.Create` →
`CaseUC.CreateThreadCaseWithFields`): if validation fails (all violations are
aggregated, not fail-fast) *or* the persistence call fails, the error folds
back as another planner round so the agent can fix and re-emit, bounded by
`PlannerLoopMax`. On success the host posts a Block Kit summary; on budget
exhaustion it posts a fallback notice.

Because a `question` ends the turn (the per-thread turn-lock cannot be held
while waiting on an async Slack reply), the task can span multiple turns. A
pending question is answered through the question form's **Submit** interaction
(`HandleThreadCaseQuestionSubmit`), which resumes the create agent via
`runThreadCaseCreation` — free-text replies / mentions in the not-yet-a-case
thread are intentionally ignored. (`ResumeThreadCaseCreation` still drives this
resume directly, but in production it is reached only by the offline eval
harness.) The **same** thread Session (and therefore the same gollem history
key) is reused across the initial turn, any question/answer resume turns, and
the later case-bound mention turns — so the conversation history is one
continuous thread. The created case id is stamped onto the Session
(`Session.CaseID`) without changing `Session.ID`. See the agent runtime
vocabulary (turn / round / budget) in `.claude/rules/architecture.md`.

### Storage layout

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

### Required IAM

The service account that runs the application needs read/write access to
the configured Cloud Storage bucket. The least-privilege role is
**Storage Object Admin** scoped to the bucket (or the prefix if you split
buckets across environments). `Storage Object Viewer` alone is
insufficient — Save mutates objects on every LLM turn.

### Reading the artifacts

History blobs are gollem `History` JSON (`github.com/gollem-dev/gollem` v0.26+
format, version 3). They can be loaded back into a Go process via
`gollem.HistoryRepository.Load(ctx, sessionID)`.

Trace blobs are gollem `trace.Trace` JSON. The `metadata.labels` map
includes:

- `session_id` — `AgentSession.ID`
- `workspace_id`, `case_id`, `thread_ts`, `action_id` — domain identifiers
- `trigger_mention_ts` — the Slack TS that triggered this turn

Use these labels to slice traces in any downstream observability tool.

The agent tools available within these sessions are described in
[Configuration](../configuration.md#agent-tool-registry-slack-mention--assist).
They share the same GitHub App installation as the Source pipeline.

### Mention runs on the case agent page

The Cloud Storage trace above is the durable, full-fidelity artifact. In
addition, every post-creation mention turn is recorded as a queryable
`JobRunLog` + `JobRunEvent` trail in Firestore so it appears on the case
agent page (`/ws/{workspace}/cases/{id}/agent`) alongside scheduled and
lifecycle Job runs, through the same `caseJobRunLogs` read path.

- Both mention hosts — `casebound` (channel-mode) and `threadcase`
  `ModeMention` (thread-mode) — record via `pkg/agent/runtrace`
  (`runtrace.Recorder` + `runtrace.Handler`), the same machinery the Job
  runner uses.
- Mention runs are not configured Jobs, so each mention turn gets its own
  fresh per-turn JobID and is tagged `EventType = model.EventTypeMention`;
  the page shows a localized "Mention" label (resolved from the eventType, not
  the opaque JobID). The `JobRunLog.TraceID` matches the Cloud Storage trace's
  `traceID`, so the two sinks correlate.
- Creation-time turns (`threadcase` `ModeCreate`) are excluded — only
  mentions in an already-created case are listed.

## See Also

- [develop/README.md](./README.md) — developer documentation index
- [User Guide](../user_guide.md) — the user-facing agent thread lifecycle and available agent tools
- [Configuration](../configuration.md) — TOML field definitions and the agent tool registry
- [CLI](../cli.md) — CLI flags and environment variables
- [Integrations](../integrations.md) — GitHub and Notion integration setup
- [Operations](../operations.md) — Sentry and observability
- [`CLAUDE.md`](../../CLAUDE.md) and [`.claude/rules/`](../../.claude/rules/) — enforced project rules
