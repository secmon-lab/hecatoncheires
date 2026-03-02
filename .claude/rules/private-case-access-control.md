# Private Case Access Control

When adding new data models, queries, mutations, or sub-resolvers that relate to Cases or are children of Cases (e.g., Actions, Knowledges, SlackMessages, or any future entities), you MUST handle private case access control:

## Checklist for New Case-Related Features

1. **UseCase layer**: Add `IsCaseAccessible` check using the parent Case's `ChannelUserIDs`
   - Write operations: Return `ErrAccessDenied` if the user is not a channel member
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
