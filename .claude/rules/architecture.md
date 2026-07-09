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
consistently in code, comments, specs, and reviews. There are three
nested levels:

- **Round** — ONE iteration of the planner loop: a single planner /
  replan LLM call plus the work that round dispatches (the `investigate`
  phase's sub-agents, a validation/commit re-emit, etc.). This is one
  iteration of the `for {}` in `planexec.Runner.Run`.
- **Turn** — ONE `runner.Run` / `RunTurn` invocation: from agent start
  until it stops to wait for user input or otherwise completes. A turn
  runs *many rounds*. A turn ends on a terminal outcome:
  - the planner asks the user (`question` / `OnQuestion` →
    `QuestionResult{Terminate:true}`) — the turn closes and waits; the
    user's reply / form-submit starts the **next** turn;
  - a terminal action commits (e.g. case create / materialize);
  - fallback (loop budget exhausted, internal error).
  A turn is NOT a single loop iteration; it spans many rounds.
- **Task** — the whole effort (e.g. creating one case), possibly spanning
  **multiple turns** separated by `question`s. (No stricter name yet;
  "task" is fine.)

**Why `question` ends the turn (not `Terminate:false`):** holding the
per-thread turn-lock open while waiting minutes/hours for a Slack submit
is not viable under horizontal scaling; the pending question is persisted
(`Session.PendingQuestion`, shared backend) and the answer arrives on a
fresh dispatched event that starts a **new turn**.

**Entry points & final output.** planexec is a generic plan-execute
framework — it knows nothing about `case` and performs no side effects
itself. It exposes three package-level entry functions (NOT `Runner`
methods, since Go methods cannot be generic):

- `Run[T Validatable](ctx, runner, req)` — structured turn. After the planner
  finalizes, planexec generates the terminal JSON, decodes it into `T`, calls
  `T.Validate()`, and regenerates on failure (bounded by `finalOutputMaxRetry`;
  gollem's schema check verifies shape only, so `Validate()` is where domain
  invariants live). Returns `*RunResult[T]` with the validated value in `.Data`.
- `RunText(ctx, runner, req)` / `ResumeText(ctx, runner, req)` — plain-text
  turn / resume. The reply is in `RunResult.Text`.

Side effects (closing a case, posting a message, persisting the entity, …) are
performed either by the **sub-agents' tools inside the loop** or by the host
**after** the turn from the returned `*T` — never by planexec itself, and never
inside the loop as a commit hook. The old `RunRequest.OnFinalize` /
`FinalOutputSchema` commit hooks are gone.

`Run[T]` does accept optional `finalizers ...func(*T) error` that run after
`T.Validate()` inside the final-output regeneration loop, but they are
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
`RunRequest.AllowDirect`, the planner may answer a *genuinely trivial*
request on round 1 without any investigation: instead of `tasks` it emits a
`direct` payload (an optional tool-id subset), and the runtime replies in a
single tool-enabled ReAct loop, returning plain text in `RunResult.Text`
with `RunResult.Direct == true`. It is strictly a fast path for
respond-style replies: even a `Run[T]` turn returns `.Text` (not `.Data`) on
the direct path, because side-effecting terminal actions are by definition not
"trivial" and must go through the normal `tasks` → replan → `finalize` loop.
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
A host that wants a per-call timeline supplies it via
`planexec.RunRequest.TraceHandler`. planexec combines it (`combineTrace`
→ `trace.Multi`) with each agent's own trace sink and wires the result
into **every** agent the run drives — the planner, each parallel
sub-agent, the direct reply, and the final synthesis. Wiring it into only
some agents silently drops the rest from the timeline.

This is exactly how the `planexec`-Job empty-timeline bug happened: the
handler was built by `JobRunner.Run` and handed to the executor, but the
planexec executor never forwarded it, and planexec drove its agents with
only the separate archive recorder. The run succeeded, the system prompt
showed (it is stored on `JobRunLog` before execution), and the timeline
stayed empty. `simple` Jobs were unaffected because the single-loop
executor wires the handler directly via `gollem.WithTrace`. When you add
a new agent execution to planexec, wire the host handler into it too.

### Mention-triggered runs on the case agent page

`JobRunLog` / `JobRunEvent` are NOT Job-only: the case agent page
(`caseJobRunLogs`) lists **every** case-scoped agent run through one read
path. Post-creation Slack mentions handled by the `casebound` (channel-mode,
direct gollem) and `threadcase` (thread-mode, planexec, `ModeMention` only)
hosts record the same records via `runtrace.Recorder`. They are not
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
- Keep the existing durable trace sink. casebound feeds the Cloud Storage
  recorder and `runtrace.Recorder.Handler()` through `trace.Multi`; threadcase
  passes the handler via `planexec.RunRequest.TraceHandler` (planexec already
  combines it with its archive recorder). Do not replace the archive trace.
- `ModeCreate` (creation-time materialize) is excluded — the requirement is
  post-creation mentions.
- Trace recording is observability: `Open`/`Finish`/event failures are
  non-fatal (`errutil.Handle`) and must never fail the mention turn.

## Budget

The budget model is the combination of **two** controls — there is NO
running "total sub-agent task count" across a turn:

1. **Round-count limit** — `PlannerLoopMax` bounds the **number of rounds
   in a turn** (`budget.canPlannerCall()`). Planner / replan output that fails
   validation is retried within this same pool. (Final-output regeneration in
   `Run[T]` — decode/`Validate()` retries — is bounded separately by
   `finalOutputMaxRetry` and does NOT consume planner rounds.) This is the main
   loop guard.
2. **Per-sub-agent budget** — `SubAgentLoopMax` is the inner gollem loop
   limit granted **fresh to every sub-agent** (so the sub-agent budget
   naturally recovers per round).

Per-round fan-out is already bounded by plan validation (≤ 5 tasks per
phase), so total sub-agent work is naturally bounded by
`PlannerLoopMax × (≤5) × SubAgentLoopMax` without a separate total cap.

- **Do NOT reintroduce a per-turn total sub-agent count.** The legacy
  `SubAgentMaxPerTurn` / `subAgentUsed` accumulator is being retired —
  the round-count limit plus the per-sub-agent budget are the only knobs.
- `PlannerLoopMax` is a loop bound, NOT "the budget". When someone says
  "the budget" in this runtime they mean the sub-agent (investigation)
  budget, which recovers per round.

`newBudget(BudgetConfig)` is constructed once per `runner.Run` (per
turn); crossing a `question` boundary starts a fresh turn.
