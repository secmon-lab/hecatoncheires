import { gql } from '@apollo/client'

// Shared selection sets. The fragments form a linear chain
// (CaseListFields ⊃ CaseMutationFields ⊃ CaseUserFields) and each
// operation interpolates exactly one root fragment constant, so a
// document never contains duplicate fragment definitions.

const CASE_USER_FIELDS = gql`
  fragment CaseUserFields on SlackUser {
    id
    name
    realName
    imageUrl
  }
`

// The selection set returned by case mutations (create / update /
// close / reopen). Deliberately narrower than CaseListFields: the
// mutation responses historically never carried slackThreadTS /
// isThreadBound / boardStatus, and this refactor keeps every
// operation's field set byte-equivalent to what it was.
const CASE_MUTATION_FIELDS = gql`
  ${CASE_USER_FIELDS}
  fragment CaseMutationFields on Case {
    id
    title
    description
    status
    isPrivate
    isTest
    accessDenied
    reporterID
    reporter {
      ...CaseUserFields
    }
    assigneeIDs
    assignees {
      ...CaseUserFields
    }
    slackChannelID
    createdAt
    updatedAt
    fields {
      fieldId
      value
    }
  }
`

const CASE_LIST_FIELDS = gql`
  ${CASE_MUTATION_FIELDS}
  fragment CaseListFields on Case {
    ...CaseMutationFields
    slackThreadTS
    isThreadBound
    boardStatus
  }
`

export const GET_CASES = gql`
  ${CASE_LIST_FIELDS}
  query GetCases($workspaceId: String!, $status: CaseStatus) {
    cases(workspaceId: $workspaceId, status: $status) {
      ...CaseListFields
    }
  }
`

export const GET_CASE = gql`
  ${CASE_LIST_FIELDS}
  query GetCase($workspaceId: String!, $id: Int!, $actionsFilter: ActionArchiveFilter) {
    case(workspaceId: $workspaceId, id: $id) {
      ...CaseListFields
      channelUserCount
      slackChannelURL
      actions(filter: $actionsFilter) {
        id
        title
        status
        assigneeID
        assignee {
          ...CaseUserFields
        }
        dueDate
        archived
        archivedAt
        createdAt
        updatedAt
      }
    }
  }
`

export const GET_CASE_MEMBERS = gql`
  ${CASE_USER_FIELDS}
  query GetCaseMembers($workspaceId: String!, $id: Int!, $limit: Int, $offset: Int, $filter: String) {
    case(workspaceId: $workspaceId, id: $id) {
      id
      channelUserCount
      channelUsers(limit: $limit, offset: $offset, filter: $filter) {
        items {
          ...CaseUserFields
        }
        totalCount
        hasMore
      }
    }
  }
`

export const CREATE_CASE = gql`
  ${CASE_MUTATION_FIELDS}
  mutation CreateCase($workspaceId: String!, $input: CreateCaseInput!) {
    createCase(workspaceId: $workspaceId, input: $input) {
      ...CaseMutationFields
    }
  }
`

export const UPDATE_CASE = gql`
  ${CASE_MUTATION_FIELDS}
  mutation UpdateCase($workspaceId: String!, $input: UpdateCaseInput!) {
    updateCase(workspaceId: $workspaceId, input: $input) {
      ...CaseMutationFields
    }
  }
`

export const DELETE_CASE = gql`
  mutation DeleteCase($workspaceId: String!, $id: Int!) {
    deleteCase(workspaceId: $workspaceId, id: $id)
  }
`

export const CLOSE_CASE = gql`
  ${CASE_MUTATION_FIELDS}
  mutation CloseCase($workspaceId: String!, $id: Int!) {
    closeCase(workspaceId: $workspaceId, id: $id) {
      ...CaseMutationFields
    }
  }
`

export const REOPEN_CASE = gql`
  ${CASE_MUTATION_FIELDS}
  mutation ReopenCase($workspaceId: String!, $id: Int!) {
    reopenCase(workspaceId: $workspaceId, id: $id) {
      ...CaseMutationFields
    }
  }
`

export const SYNC_CASE_CHANNEL_USERS = gql`
  mutation SyncCaseChannelUsers($workspaceId: String!, $id: Int!) {
    syncCaseChannelUsers(workspaceId: $workspaceId, id: $id) {
      id
      channelUserCount
    }
  }
`

// ASSIGN_CASE / UNASSIGN_CASE mutate the assignee set by delta (add / remove
// the listed users) instead of replacing the whole list via updateCase. The
// server applies the change atomically, so concurrent edits cannot clobber
// one another. Assignees can ONLY be changed through these — updateCase /
// submitDraft no longer accept assigneeIDs.
export const ASSIGN_CASE = gql`
  ${CASE_USER_FIELDS}
  mutation AssignCase($workspaceId: String!, $id: Int!, $userIDs: [String!]!) {
    assignCase(workspaceId: $workspaceId, id: $id, userIDs: $userIDs) {
      id
      assigneeIDs
      assignees {
        ...CaseUserFields
      }
      updatedAt
    }
  }
`

export const UNASSIGN_CASE = gql`
  ${CASE_USER_FIELDS}
  mutation UnassignCase($workspaceId: String!, $id: Int!, $userIDs: [String!]!) {
    unassignCase(workspaceId: $workspaceId, id: $id, userIDs: $userIDs) {
      id
      assigneeIDs
      assignees {
        ...CaseUserFields
      }
      updatedAt
    }
  }
`
