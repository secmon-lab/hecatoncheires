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

The obvious implementation violates this. `agentkit.Session().CallTool` appends
each result to the conversation as its own message (`session.go`), and gollem's
Gemini adapter maps one message to one turn (`llm/gemini/convert_message.go`,
`convertMessagesToGemini` — it does not coalesce consecutive tool messages). So a
Strategy that answers a parallel call turn one result at a time splits the one
required turn into N. The pre-agentkit loop never hit this because it passed all
results into one `Generate`, which gollem packs into one turn.

Both Strategies therefore do the same thing, and a new one must too:

- Run each call through the **primitive** `Syscalls.CallTool`, which executes the
  tool (Limit, Metrics and trace unchanged — the session's CallTool calls it
  internally) without touching the conversation.
- Hold the results on the checkpointed state as `toolcall.Response`
  (`pkg/agent/toolcall`), which survives the JSON round trip a checkpoint makes.
  A FAILED tool is held too: the call still has to be answered, and the failure is
  what the model reacts to.
- Report all of them as the inputs of the next `Generate`, then clear them —
  **after** that call succeeded, since a failed transition is retried from the
  checkpoint and dropping them earlier leaves its calls unanswered forever.

This does not change the transition split: one transition is still one LLM call or
one tool call. What changes is only where the result is kept until it is reported.

Pinned by `TestParallelToolResultsAreReportedInOneTurn` (react) and
`TestParallelPlannerToolCallsAreAnsweredInOneTurn` (planexec).

## Budget

The budget is **per Process**, enforced by the runtime rather than counted by
each host. `budget.Config` (`pkg/agent/budget`) declares four numbers and turns
them into an `agentkit.Limiter` the Kernel consults before every transition:

- `MaxSteps` — committed transitions. It is the successor of the old
  `PlannerLoopMax`, but it counts transitions, not planner rounds: one LLM call
  or one tool call is one step.
- `MaxInputTokens` / `MaxOutputTokens` — the two token counts read off
  `Process.Metrics`, so they accumulate across claims and instances.
- `NoticeRatio` — the fraction of any ceiling at which the Strategy is *told* it
  is close, so it can wrap up instead of being cut off.

Three consequences to keep in mind:

- **A ceiling produces `LimitKindStop`, the notice threshold produces
  `LimitKindNotice`, and Stop wins when both apply.** A Strategy that observes a
  notice is expected to skip further fan-out and head for its terminal output;
  planexec does exactly that.
- **Sub-agents get their own Process and therefore their own budget** (the Task
  tier of `agentkernel.Budgets`). There is no per-turn total sub-agent count and
  none should be reintroduced — a parent's `MaxSteps` bounds how many times it
  can fan out, and each child's own budget bounds that child.
- **The budget spans the whole task, not one turn.** Because the counters live on
  the Process, a run resumed after a question continues against the same
  ceilings; a fresh trigger spawns a fresh Process with fresh ones.

Per-round fan-out is separately bounded by plan validation (≤ 5 tasks per
phase).
