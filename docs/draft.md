# Case Drafts (Save-as-Draft from Slack)

Hecatoncheires lets users save a half-written case from the Slack
`/cmd` creation modal and come back to it later from the web. The
saved entry — a **Case Draft** — has its own pre-assigned case number,
its own author-scoped listing, and a single-click "Submit" path that
promotes it to a regular OPEN case.

## Lifecycle

Cases follow a simple linear lifecycle:

```
                        SubmitDraft
                            │
              ┌── DRAFT ────┴────▶ OPEN ◀──┬── reopen ── CLOSED
              │                            │
              └── DiscardDraft (delete)    └── closeCase
```

* `DRAFT` — saved from Slack; visible only to the reporter, hidden
  from the default `cases` listing, no Slack channel binding, no
  notifications.
* `OPEN` — submitted; behaves exactly like a case created directly
  via the modal's Submit button (channel created, invites posted,
  bookmark added, welcome message rendered).
* `CLOSED` — closed via `closeCase`; can be re-opened via
  `reopenCase`. `closeCase` / `reopenCase` reject `DRAFT` cases.

## Slack: Save as Draft

The `/cmd` creation modal exposes a **Draft mode** checkbox inside
the **Options** group, next to the existing **Private case** checkbox.
When the user ticks **Draft mode** and presses the modal's footer
**Create** button:

1. Slack delivers the view_submission to `HandleCaseCreationSubmit`.
2. The handler detects the `draft` option in the Options checkbox
   group and routes the request through
   `CaseUseCase.CreateDraft` instead of `CaseUseCase.CreateCase`.
3. The case is persisted with `status: DRAFT`, reporter set to the
   submitting Slack user, and no Slack channel is created.
4. An ephemeral message is posted in the originating channel
   pointing to the web Drafts page (`/ws/{wsId}/drafts`).
5. Slack auto-closes the modal as usual for view_submission.

Choosing not to tick **Draft mode** runs the standard `CreateCase`
path (channel created, invites posted, bookmark added, welcome
message rendered). The two flags are independent: ticking both
**Private case** and **Draft mode** yields a private draft.

No new slash command is added, and there is no longer a separate
**Save as draft** button in the modal body — the legacy block_actions
handler (`HandleSaveAsDraftClick`) is kept for backward compatibility
with any in-flight callbacks emitted before the layout change but is
no longer surfaced through the modal.

## Web: Drafts page

Logged-in users see a **Drafts** entry in the workspace sidebar that
links to `/ws/{wsId}/drafts`. The list shows the user's own drafts
(reporter scope is enforced server-side; another user's drafts are
not surfaced through any listing or single-case fetch).

Each row links to `/ws/{wsId}/drafts/{id}` — a read-only detail view
that shows the saved title, description, privacy flag, and any
workspace-custom field values entered before saving. The detail view
exposes two actions:

* **Submit** — promotes the draft to OPEN by calling
  `submitDraft(workspaceId, id)`. Submit requires a non-empty title.
  After a successful submit, the user is taken to the regular case
  detail page (`/ws/{wsId}/cases/{id}`).
* **Discard** — permanently deletes the draft via
  `discardDraft(workspaceId, id)`. Only the reporter can discard.

Draft *editing* is intentionally out of scope for the initial
release: users who want to revise the draft re-open the Slack modal
to start fresh, or pick up the saved entry as-is and add details
after Submit.

## GraphQL surface

```graphql
# DRAFT is excluded from cases() unless explicitly requested.
enum CaseStatus { DRAFT OPEN CLOSED }

extend type Query {
  # Author-scoped: returns only the auth-context user's own drafts.
  drafts(workspaceId: String!): [Case!]!
}

extend type Mutation {
  # Promotes a draft to OPEN and triggers Slack channel creation etc.
  submitDraft(workspaceId: String!, id: Int!): Case!
  # Permanently deletes the caller's own draft.
  discardDraft(workspaceId: String!, id: Int!): Boolean!
}
```

The general `cases(workspaceId, status)` listing **excludes** DRAFT
by default. Passing `status: DRAFT` works but is enforced
author-scoped at the resolver — strangers see an empty list.

## Storage / scaling notes

* No new Firestore collection is introduced: drafts live in the same
  per-workspace `cases/{caseID}` collection as everything else and
  share the same auto-increment counter, so case numbers stay
  contiguous across draft/open/closed states. Submit only flips
  `Status` from DRAFT to OPEN — the case ID never changes.
* DRAFT exclusion in the default `cases` listing is done with a
  single-field `Status in [OPEN, CLOSED]` filter; no new composite
  Firestore index is required.

## Access control

* Only the case's reporter (the Slack user who clicked Save as Draft)
  can see, submit, or discard a draft.
* Drafts skip private-case channel membership checks because there
  is no Slack channel until Submit — the reporter check is
  sufficient.
* `closeCase` and `reopenCase` refuse to operate on DRAFT cases and
  return `ErrCaseIsDraft`. Drafts must Submit first (or be
  discarded).
