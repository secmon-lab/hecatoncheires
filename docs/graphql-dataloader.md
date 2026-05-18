# GraphQL DataLoader

The GraphQL layer uses a request-scoped DataLoader pattern (via
[`github.com/graph-gophers/dataloader/v7`](https://github.com/graph-gophers/dataloader))
to collapse N+1 fetches that arise when a list query renders sub-resolvers
for each row.

## Where it lives

- `pkg/controller/graphql/dataloader.go` — loader definitions, batch
  functions, request-context plumbing
- `pkg/cli/serve.go` — middleware that instantiates one
  `*DataLoaders` per HTTP request before invoking the gqlgen handler
- `pkg/controller/http/graphql_test.go` — the same per-request
  wiring on the test side, so resolver tests exercise the real
  batching path

## Why request-scoped

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

## What gets batched

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

## Calling convention from resolvers

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

### Handling missing keys

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

## Adding a new loader

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

## Why we didn't keep the old "fake DataLoader"

Before this rewrite, `pkg/controller/graphql/dataloader.go` exposed
types named `SlackUserLoader`, `ActionLoader` etc. but each was just a
batch-fetch helper: `Load(ctx, ids)` made one repository call and
returned. Resolvers called it per row (`Load(ctx, []string{singleID})`)
because there was no debounce layer, so a 20-case list page issued 20
`SlackUser.GetByIDs` calls for reporter, 20 more for assignees, and 20
Slack API calls for channel names — even with caching on top. The
graph-gophers loader collapses each of those to one call per request.
