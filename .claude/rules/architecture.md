---
paths:
  - "pkg/**/*.go"
---

# Architecture & layer responsibilities

The codebase is laid out as a classic layered architecture
(`controller → usecase → repository / service`). Each layer's job is
narrowly defined; cross-layer leakage is the most common code-review
failure mode in this repo, so the boundaries below are non-negotiable.
Apply them even when no rule explicitly calls them out — this section is
the authoritative checklist.

## controller (`pkg/controller/`)

**Responsibility:** translate transport-level concerns to usecase calls
and back. Nothing else.

The controller may:

- Parse the inbound request (body, headers, query/path params, signed
  payload verification, multipart, form decoding).
- Bound the request (size limits, auth checks, content-type validation).
- Pick which usecase method to call and marshal the request into that
  method's input struct.
- Translate the usecase's return value into a response (HTTP status
  code, GraphQL field, redirect, header).
- Acknowledge async / fire-and-forget contracts (e.g. write 200 to Slack
  before dispatching, since Slack enforces a 3-second deadline).

The controller MUST NOT:

- Touch repositories. No `repo.Case().Get`, no `repo.User().List`. If
  you need an entity loaded to decide what to do, that decision belongs
  in the usecase.
- Resolve domain identifiers (channel id → workspace id, slack user id
  → internal user, etc.). These mappings are domain logic.
- Call external services (Slack API, LLM, Notion, Firestore). Even
  "innocent" status pings belong in a service or usecase wrapper.
- Build domain blocks / messages (Slack Block Kit, email bodies, LLM
  prompts). Rendering belongs in `pkg/service/<name>/` or
  `pkg/usecase/`.
- Hold business invariants. Invariants belong inside the usecase that
  owns the entity.

If the controller needs information to make a decision, the answer is
*not* "load the entity here". The answer is "make the usecase method
idempotent and let it decide". The controller hands off raw payload
values; the usecase resolves and decides.

## usecase (`pkg/usecase/`)

**Responsibility:** orchestrate the business operation end-to-end.

The usecase:

- Resolves identifiers (channel → workspace, case id → case, etc.).
- Loads / mutates persistent state through `interfaces.Repository`.
- Calls external services through their respective service interfaces.
- Enforces invariants and idempotency (re-deliveries, duplicate clicks,
  already-finalised entities).
- Dispatches background work via `pkg/utils/async.Dispatch` when the
  operation has a sync entry point and an async tail.
- Returns *domain* errors / states; never HTTP status codes.

A usecase method's signature should take only domain primitives and the
raw payload values the entry point captured.

## Entry-point unification (NON-NEGOTIABLE)

A given business operation has **one** usecase method, regardless of
how many transport-level entry points trigger it. Slack interactivity,
GraphQL mutations, the CLI, and any future trigger all funnel into the
same usecase function — they MUST NOT each carry their own copy of the
rules, side-effects, or notifications.

This is the single most important invariant of this codebase. Every
business rule (validation, persistence, history-recording, Slack
notifications, idempotency, etc.) must live below the controller layer
so that *every* entry point triggers the same behaviour automatically.

### Anti-patterns (do not write this code)

```go
// WRONG: Slack handler writes to the repository directly to "skip
// the overhead" of the usecase. Now history, notifier, and any future
// hook fire only on the GraphQL path.
c.Status = newStatus
if _, err := repo.Case().Update(ctx, wsID, c); err != nil { ... }

// WRONG: business rule duplicated at the controller. The next reviewer
// has to remember both copies and keep them in sync.
if graphql {
    if isClosedStatus(newStatus) { recordClose(...) }
}
if slack {
    if isClosedStatus(newStatus) { recordClose(...) }
}
```

### Checklist before adding a new entry point

- [ ] Does an existing usecase method already implement this business
      operation? If yes, call it. If you find yourself copy-pasting
      logic from another handler, stop and refactor the shared logic
      into the usecase first.
- [ ] If you need to add a new side effect (history, notifier,
      generation job), is it added inside the usecase method, not at
      the entry point?
- [ ] Are repository writes confined to the usecase layer? A
      controller that calls `repo.X().Update` directly is a layering
      violation.
- [ ] If the operation has both a sync gate (validation) and an async
      tail (LLM, Slack post), does the *single* usecase method own
      both halves?

## Slack interactivity: ack-fast / dispatch-async (NON-NEGOTIABLE)

Slack enforces a **3-second deadline** on every interactivity callback
— `events_api`, `block_actions`, `view_submission`, `slash_commands`,
and `message_action` alike. Miss it and the user sees "We had some
trouble connecting" even though the work might eventually succeed.
Treat 3 seconds as 1 second of headroom: anything that talks to an
LLM, a database, or another Slack endpoint MUST run in the async tail.

### The default shape for Slack handlers

```go
// 1. Decode the raw payload (signature already verified by middleware).
// 2. Pick the usecase method.
// 3. Capture only the raw fields the usecase needs.
// 4. Ack Slack — write 200 (or the response_action body) RIGHT NOW.
// 5. async.Dispatch(ctx, func(ctx) error { return uc.HandleX(ctx, ...) })
```

The controller does NOT load entities, resolve workspaces, render
blocks, post Slack replies, call the LLM, or do anything else that
takes non-trivial time before acking. If the usecase needs to validate
input synchronously to return `response_action: errors`, do *only* the
validation sync, then internally `async.Dispatch` the heavy tail.

### Checklist before declaring a Slack handler done

- [ ] Does the controller call any usecase method that touches the
      LLM / Firestore / Slack API in its sync path? If yes, refactor
      — that work belongs in the async tail.
- [ ] If the usecase must run sync to return validation errors, is
      the *post-validation* tail wrapped in `async.Dispatch`?
- [ ] Tests covering the entry point call `async.Wait()` before
      asserting on side effects.
- [ ] The async tail re-loads any mutable state (don't reuse a
      `*model.X` pointer captured from the sync request) and
      re-checks idempotency.

## repository (`pkg/repository/`) and service (`pkg/service/`)

**Responsibility:** narrow adapters over a single backend.

- `repository/` only knows how to read/write entities. No business
  decisions, no Slack calls, no fan-out to other repositories.
- `service/<name>/` wraps a single external system (Slack, Notion,
  GitHub). It builds the protocol-level payloads (e.g. Block Kit
  blocks) and calls the third-party SDK. It does not load entities,
  does not consult the registry.

### Repository write contract (NON-NEGOTIABLE)

This subsection encodes the lesson from a real bug where the
Firestore `caseRepository.Create` was rebuilding the persisted
`*model.Case` via a field-by-field struct literal — and silently
dropped `ReporterID` (which had been added to the domain model
later). Every case persisted via Firestore lost its reporter, the
GraphQL `reporter` resolver returned nil, and the Cases page showed
empty Reporter cells indistinguishable from "no reporter recorded".
No test caught it because the memory repo round-tripped fine and the
Firestore tests skipped without `FIRESTORE_PROJECT_ID`.

The rules below exist to make that class of bug structurally
impossible:

- **NEVER copy `*model.X` field-by-field inside a repository.**
  Forbidden patterns include:
  - `created := &model.X{ID: ..., Title: x.Title, …}` — when a new
    field is added to `model.X`, this literal silently drops it.
  - Mirror "doc" struct types (`type caseDoc struct { … }`) paired
    with `toDoc(*model.X)` / `fromDoc(*doc) *model.X` converter
    functions. CLAUDE.md (`firestore.md`) already prohibits these
    for Firestore specifically; the broader principle applies to
    every backend.
  - `firestore:"..."` struct tags. Same reason — they encode a
    separate wire schema that drifts from the model.
- **The legal shape of `Create`** is: validate (`model.X.Validate()`)
  → assign the storage-side ID directly on the caller's pointer
  (`x.ID = nextID`) → `Set(ctx, x)` → `return x, nil`. Nothing
  else gets copied or rebuilt.
- **The legal shape of `Update`** is: validate → existence check →
  `Set(ctx, x)` → `return x, nil`. The caller's pointer is the
  source of truth for every field, including timestamps.
- **`time.Now()` does not belong in repository write methods.**
  Timestamp policy (CreatedAt on insert, UpdatedAt on every write)
  is business state and belongs in the usecase that owns the
  entity. A repo that stamps timestamps is forcing every caller
  through one clock and silently overrides the value the caller
  passed in. (Backends that need a server-side write timestamp for
  ordering may keep an internal field — that is separate from the
  domain CreatedAt / UpdatedAt.)
- **The `Validate()` method on each persisted model is mandatory
  invariant enforcement.** Repositories MUST call it before every
  write. Required identity fields (ReporterID, CreatorID, etc.)
  belong in `Validate` so that a usecase / handler bug that
  forgets to inject the reporter (e.g. a Slack interactivity
  callback that skipped `auth.ContextWithToken`) fails loudly at
  the first write instead of silently producing unattributable
  data. **Scoped exception:** `Case.ValidateNew` enforces
  `ReporterID` only for channel-mode Cases (the reporter is the
  channel creator). Thread-mode Cases (`SlackThreadTS` set) may be
  created by an integration bot's channel-root intake post that
  names no human, so an empty `ReporterID` is a legitimate state
  there; the GraphQL `reporter` field is nullable and resolves to
  null. A relaxation like this must be narrowly scoped and the
  reason recorded at the check, never a blanket removal.
- **Every persisted model needs a repository-level round-trip test
  that creates with all fields populated and reads each one back
  exhaustively.** Tests that only assert `Title` and `ID` cannot
  catch a Firestore Create that drops `ReporterID`. The check
  belongs in the shared `runXxxRepositoryTest` helper so memory
  and Firestore are compared apples-to-apples.

### Repository test environment requirement

The Firestore implementation MUST be exercised, not skipped. A
build that skips Firestore tests because `FIRESTORE_PROJECT_ID` is
unset gives a false green: the memory repo round-trips models via
`copyCase` (full struct copy), so a field dropped only on the
Firestore Create path never surfaces. Run the Firestore tests
against the Firestore emulator in CI and locally — the same shared
helper produces identical assertions across both implementations.

**The `_Firestore` tests MUST NOT `t.Skip` on a missing env var.**
The shared helper (`newFirestoreRepository`) never skips: when no
real-Firestore project is configured it defaults to a local emulator
(`FIRESTORE_EMULATOR_HOST=127.0.0.1:28615`, project `test-project`,
database `(default)`), so a bare `go test ./...` runs the tests and
fails loudly with a connection error when no emulator is reachable,
instead of passing as a no-op. To target real Firestore instead, set
`TEST_FIRESTORE_PROJECT_ID` (with ADC). Run the emulator locally via
`task test:firestore` (Docker); CI starts the same emulator in
`.github/workflows/test.yml`. Never reintroduce an env-gated skip
here — that silent skip is the exact failure mode that hid the
`ReporterID` drop (issue #189).

## domain (`pkg/domain/`)

Pure types, interfaces, and validation. No I/O, no logging, no
goroutines. Models in `pkg/domain/model/` are also the Firestore wire
format, so additions there must remain serialisable.

## Quick smell tests

- *"Could I move this code into the controller / out of the
  controller without changing behaviour?"* If yes, it is in the wrong
  layer.
- *"Does this controller import `repository` or `gollem` or
  `service/slack` for anything other than passing to a usecase
  constructor?"* If yes, push it down.
- *"Does this usecase return `http.StatusBadRequest`?"* If yes, the
  layering is leaking up.
- *"If I rewrote the transport (HTTP → gRPC → CLI), how much usecase
  code would I need to change?"* The answer should be "zero".
- *"If I trigger the same business operation from Slack and from the
  GraphQL API, do they hit the same usecase method?"* If no — or if
  logic is duplicated at the controller level — fix it before
  merging.

# Agent runtime vocabulary (planexec / proposal / threadcase)

These terms are easy to conflate; they have precise meanings across the
plan-and-execute agent runtime (`pkg/usecase/agent/...`). Use them
consistently in code, comments, specs, and reviews. There are four
nested levels:

- **Transition** — ONE `Strategy.Step` call: exactly one LLM call or one tool
  call, committed to storage before the next begins. It is the unit the
  runtime's step budget counts and the unit a claim can die between.
- **Round** — ONE planner / replan LLM call plus the work it dispatches (the
  plan's sub-agent tasks, a re-emit after a validation failure). A round spans
  several transitions.
- **Turn** — ONE `Spawn` and the Process it creates, from start until the
  Process reaches a terminal state. A turn runs *many rounds*. It ends on:
  - the planner asking the user (`question` → `Done(OutputQuestion)`) — the
    Process closes and the user's reply / form-submit starts the **next** turn;
  - a terminal output the host applies (e.g. case create / materialize);
  - failure (budget exhausted, internal error), which the host reports as a
    fallback.
- **Task** — the whole effort (e.g. creating one case), possibly spanning
  **multiple turns** separated by `question`s. (No stricter name yet;
  "task" is fine.)

**Why `question` ends the turn (not `Terminate:false`):** keeping a run live
while waiting minutes/hours for a Slack submit is not viable under horizontal
scaling — the thread's subject would stay held, refusing every other trigger.
The pending question is persisted (`Session.PendingQuestion`, shared backend)
and the answer arrives on a fresh dispatched event that starts a **new turn**.

**A resumed turn must inherit the asking run's conversation.** Because the answer
runs as a NEW Process, it starts from an empty history unless the Spawn says
otherwise — `WithSubject` serialises Processes, it does not link them. So the run
that asked is recorded on `PendingQuestion.AskedByProcessID`, and the answering
Spawn passes `WithInheritedHistory`. Without it the agent sees only the answer
text, with no record of the request it came from, the investigation behind the
question, or the question itself — which is exactly what the pre-agentkit runtime
gave it for free, by keying gollem history on the Session.

Pass the option through the host's `inheritOpts` helper, not directly: agentkit
**refuses** a Spawn naming an issuer that committed no conversation, so passing it
blindly turns "the asking run recorded nothing" into "the answer fails outright".
Starting fresh is the correct degradation.

**Entry points & final output.** planexec is a generic plan-execute
framework — it knows nothing about `case` and performs no side effects
itself. It exists ONLY as an agentkit Strategy (`planexec.Register`): the
in-process `Runner` and its `Run` / `RunText` / `ResumeText` entry functions are
gone, and every host — `serve`, `tick`, the eval harness — spawns onto the
runtime.

The host's terminal-output type is declared as `Config[T]` (`T` constrained by
`Validatable`), and the reply comes back as `Output[T]` through the completion
handler. planexec generates the terminal JSON, decodes it into `T`, calls
`T.Validate()`, and regenerates on failure (bounded by `finalOutputMaxRetry`;
gollem's schema check verifies shape only, so `Validate()` is where domain
invariants live). Plain-text hosts set `Config[T]{TextOnly: true}` with
`T = TextResult`.

Side effects (closing a case, posting a message, persisting the entity, …) are
performed either by the **sub-agents' tools inside the loop** or by the host
**after** the turn, from `onFinish` — never by planexec itself, and never inside
the loop as a commit hook. That rule is now enforced by the fact that the loop
and the handler are different processes.

`Config[T].Finalizers` run after `T.Validate()` inside the final-output
regeneration loop, but they are
**validation-only and side-effect-free**: they let a host enforce an invariant
that needs context `T.Validate()` cannot see (e.g. a workspace field schema), and
a returned error is fed back to the model and the output regenerated. This is how
ModeCreate feeds a bad field value (non-RFC3339 date, missing required field)
back for correction. Committing the case is still a post-turn `Handler.Create`,
NOT a finalizer side effect — a persistence failure there is surfaced and falls
back rather than being fed to the model, which cannot repair an infrastructure
error by re-emitting JSON. A finalizer must be side-effect-free because a retried
attempt re-runs it. See `.claude/rules/planexec.md` for the create-path wiring.

**Explicit termination.** The loop terminates ONLY when a replan round emits an
explicit `finalize` action. A replan must set exactly one of `tasks` /
`question` / `finalize`; setting none is rejected and folded back into another
replan round (the old "empty tasks = done" implicit termination is gone, so a
planner that merely forgot to emit tasks can no longer silently terminate).

**Direct mode (round-1 fast path).** When the host sets
`Input.AllowDirect`, the planner may answer a *genuinely trivial*
request on round 1 without any investigation: instead of `tasks` it emits a
`direct` payload (an optional tool-id subset), and the runtime replies through a
single ReAct child, returning plain text. It is strictly a fast path for
respond-style replies: even a structured host's turn returns text (not a decoded
`T`) on the direct path, because side-effecting terminal actions are by definition
not "trivial" and must go through the normal `tasks` → replan → `finalize` loop.
Hosts opt in (`threadcase` enables it for mention mode but disables it for
`ModeCreate`; `job` enables it; structured-only hosts leave it off). The
planner prompt guards it hard: "when in any doubt, investigate."

**The direct child is the only child whose text is published, so it gets its own
prompt.** `launchDirect` builds it from `prompts/direct.md`
(`buildDirectSystemPrompt`), never from the sub-agent template — that child's
output goes to the user verbatim, while every other child's goes to the planner.
The distinction is not stylistic: `prompts/subagent.md` instructs its reader to
open with a one-line conclusion, follow it with a supporting-evidence section
naming the sources it consulted, and address the result to "the parent planner".
Prompted that way — which is what `launchDirect` did between #261 and the fix —
the direct child wrote exactly that, and the whole report was posted into the
Slack thread as the reply. Two things follow for anyone changing this path:

- **The host's persona prompt and `Input.LanguageLabel` must reach it.** It is the
  one call in the run whose text the user reads, and nothing downstream rewrites
  or translates it — a lost language directive means the reply arrives in the
  wrong language, with no other symptom. `LanguageLabel` is the host's to fill,
  and all three planexec hosts resolve it through `i18n.LanguageLabel` so they
  cannot answer differently; a host that leaves it empty gives its whole run —
  planner, terminal output and direct reply alike — no directive at all.
- **The host prompt it carries is written for the planner**, so it names decision
  shapes (`respond`, `materialize`) and demands JSON. `prompts/direct.md`
  therefore overrides that half explicitly. Keep the override when editing either
  prompt: a direct child that emitted a decision object would have the JSON posted
  as prose.
- **Its text is read through `directReplyText`, not `collectResults`.** The latter
  bounds every child summary at `subAgentSummaryMaxBytes` (8 KiB) and appends an
  "…[truncated]" marker, which is correct for text fed back into the planner's
  context and wrong for a reply: it would cut the message off mid-sentence and
  publish the marker. Only the Job host discards the text — it reflects on the
  run's history instead, on `OutputFinal` and `OutputDirect` alike — so
  `prompts/direct.md` says the text is the turn's answer rather than claiming
  every host posts it.

**The planner's `message` is unread code-side and kept on purpose.** No code reads
`PlanResult.Message` / `ReplanResult.Message` — the `Sink` that once delivered it
was removed with the in-process Runner — but the planner reply carrying it is
committed to the run's conversation and recorded as that transition's
`LLM_RESPONSE`, so it is what the run timeline and the trace archive preserve of
WHY a turn decided as it did. Do not delete the field to tidy up an unread value.
Both halves must be stated wherever it is described (`plan.go`'s
`rationaleDescription` and `prompts/planner.md`, which must not drift from each
other): told the field is user-facing, a planner writes the answer into it and the
turn ends with the reply nowhere — the previous wording, "rationale shown to the
user", said exactly that; told only that nobody reads it, the planner emits a
placeholder and the record goes empty. And it is never wired into a reply: the
user sees the strategy's progress lines and the terminal output, never this.

## Agent tool wiring (host coverage) (NON-NEGOTIABLE)

A new agent tool is, by default, made available to **every** agent host that
legitimately needs it — not just the one path you happened to be working on.
The plan-and-execute runtime is driven from several hosts (`agent/proposal`
for mention/assist case-draft, `agent/job` for scheduled and lifecycle Jobs,
etc.), and each host assembles its own tool slice. Wiring a tool into only one
host silently starves the others.

This rule exists because read-only Slack/Notion tools were once wired only
into the mention/assist usecase path and forgotten on the Job path: the Job
agent was told (by its prompt) to read its case thread first, had no read
tool, and instead spammed the thread with "checking…" posts via the only
Slack tool it did have (the poster).

- When you add a tool, audit **all** host tool-builders and wire it into each
  one that should have it. For Jobs the single supply point is
  `buildJobTools()` in `pkg/cli/job_runtime.go`; the mention/assist path is
  wired via the `usecase` options in `pkg/cli/serve.go`. A tool that exists
  for one host but not another is a bug unless there is a documented reason.
- **Non-Action tools default to both channel-mode and thread-mode.** Only
  Action / `core` tools are mode-gated (Actions exist only in channel-mode).
  Read-only and auxiliary tools (Slack read, Notion, web fetch, knowledge,
  memo) are wired in both modes unless a specific, recorded reason excludes
  them.
- A tool whose dependency is nil must degrade safely (its constructor returns
  no tool), so wiring it unconditionally across hosts is safe even when a
  backend is not configured in a given deployment.
- **A prompt that instructs the agent to use a tool, and the wiring that
  actually provides that tool, must ship together.** A prompt that names a
  tool the host never wires drives the model to violate the prompt. When you
  change one, verify the other — and verify the agent has the context it needs
  to call the tool (e.g. a thread-reading tool needs the `thread_ts` exposed in
  the system prompt, not just the channel id).

## Trace handler wiring (host per-event timeline)

The same "wire it into every agent" discipline applies to the host's
per-event trace handler, not just tools. The shared handler lives in
`pkg/agent/runtrace` (`runtrace.Handler`), which turns gollem LLM / tool
call boundaries into the `JobRunEvent` records the run-detail UI reads.

On the durable runtime no host wires it per agent: the **claim middleware**
(`runTimeline` in `pkg/agent/kernel/middleware.go`) installs one for every claim
of every Process, parent and child alike, alongside the Cloud Storage archive
sink (combined via `trace.Multi`). That is what makes coverage automatic — a new
agent, or a new kind of transition, lands on the timeline without anyone
remembering to pass a handler down.

Two properties this relies on, which a change here must preserve:

- **The timeline is keyed on the Scope, not the handler.** A run gets one only
  when its Scope names workspace, case, job and job-run id. A run that keeps no
  run record leaves the archive as its only trace — the intended outcome, not a
  gap.
- **`Sequence` is allocated by the repository inside each write**
  (`JobRunEventRepository.AppendNext`), so the several Handlers a run accumulates
  — this claim's, a later claim's on another instance, the run owner's
  `RUN_ERROR` — append into one ordered timeline with nothing shared between
  them. Never reintroduce an in-process counter; it would hand the same number
  out twice.
- **The model name is read from the provider, not from agentkit.**
  `agentkit.GenerateResult` reports the tokens but not which model produced them,
  so `generateMiddleware` installs an `agenttrace.ModelCapture` in the *gollem*
  trace context for the duration of the call and takes the name off the
  `trace.LLMCallData` the provider client builds. It must stay a capture rather
  than the run's real handler: the provider drives the same `StartLLMCall` /
  `EndLLMCall` pair, so a real handler there records every call twice. An empty
  `model` on every event is what issue #266 reported.
- **The claim's handler is published in the gollem trace context while a tool
  runs** (`toolCallMiddleware`), so a tool that reaches an LLM itself — the
  knowledge tools' embedding calls, webfetch's page analysis — is recorded as an
  LLM call nested in its tool span. The pre-agentkit hosts got this from
  `gollem.WithTrace`, which published the handler for the whole `Execute`; after
  the migration nothing did, and those calls vanished from the timeline. This
  cannot duplicate a Generate: an agentkit Generate never runs inside a tool.
- **A `TOOL_CALL`'s `ParentSequence` is resolved when the tool STARTS**
  (`Handler.StartToolExec`, held on the span), never when it ends. Because of the
  bullet above, a tool that reaches an LLM itself records an `LLM_RESPONSE` while
  it runs, so the most recent response at the end of the tool is the tool's own —
  an end-time lookup points the row at an event nested inside it. Pinned by
  `TestAToolsOwnLLMCallDoesNotBecomeItsParent`.

Every production path now goes through the middleware — `tick` included, since it
spawns onto the same runtime and drives the worker itself. The in-process Job
executor still wires the handler directly via `gollem.WithTrace`, but nothing in
production reaches it any more (only the eval harness and tests). The bug that
produced this rule was a `planexec` Job whose executor never forwarded the handler
it was given, so the run succeeded with an empty timeline. Under the middleware that
failure mode no longer has a place to hide.

### Mention-triggered runs on the case agent page

`JobRunLog` / `JobRunEvent` are NOT Job-only: the case agent page
(`caseJobRunLogs`) lists **every** case-scoped agent run through one read
path. Post-creation Slack mentions handled by the `casebound` (channel-mode,
agentkit + the `react` strategy) and `threadcase` (thread-mode, planexec,
`ModeMention` only) hosts record the same records. They are not
configured Jobs, so **each mention turn gets its own fresh per-turn JobID**
(a UUID) and is tagged `EventType = model.EventTypeMention`. `EventType` — not
a reserved JobID — is the discriminator: `ResolveJobName` maps a run with that
eventType to a localized "Mention" label regardless of its opaque JobID, and
the registry-backed `caseJobs` (Automated Jobs) list never shows them because
their JobIDs are not in the workspace config. (Per-turn IDs keep each mention
run a standalone record and sidestep any per-JobID log-window cap; the
per-case mention count is small — order 10-20 — so the extra `JobRun` docs and
the O(N) `ListByCase`/`findLog` fan-out are negligible.)

Rules for this path:

- `runtrace.Recorder.Open` creates the RUNNING `JobRunLog`; `Finish`
  transitions it to SUCCESS/FAILED and calls `RecordRun`, which materialises
  the `JobRun` summary doc `ListByCase` reads. The summary is materialised at
  **Finish**, not Open — the mention hosts serialise concurrent turns through
  their own per-thread session lock, so the Recorder must NOT take the Job
  lease (it would falsely exclude a concurrent mention on a different thread
  of the same case). A side effect of per-turn JobIDs: a run interrupted
  before `Finish` never gets a parent `JobRun` doc, so its orphan RUNNING log
  is simply never listed (no perpetual-RUNNING row). The lifecycle method is
  named `Finish` (not `Close`) because it ends a run record, not an
  `io.Closer` (the goast policy reserves `.Close()` for `safe.Close`).
- **A durable run cannot hold a Recorder, so it opens and finishes the log at
  two separate points.** `casebound` calls `runtrace.Open` right after Spawn
  succeeds and `runtrace.FinishRun` from its agentkit completion handler, which
  reloads the RUNNING log from storage and folds in the usage read off
  `Process.Metrics`. Two consequences to preserve: the log is opened only
  **after** a successful Spawn (a refused turn must leave no orphan RUNNING
  row), and the usage totals come from the Process, because the run's
  transitions span claims and possibly instances and no single in-process
  handler sees them all.
- Keep the existing durable trace sink. No durable host wires one itself: the
  claim middleware (`pkg/agent/kernel/middleware.go`) opens a Cloud Storage trace
  per **claim**, keyed on the Process id rather than the Slack session id, and
  installs the `JobRunEvent` timeline beside it. See § "Trace handler wiring"
  above for what that guarantees. Do not replace the archive trace.
- `ModeCreate` (creation-time materialize) is excluded — the requirement is
  post-creation mentions.
- Trace recording is observability: `Open`/`Finish`/event failures are
  non-fatal (`errutil.Handle`) and must never fail the mention turn.

## Agent runtime: a rejected tool call says which argument was wrong

gollem validates a tool call's arguments against the `ToolSpec` before the tool
runs, and the message it produces names the tool and the expectation but NOT the
offending parameter: the name is carried as a goerr value on a per-parameter
error held in an unexported slice outside the `Unwrap` chain, so neither
`Error()` nor `goerr.Values` renders it, and it is unreachable from this
application. A model told only "expected array type" for a tool whose `creates` /
`updates` / `archives` are all arrays cannot tell which one to repair, and
re-emits the same call — which is what `memo__apply_memo_changes` did in
production (ARGUS-8S).

`toolArgsFeedbackMiddleware` (`pkg/agent/kernel/middleware.go`) supplies the
missing half by stating the SHAPE of the arguments that were actually received;
exactly one of them contradicts the expectation. Four properties a change here
must preserve:

- **Shape only, never a value.** The error reaches the run timeline and the
  operator's Sentry as well as the model, and tool arguments carry case content.
- **It is registered AFTER `toolCallMiddleware`**, so it runs inside the trace
  bracket and the archive records the same message the model was given.
- **It wraps, it does not replace.** `errors.Is(err, gollem.ErrToolArgsValidation)`
  must keep holding, and an error that is not an argument rejection is passed
  through untouched.
- **The shape reaches the position gollem can refuse.** gollem stops at the first
  array element that fails and reports its index as a goerr value that is lost
  the same way the parameter name is, so a mixed array is listed per index rather
  than collapsed onto its first entry, and `argShapeMaxDepth` reaches the deepest
  value a tool spec declares (memo's `creates[] -> object -> fields[] -> object ->
  values[] -> string`). Raising a tool spec's nesting means raising that bound.

A model's argument mistake still reaches Sentry through the strategies'
`errutil.Handle`, and that is deliberate: the run recovers, but a model looping
on the same rejected call is exactly what an operator needs to see. Do not
silence it — make the feedback good enough that the loop stops.

Better feedback did not stop that loop, though: the model re-emitted the same
`memo__apply_memo_changes` call, so the memo writes were never applied while the
run still reported success. `toolargs.Coerce` (`pkg/agent/toolargs`) closes the
one case that needs no guessing — a single value sent for an array-typed
argument, which reads as the batch of one the model meant — by wrapping it before
gollem validates. Three properties a change here must preserve:

- **It runs at the strategy, not as a middleware.** A `ToolCallMiddleware` cannot
  see the `ToolSpec` (agentkit resolves it inside `toolCallBase`, after the
  chain), so the coercion sits at the two sites that answer a model's call —
  `react`'s `stepTool` and `planexec`'s `stepPlannerTool`, both calling
  `Session().CallTool` — which is also where `sys.Tools()` is at hand. A third
  strategy must call it too.
- **Only the array case, and only what already contradicts the spec.** Any other
  mismatch is still a rejection explained by `toolArgsFeedbackMiddleware`;
  inventing a reading for one would hide a real mistake behind a wrong guess.
- **The call held on the checkpointed state is not mutated.** `Coerce` returns a
  new arguments map, so a replayed transition coerces the same original again.

What remains open is the run's own visibility: a rejection the model cannot
repair is still absorbed as that call's response, so the turn completes
successfully with the work undone, and only the Sentry report says otherwise.

## Agent runtime: a failed tool call says why it failed

The same gap exists one layer out, for a tool that ran and failed. A strategy
answers the call with `err.Error()` (`pkg/agent/toolcall`), and goerr renders a
chain as `message: message: message` while keeping everything attached with
`goerr.V` outside that string. So the diagnostic half of a failure never reached
the model: a Jira search rejected for a malformed JQL came back as "Jira API
returned non-2xx", while Jira's own "Error in the JQL Query: Expecting either
'OR' or 'AND' but got ..." sat in a `body` value only Sentry saw (ARGUS-96).
A model that cannot see why its query was refused re-emits the same query.

`toolErrorValuesMiddleware` (`pkg/agent/kernel/middleware.go`) renders
`goerr.Values(err)` — which merges the whole chain — as indented `key=value`
lines appended to the message. Five properties a change here must preserve:

- **It is registered AFTER `toolCallMiddleware`**, so it runs inside the trace
  bracket and the timeline and archive record the same message the model was
  given. Pinned by `TestAFailedToolCallTellsTheModelWhyItFailed`.
- **It wraps, it does not replace.** `errors.Is` must keep holding for every
  sentinel a caller discriminates on — `agentkit.ErrLimitExceeded` above all,
  which `react`'s `stepTool` reads to stop a run at a closed budget.
- **An argument rejection is left alone.** `toolArgsFeedbackMiddleware` owns that
  error class, and the only value agentkit attaches there is the tool name
  gollem's message already states. The two middlewares cover disjoint classes.
- **Values are redacted before rendering, by the project's own policy plus a
  key-name rule.** This line goes to the LLM provider and into the Slack thread,
  not just to an operator's sink, so the shared policy (`logging.RedactOptions`,
  the `masq:"secret"` tag and the `Authorization` field name) is applied and any
  key whose NAME reads as a credential is redacted outright. A false positive
  costs one diagnostic; a false negative puts a credential at a third party.
  Extend the policy in `logging.RedactOptions` so both sinks move together.
- **One value per line, single-line strings rendered raw.** The values that
  matter are API error bodies and queries, full of commas and quotes of their
  own; JSON-encoding them hands the model an escaped document to unpick, and a
  comma-separated list gives it no reliable separator. Both bounds
  (`errorValueMaxLen`, `errorValuesMaxLen`) cut on a rune boundary via
  `runtrace.Truncate`, because a broken rune would reach the model, the run
  timeline and Sentry alike.

Note what this does NOT change: the failure still reaches Sentry through the
strategies' `errutil.Handle`, for the same reason the argument rejection does.

**What it DID change, and the rule that follows: on a tool-call path there is no
longer such a thing as an operator-only diagnostic.** A value attached with
`goerr.V` to an error a tool returns is shown to the model, sent to the LLM
provider, and reproduced in the Slack thread. So:

- **Do not attach anything the model must not see.** Attach an identifier, a
  status, a size, a shape — not a credential, and not content that some other
  control was supposed to gate.
- **Check the site's comments when you change what it attaches.** Two places had
  written down the opposite assumption in prose and were relying on it:
  `pkg/agent/tool/notion/client.go` said in as many words that `err.Error()` is
  "the whole of what the agent is told", and `pkg/agent/tool/webfetch`'s analyze
  attached the screening model's raw output — which echoes the fetched page back
  inside its `markdown` field — so a screening reply that failed to parse handed
  the outer model the very body the screen exists to gate. That one is a control
  bypass, not a leak of diagnostics, and it is why this rule is stated positively
  rather than left implicit. Pinned by the analyze test in
  `pkg/agent/tool/webfetch/webfetch_test.go`.
- **The redaction is a backstop, not the boundary.** It matches value KEY names
  and the nested fields `logging.RedactOptions` covers; it does not read inside a
  string. A secret in the middle of a response body would pass through it.

## Agent runtime: no duplicated side effects (NON-NEGOTIABLE)

Every `Kernel.Serve` call in this application MUST pass
`agentkernel.NoDuplicateSideEffects()`.

An agentkit transition performs its effect and is checkpointed afterwards, so a
claim that dies in between leaves a Process whose last checkpoint still asks for
the call that already happened. agentkit's default permits three such takeovers
to re-run it. This application's tools cannot survive that: `core__create_action`,
`case__update_case`, the memo / knowledge creators and `slack__post_message` all
take effect on the first call and carry no idempotency key, so a replay means a
second Action, a second post, a second record.

The option sets the bound to 0, which fails the run instead of re-running it —
the same outcome the pre-agentkit runtime produced for a crashed turn, so nothing
regresses. It may be relaxed only once **every** side-effecting tool is idempotent
under a replayed `(ProcessID, FunctionCall.ID)` pair; that is a change to the tool
contract, not a Serve-option tweak.

The same reasoning is why a new side-effecting tool must not be written assuming
"it runs at most once".

## Agent runtime: the completion handler is best-effort (KNOWN GAP)

agentkit calls `WithOnFinish` once, after the terminal transition has committed,
and does not retry it (ADR-0014). A process that dies between the commit and the
call loses whatever the handler was going to do. Every durable host's outward
work is in that handler: the Slack reply, the case creation, the run-log close.

For a REPLY this is parity, not a regression — the pre-agentkit runtime posted
in-process, so the same crash lost the same reply.

For a JOB it is a real, if narrow, widening. `runtrace.FinishRun` is what calls
`JobRun.RecordRun`, which advances `LastRunAt` and materialises the summary the
case agent page lists. Lose the handler and the run log stays RUNNING, the
summary keeps its previous timestamp, and the scheduler treats the Job as still
due — so a run whose work actually completed can be run again. The pre-agentkit
path had the same outcome for a crash DURING the run; what is new is the sliver
between "the agent finished" and "the record says so".

Closing it needs the terminal transition and the record of what to do next to
commit together — an outbox row written in the same commit, drained by a worker,
keyed on `(ProcessID, operation)` so a redelivery is a no-op. That is the
"outbox-backed delivery" item, and it is NOT done. Until it is:

- **Do not add a new side effect to a completion handler and call it durable.**
  It is best-effort, exactly like the reply beside it.
- **Do not treat `NoDuplicateSideEffects()` as covering this.** That option bounds
  unclean reclaims of a transition; it says nothing about the handler that runs
  after the transition committed.

## Agent runtime: the subject is free before the handler finishes (KNOWN GAP)

A Process holds its subject only while it is open. agentkit releases it at the
terminal commit and calls the completion handler *after*
(`fireFinish`, worker.go) — and `FindOpenProcessBySubject`, which is what makes a
second `Spawn` report busy, matches pending/running/waiting only.

So "one live run per thread" is exact for the RUN, and not exact for the run's
outward work: from the terminal commit until the handler returns, a new trigger on
the same thread can Spawn successfully while the previous turn is still applying
its case update, its Slack post, and its Session write. Two consequences:

- The new turn may read a Case or Session the previous handler has not written yet.

This is NOT the best-effort-handler gap above: no crash is involved.

The **clobbering** half of it is closed: no path that runs before or after a turn
writes the whole Session document any more. Two things follow for anyone changing
this area:

- **Every Session write around a turn is field-scoped, never a `Session.Put`.**
  That covers both sides — the spawning call and the completion handler — because
  either can be running while the other writes the same row. The repository
  operations are `AdvanceLastMention` (monotonic), `AssociateProposal`,
  `StampLastAction`, `SetPendingQuestion` and `BindCase`; add another one rather
  than reaching for `Put`. A full write there is a bug even though it looks like it
  happens "before" or "after" the run.
  The single exception is `persistCreateSession`, the create flow's FIRST write:
  the row may not exist yet, so a field-scoped update has nothing to update. Its
  residual risk is recorded at the function.
- **Nothing a completion handler needs may be read back from mutable shared
  state.** The handler runs after the turn, and by then the thread may point
  somewhere else. Anything that identifies what THIS run was working on travels on
  the run, in `Scope` — `Scope.ProposalID` is the worked example: reading the
  Session's `ProposalID` instead let one turn's draft receive another turn's
  result. Corollary: a value that identifies the run's target must be written to
  shared state only AFTER the Spawn is accepted, or a refused turn leaves the
  thread pointing at work nobody is doing.
- **Closing the REST of the gap means the outward work runs under the subject** — as
  a final durable transition, or as a child Process the subject still covers — not
  by adding locks around the handler. Narrow writes stop one turn from erasing
  another's record; they do not stop a new turn from reading state the previous
  handler has not written yet. Until that is done, do not describe the thread as
  serialised end to end.

## Agent runtime: a parallel tool-call turn is answered in ONE call (NON-NEGOTIABLE)

A model turn holding N function calls must be answered by a SINGLE turn holding N
function responses. Gemini rejects the request outright otherwise — "Please ensure
that the number of function response parts is equal to the number of function call
parts of the function call turn" — and once the conversation holds the wrong shape
every later call in the run is rejected, so the turn dies with no answer.

Since agentkit v0.3.0 and gollem v0.28.2 the grouping is done for us, at both
layers: `agentkit.Session().CallTool` appends a result into the TRAILING tool
message when there is one rather than starting a new message (`session.go`), and
gollem's Claude and Gemini converters merge a run of consecutive tool messages
into one provider turn (`internal/convert`, `MergeConsecutiveToolMessages`).
Grouping by run is exact rather than convenient: a further tool call cannot appear
without an assistant message between, so consecutive tool messages always answer
the same call turn.

Both Strategies therefore do the same thing, and a new one must too:

- Answer each call the MODEL asked for through **`Syscalls.Session().CallTool`**,
  one per transition, which runs the tool and appends its result to the managed
  conversation. A FAILED call is answered too — agentkit records the failure as an
  error result, which is what the model reacts to on its next turn. (For a call the
  Strategy makes on its own initiative there is no matching `tool_use`, so that one
  goes through the primitive `Syscalls.CallTool` instead.)
- Send **no input at all** on the call that follows the tool phase. The answered
  calls are the turn the model is waiting on; an input would land as a second user
  turn behind them. This is tracked as a `ToolsAnswered` flag on the checkpointed
  state, cleared only after that call succeeded, because a failed transition is
  retried from the checkpoint and has to continue the same way.
- Do NOT run the calls through the primitive `Syscalls.CallTool` and hold their
  results on the state to send as the next `Generate`'s inputs. That was the
  previous design, from before the grouping existed upstream; it buys nothing now
  and leaves the committed conversation a transition behind what actually ran.

This does not change the transition split: one transition is still one LLM call or
one tool call.

Pinned by `TestParallelToolResultsAreReportedInOneTurn` (react) and
`TestParallelPlannerToolCallsAreAnsweredInOneTurn` (planexec). Both read the
answers off the CONVERSATION each call was seeded with, not off its inputs — which
is where a regression to per-turn results or to unanswered calls would show.

**Two consequences, both learnt from live rejections.** Both surface as the same
unhelpful message — "Requests ending with a model turn are not supported" — and
both repeat on every retry, so a run that hits either produces nothing.

1. **The call answering a tool round carries NOTHING ELSE.** Text sent with the
   results makes the provider stop recognising them as the answer to the model's
   call, so it reports the request as ending on the model turn. Whatever text was
   waiting keeps waiting; anything that must reach the model NOW goes in the system
   prompt for that call, which is rebuilt per call from the state. That is where
   planexec's tool-allowance notice and react's budget notice live — the notices
   fire exactly when a call is likely to be answering a tool round, which is why
   they cannot be inputs — and it is also where planexec's FINAL prompt goes when
   the terminal call answers one, since a budget notice can make that the only
   terminal call the run ever makes. "Waiting keeps waiting" is only correct for
   text a LATER call will send anyway; text that has no later call must be moved,
   not dropped.
   The notice has exactly ONE route: `plannerPrompt` puts it in the system prompt
   of every planning and terminal call. Do not also prepend it to a user prompt —
   that says it twice on the calls that send one, and silently drops it on the
   calls that do not.
2. **A call must never send an empty input UNLESS the conversation already answers
   the model's last turn.** gollem appends no user content for an empty input
   (`llm/gemini/client.go`, `len(parts) > 0`), so with nothing behind it the request
   ends on the previous model turn. After a tool phase the opposite holds — the
   results are behind it, and sending nothing is the correct and documented way to
   continue. planexec's `plannerInput` and react's `stepGenerate` decide exactly
   that, and fall back to a short continuation for every other case.

Pinned by `TestAPlanningCallSendsOneWellFormedTurn`,
`TestTheToolAllowanceIsToldInTheSystemPrompt`,
`TestTerminalCallIsToldWhenItsToolAllowanceIsSpent`,
`TestPlannerToolResultsReachTheTerminalCall` (planexec) and
`TestBudgetNoticeReachesTheModel` (react). The clear-only-after-success half is
pinned by `TestAnsweredToolCallsSurviveAFailedPlanningCall` (planexec) and
`TestAnsweredToolCallsSurviveAFailedGenerate` (react), which fail the call that
continues from an answered round and check the retry sends the same shape.

## Budget

The budget is **per Process**, enforced by the runtime rather than counted by
each host. There are two tiers, and they are bounded by different quantities.

**The root tier is bounded by MONEY.** `budget.Root` declares only `MaxSteps` and
`NoticeRatio`; what a root run may SPEND is a `budget.RunLimit` — a
`pricing.NanoUSD` ceiling plus the `pricing.Rate` of the model the run generates
through — resolved PER RUN by a `budget.LimitResolver`. A token figure cannot do
this job: which model a run uses is operator configuration (a Job names one), and
the models a deployment may name differ in price by more than an order of
magnitude, so any token ceiling is right for one of them and wrong for another.
`MaxSteps` stays, but it is not a spend limit — it stops a run that never
terminates even while it costs almost nothing.

**The sub-agent tier keeps tokens.** `budget.Config` (steps + input tokens +
output tokens + notice ratio) is unchanged and now applies to the Task tier only:
it bounds one investigation's share of a turn, and the money ceiling on the root
already bounds what the whole tree may cost. Input and output stay separate
because output tokens cost several times what input tokens do.

Three properties of the money path a change here must preserve:

- **The resolver reads the run's metadata, and the price and the client come from
  the same definition.** `agentkernel.ModelPolicy` answers both "which model does
  this run generate through" (by rewriting `GenerateRequest.Role` in
  `modelRoleMiddleware`) and "what is it judged against" (`Resolve`). They are one
  value on purpose: resolving them separately is how a run ends up generating with
  a cheap model while metered at an expensive one's rate. A run whose reference
  name is no longer defined falls back to the DEFAULT model on both halves.
- **A run whose ceiling cannot be resolved is stopped, not run unbounded.**
  `Root.Limiter` answers `LimitStop("this run has no priced budget")` for a nil
  resolver, a non-positive budget, or a rate that prices input or output at zero.
  Startup validation (`ModelPolicyInput.Validate`,
  `config.ValidateJobModels`) makes that unreachable for a configured deployment.
- **Cost is computed from the four token counts**, not from a total:
  `Rate.Cost` charges cache reads at their discount and cache writes at their
  premium, and clamps the uncached remainder at zero so a provider reporting input
  exclusive of its cache components cannot credit the budget.

`NoticeRatio` is the fraction of the budget or the step ceiling a run may spend
before it is told to conclude. What is left is the **reserve** — a tenth by
default — and the rest of the run is TWO moves, because a turn's side effects
happen through tools and nowhere else: a run cut off before its `case__*` call or
its post has done nothing, whatever it managed to say.

Five consequences to keep in mind:

- **A ceiling produces `LimitKindStop`, the notice threshold produces
  `LimitKindNotice`, and Stop wins when both apply.** Stop is read at the
  transition boundary and fails the Process WITHOUT calling `Step`
  (`worker.go`, `driveClaim`), so nothing can run after it — the reserve is the
  only place a run gets to finish what it started, and it exists before the Stop
  rather than after it.
- **A Strategy that observes a notice makes two moves, and the instruction flips
  between them.** The first says "THIS turn is your final tool call, do not write
  your result yet"; the call carrying that tool's result says "call nothing more,
  write your result now, briefly". Both `react` and `planexec` implement it, each
  with its own wording (`reserveInstruction` / `reserveSpentInstruction`), because
  one asks for an answer and the other for a terminal output. planexec also skips
  further fan-out: `stepPlan` and `stepReplan` divert to `phaseFinal` at their top
  without generating.

  Two properties a change here must preserve:

  - **The first move offers no alternative.** The conditional it replaced ("if
    something is outstanding call it, otherwise answer now") let a model skip
    straight to the answer with the side effect the task was for unperformed,
    which is what the reserve exists to prevent.
  - **Neither instruction is a gate.** A model that writes its result instead of
    calling a tool ends the run there and must NOT be re-prompted: nothing can
    make a model call a tool (agentkit's `WithTools` only appends, so the tools
    cannot be withheld), and a run with nothing left to call would spend the whole
    reserve being asked again. A call made past the one reserve round is still run
    and answered, for the reason § "a parallel tool-call turn is answered in ONE
    call" gives. Pinned by `TestTheReserveFirstMoveIsANudgeNotAGate` and
    `TestTheReserveBoundIsANudgeNotAGate` (react).
- **The notice reaches the model through the SYSTEM prompt, never as an input** —
  see § "a parallel tool-call turn is answered in ONE call" for why the call
  answering a tool round sends no user turn to carry it. In planexec that route is
  `plannerPrompt`, and the reserve instruction REPLACES the tool-allowance notice
  there rather than joining it: the two would arrive together saying opposite
  things.
- **A sub-agent's spend is charged to its parent as well as to itself.** A child
  gets its own Process and its own ceiling (the Task tier of
  `agentkernel.Budgets`), AND agentkit folds the finished child's whole `Metrics`
  into the parent at the child's terminal commit (`worker.go`, `reportToParent`:
  `pClone.Metrics = pClone.Metrics.add(child.Metrics)`). So the ROOT ceiling bounds
  the **entire subtree**, not the planner's own transitions.

  Two things follow, and both have already bitten:

  - **A root ceiling must be sized to cover every child a turn can spawn.** Plan
    validation allows up to 5 tasks per round and a turn may run several rounds,
    so a root ceiling near a single task's ceiling means the run dies of its
    children's spend with the planner barely started.
  - **The ceiling is crossed in jumps, not one step at a time.** The fold lands in
    a single write when the child finishes, so the parent can go from well under
    the ceiling to far past it between two of its own transitions. The
    per-transition `Limit` check (`worker.go`, `driveClaim` → `callLimit`) cannot
    prevent that; it only reports it afterwards. A "step budget exhausted (122/64)"
    is not a bug in the accounting — it is this fold.

  Pinned by `TestAChildsStepsAreChargedToItsParent` and
  `TestAParentIsStoppedByItsChildrensSpend` in
  `pkg/usecase/agent/planexec/strategy_test.go`. There is no per-turn total
  sub-agent count and none should be reintroduced: the root ceiling IS that bound.
- **The budget spans the whole task, not one turn.** Because the counters live on
  the Process, a run resumed after a question continues against the same
  ceilings; a fresh trigger spawns a fresh Process with fresh ones.

Per-round fan-out is separately bounded by plan validation (≤ 5 tasks per
phase).
