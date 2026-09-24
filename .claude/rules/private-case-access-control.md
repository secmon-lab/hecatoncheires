# Private Case Access Control

When adding new data models, queries, mutations, or sub-resolvers that relate to Cases or are children of Cases (e.g., Actions, SlackMessages, or any future entities), you MUST handle private case access control:

Note: Knowledge is NOT a Case child — it is a workspace-level entity with no CaseID (`pkg/domain/model/knowledge.go`), so it is out of scope for this rule.

## Checklist for New Case-Related Features

1. **UseCase layer**: Add `IsCaseAccessible` check using the parent Case's `ChannelUserIDs`
   - Write operations: Return `ErrAccessDenied` if the user is not a channel member.
     **Do NOT open-code the check.** Route it through the shared gate in
     `pkg/usecase/case_access.go`:
     - `loadCaseForWrite(ctx, repo, workspaceID, id)` — the "Get + access gate" for
       token-driven Case write paths (used by `CaseUseCase` / `MemoUseCase`). A new
       write path that loads a Case must use this so the check cannot be forgotten.
     - `assertCaseWriteAccess(c, actorID, checkAccess)` — the single deny-decision
       function (draft-aware: a private draft falls back to its reporter). Use it
       directly when the actor is not the context token — e.g. the Slack-Actor-aware
       Action paths resolve the actor via `actorForAccess(ctx, actor)` first.
   - Read operations: Filter out or restrict inaccessible items (use `RestrictCase` for Cases, return empty list for child entities)
   - Use `tokenErr == nil` pattern (not hard error) to maintain backward compatibility with system/bot contexts that have no auth token

2. **Resolver layer**: If the parent Case has `AccessDenied == true`, sub-resolvers must return empty results (no access control logic in resolvers, just check the flag)

3. **Tests**: Write tests covering:
   - Member access (should succeed)
   - Non-member access (should be denied/restricted)
   - No auth token context (should bypass access control for backward compatibility)

## Reference Implementation

- Access control helpers: `pkg/domain/model/case.go` (`IsCaseAccessible`, `RestrictCase`)
- UseCase pattern: `pkg/usecase/case.go`, `pkg/usecase/action.go`
- E2E tests: `pkg/controller/http/graphql_test.go` (`TestGraphQLHandler_PrivateCaseAccessControl`)

## Workspace access control

A workspace's `[authz]` Rego policy decides whether a user may use that
workspace at all (`docs/configuration.md` § Authorization Section). It sits in
front of the private-case check above: a user denied the workspace never
reaches a Case in it. The decision is made by `interfaces.WorkspaceAuthorizer`
(`pkg/usecase/workspace_access.go`); a denial is `model.ErrWorkspaceAccessDenied`.
Like the private-case check, a context with no auth token and a bot actor are
not checked.

When adding an entry point:

- **GraphQL**: a `Query` / `Mutation` root field whose workspace is a top-level
  `workspaceId` argument is checked automatically by
  `WorkspaceAccessMiddleware` (`pkg/controller/graphql/workspace_access.go`).
  A root field that carries the workspace any other way (inside an input
  object, derived from another id) is NOT covered and must call
  `AuthorizeCurrentUser` in its usecase.
- **Slack**: once the handler has resolved the workspace and the acting Slack
  user, call `authorizeSlackActor` (`pkg/usecase/workspace_access_slack.go`)
  before doing any work; it posts the denial to the user. Event handlers in the
  dispatcher use `eventActorAllowed`; view submissions answer with
  `deniedModalFor`.
- **Cross-workspace reads** (dashboard, workspace lists, workspace pickers, the
  agent's workspace tools): pass the candidate workspaces through
  `FilterAccessible` / `FilterAccessibleForCurrentUser` instead of checking
  each one ad hoc.
- **Tests**: cover an allowed user, a denied user (assert nothing was written
  and the denial reached the user), and the no-token / bot case.
