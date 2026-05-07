# Action Steps

Action Steps are small, binary-state work items that live under an Action.
They give a single Action the granularity of a checklist — useful when the
Action is large enough that "done" is best expressed as the union of several
intermediate completions (collected logs, identified affected systems, etc.)
without spawning a separate Action for each.

## Behaviour

- A Step has a title and a binary state: **ongoing** (`doneAt == null`) or
  **done** (`doneAt != null`). There are no other states; Action's
  workspace-customisable status set does not apply to Steps.
- Steps are scoped to a single Action and ordered by creation time.
- The WebUI for an Action surfaces a `done/total` progress badge whenever
  the Action has at least one Step. The Kanban card for an Action also
  shows the same `done/total` badge so progress is visible without opening
  the detail modal.
- Renaming a Step is supported (typo correction, rephrasing). The history
  records old and new titles.

## Lifecycle events

The following structural changes are persisted as `ActionEvent` records and
also posted as a context-block thread reply on the Action's Slack message,
so the Action's Activity feed and the Slack thread stay aligned:

| Event                  | `ActionEventKind`         | Slack notification text (EN)                      |
|------------------------|---------------------------|---------------------------------------------------|
| Step added             | `STEP_ADDED`              | `:heavy_plus_sign: {actor} added step "{title}"`  |
| Step removed           | `STEP_REMOVED`            | `:heavy_minus_sign: {actor} removed step "{title}"` |
| Step marked done       | `STEP_DONE`               | `:white_check_mark: {actor} completed step "{title}"` |
| Step reverted to ongoing | `STEP_REOPENED`         | `:arrow_backward: {actor} reopened step "{title}"`  |
| Step renamed           | `STEP_TITLE_CHANGED`      | `:pencil2: {actor} renamed step "{old}" -> "{new}"` |

Both the Activity record and the Slack post are best-effort: if either
fails, the underlying Step CRUD still succeeds and the failure is reported
through `errutil.Handle` (Sentry / structured log) rather than rolled back.

The Slack thread post requires the parent Case to have a `slackChannelID`
and the Action to have a `slackMessageTS`. When either is missing
(e.g. legacy actions whose initial post never reached Slack), the
notification is silently skipped — the Activity record still happens.

## Access control

Step access mirrors the parent Case's privacy:

- For private Cases (`isPrivate == true`), only members listed in
  `channelUserIDs` may add / toggle / rename / delete Steps.
- Read access (the `Action.steps` and `Action.stepProgress` GraphQL
  fields) returns an empty list / `0/0` for non-members instead of
  raising an error, matching the Case-level resolver behaviour.
- System / bot contexts (no auth token in `context.Context`) bypass the
  check, so the agent tool path is unaffected.

## Surfaces

### Web UI

Action detail modal:

- Read-only metadata (`createdBy`, `doneBy`, timestamps) is intentionally
  hidden from the WebUI to keep the surface minimal. Those fields are
  persisted and exposed by GraphQL / agent tools for callers that need
  them.
- Title clicks toggle inline edit. Save: blur or Enter (with IME
  composition guard). Cancel: Escape. Empty / unchanged titles are
  no-ops and do not record a `STEP_TITLE_CHANGED` event.

Kanban card:

- A `done/total` pill appears on each Action card when
  `stepProgress.total > 0`. When an Action has no Steps the badge is
  hidden entirely.

### GraphQL

Schema additions are listed in `graphql/schema.graphql`:

- Type: `ActionStep`, `ActionStepProgress`
- `Action.steps: [ActionStep!]!`, `Action.stepProgress: ActionStepProgress!`
- Mutations: `addActionStep`, `setActionStepDone`, `renameActionStep`,
  `deleteActionStep`
- Inputs: `AddActionStepInput`, `SetActionStepDoneInput`,
  `RenameActionStepInput`, `DeleteActionStepInput`

### Agent tools (gollem)

Available under the `core__` prefix — registered via
`pkg/agent/tool/core/action_step.go` and routed through
`ActionStepUseCase` so tool-driven changes share the same notification +
event-recording behaviour as GraphQL / WebUI changes:

- `core__list_action_steps(action_id)` — returns step list + `done` /
  `total` counters
- `core__add_action_step(action_id, title)`
- `core__set_action_step_done(action_id, step_id, done)`
- `core__rename_action_step(action_id, step_id, title)`
- `core__delete_action_step(action_id, step_id)`

Tool-driven mutations are attributed to `ActorKindSystem`, so Slack
notifications render as "system" rather than mentioning a Slack user.

### Slack interactivity

There is no Slack-side affordance for Step CRUD in the current scope.
All mutations go through GraphQL (WebUI) or agent tools (LLM).

## Implementation pointers

- Domain model: `pkg/domain/model/action_step.go`
- Repository interface: `pkg/domain/interfaces/action_step.go`
- Memory / Firestore backends: `pkg/repository/{memory,firestore}/action_step.go`
- Use case: `pkg/usecase/action_step.go`
- Agent tools: `pkg/agent/tool/core/action_step.go`,
  `pkg/usecase/action_step_tool_adapter.go`
- GraphQL resolvers: `pkg/controller/graphql/schema.resolvers.go`
  (`actionResolver.Steps` / `StepProgress`, `mutationResolver.AddActionStep`
  ...)
- Frontend: `frontend/src/components/StepList.tsx`,
  `frontend/src/graphql/actionStep.ts`

## Firestore layout

Steps are stored as a subcollection under their parent Action:

```
workspaces/{workspaceID}/actions/{actionID}/steps/{stepID}
```

`stepID` is a UUID; per-Action ordering uses the `CreatedAt` field on a
single-field index (no composite index is required).
