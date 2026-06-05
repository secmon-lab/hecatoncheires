import { gql } from '@apollo/client'

// GET_CASE_STATUS_CONFIG returns the configurable Case status set (the Kanban
// columns) for a thread-mode workspace, or null for channel-mode workspaces.
export const GET_CASE_STATUS_CONFIG = gql`
  query GetCaseStatusConfig($workspaceId: String!) {
    caseStatusConfig(workspaceId: $workspaceId) {
      initial
      closed
      statuses {
        id
        name
        description
        color
        emoji
      }
    }
  }
`

// UPDATE_CASE_STATUS moves a thread-mode case to a new board status (Kanban
// column). The lifecycle status is synced server-side.
export const UPDATE_CASE_STATUS = gql`
  mutation UpdateCaseStatus($workspaceId: String!, $input: UpdateCaseStatusInput!) {
    updateCaseStatus(workspaceId: $workspaceId, input: $input) {
      id
      title
      status
      boardStatus
      isThreadBound
    }
  }
`
