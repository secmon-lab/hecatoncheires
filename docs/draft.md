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

The `/cmd` creation modal carries a **Save as draft** button alongside
the modal's Submit. Clicking it:

1. ACKs the block_actions interaction immediately.
2. Asynchronously persists the modal's current state as a Case with
   `status: DRAFT`, reporter set to the clicking Slack user.
3. Swaps the modal in place with a small "Saved" splash so the user
   sees an unambiguous confirmation.
4. Posts an ephemeral message in the originating channel pointing to
   the web Drafts page (`/ws/{wsId}/drafts`).

No new slash command is added — Save as Draft is just a button on the
existing modal.

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
