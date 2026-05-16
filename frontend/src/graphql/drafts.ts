import { gql } from '@apollo/client'

// GET_DRAFTS returns the auth-context user's own draft cases in the given
// workspace. Server-side scoping is enforced — the query carries no
// reporter argument because every caller is already authenticated.
export const GET_DRAFTS = gql`
  query GetDrafts($workspaceId: String!) {
    drafts(workspaceId: $workspaceId) {
      id
      title
      description
      status
      isPrivate
      reporterID
      createdAt
      updatedAt
      fields {
        fieldId
        value
      }
    }
  }
`

// GET_DRAFT reads a single draft for the detail view. The server hides
// other users' drafts (and surfaces non-drafts as ErrCaseNotDraft), so the
// frontend just renders `case` and trusts the contract.
export const GET_DRAFT = gql`
  query GetDraft($workspaceId: String!, $id: Int!) {
    case(workspaceId: $workspaceId, id: $id) {
      id
      title
      description
      status
      isPrivate
      reporterID
      assigneeIDs
      createdAt
      updatedAt
      fields {
        fieldId
        value
      }
    }
  }
`

export const SUBMIT_DRAFT = gql`
  mutation SubmitDraft($workspaceId: String!, $id: Int!) {
    submitDraft(workspaceId: $workspaceId, id: $id) {
      id
      title
      status
    }
  }
`

export const DISCARD_DRAFT = gql`
  mutation DiscardDraft($workspaceId: String!, $id: Int!) {
    discardDraft(workspaceId: $workspaceId, id: $id)
  }
`
