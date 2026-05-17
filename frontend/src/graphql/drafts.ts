import { gql } from '@apollo/client'

// GET_DRAFTS returns every draft case in the workspace. Drafts are
// workspace-wide so the list isn't filtered by reporter; private drafts
// are the only exception (the server hides them from non-reporters).
//
// Shape mirrors GET_CASES closely so the Case List page can reuse its
// row renderer when the Drafts tab is active.
export const GET_DRAFTS = gql`
  query GetDrafts($workspaceId: String!) {
    drafts(workspaceId: $workspaceId) {
      id
      title
      description
      status
      isPrivate
      accessDenied
      reporterID
      reporter {
        id
        name
        realName
        imageUrl
      }
      assigneeIDs
      assignees {
        id
        name
        realName
        imageUrl
      }
      slackChannelID
      slackChannelName
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

// CREATE_DRAFT persists the in-flight case form payload as a DRAFT case.
// Mirror of CREATE_CASE but every field is optional, so the user can save
// a half-finished form. The server enforces title presence at SubmitDraft
// time, not here.
export const CREATE_DRAFT = gql`
  mutation CreateDraft($workspaceId: String!, $input: CreateDraftInput!) {
    createDraft(workspaceId: $workspaceId, input: $input) {
      id
      title
      status
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
