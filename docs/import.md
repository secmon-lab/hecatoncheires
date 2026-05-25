# Case Import (YAML)

Hecatoncheires lets a workspace member bulk-create Cases (and the Actions
underneath them) from a YAML file.  Imported Cases are saved in **DRAFT**
state — no Slack channel is created, no notifications fire — so a large
import never trips Slack rate limits or fills up channel lists.  Each
DRAFT can later be promoted to OPEN through the normal SubmitDraft path.

## Workflow

1. **CaseList → [Import]** opens `/ws/:workspaceId/imports/new`.
2. Drop a `.yaml` / `.yml` file (or click to pick one).  The file is
   parsed and validated server-side; an `ImportSession` is persisted in
   Firestore and the page redirects to `/ws/:workspaceId/imports/:id`.
3. Review the **Cases to create** preview.  Issues (missing titles,
   invalid field values, unknown users …) are shown inline with the
   exact YAML path that triggered them.
4. When the session is `valid` (no error-severity issues), press
   **Execute import**.  The server walks the snapshot, calling the
   existing `CreateDraft` / `CreateAction` usecases for each item.
5. The page redraws as the result variant:
   - **APPLIED** – every Case was created.  A summary banner with an
     **Open Cases list** link.
   - **FAILED** – at least one item failed.  The Result list shows
     three per-Case states (`✓ created`, `✗ failed`, `− skipped`) so
     you can tell exactly how far the import got.

## No list view — keep the URL

There is **no Imports list page**.  Sessions live on indefinitely in
Firestore but are only reachable through their session-id URL, and only
by the creator who uploaded them.  Bookmark the page (or note the URL)
if you need to come back to a pending session later.

## YAML schema

The file is described by the JSON Schema rendered (and copyable) on the
upload page.  In summary:

```yaml
version: 1
cases:
  - title: "Suspicious login"
    description: |
      Multiple failed attempts from 10.0.0.1
    isPrivate: false
    assigneeIDs: [U12345678]
    fields:
      severity: high
      source: aws-cloudtrail
    actions:
      - title: "Block source IP"
        assigneeID: U87654321
        dueDate: 2026-06-30T00:00:00Z
      - title: "Notify SOC"
```

- `version` must be `1`.
- `cases[].title` is **required**.
- `cases[].fields` is validated against the workspace field schema.
  Select-type fields must use a defined option ID; required fields must
  be present.  Errors here surface in the preview before execute.
- `cases[].actions[].title` is **required**.
- `dueDate` accepts RFC3339 (`2026-06-30T00:00:00Z`) or `YYYY-MM-DD`.

## Failure semantics

- **No rollback.** If item N fails, items 1..N-1 stay created in
  Firestore.  Because the imported Cases are DRAFTs, nothing leaked to
  Slack — the user can review or delete them from the Cases list.
- **No skipped retry.** Items after the first failure are marked
  `skipped` in the snapshot and not attempted in this run.
- **One-shot session.** A session is `PENDING` until executed; once it
  is `APPLIED` or `FAILED`, it cannot be re-executed.  To retry,
  upload the file again to create a new session.

## Schema drift protection

The workspace's field schema is hashed at upload time and re-checked at
execute time.  If the schema has changed between the two steps,
`executeCaseImport` is refused and the session is annotated with the
specific Cases whose `fields` no longer pass — so the user knows which
values to update before creating a new import.

## API surface

```graphql
type Query {
  caseImport(workspaceId: String!, id: ID!): ImportSession!
}

type Mutation {
  createCaseImport(workspaceId: String!, input: CreateCaseImportInput!): ImportSession!
  executeCaseImport(workspaceId: String!, id: ID!): ImportSession!
}
```

`ImportSession` carries the full normalized snapshot plus per-Case /
per-Action results once execute has run.  See
`graphql/schema.graphql` for the exhaustive field list.
