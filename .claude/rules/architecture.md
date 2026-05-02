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
