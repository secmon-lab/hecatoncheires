# Private Case Access Control

When adding new data models, queries, mutations, or sub-resolvers that relate to Cases or are children of Cases (e.g., Actions, Knowledges, SlackMessages, or any future entities), you MUST handle private case access control:

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
