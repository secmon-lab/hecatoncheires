import { gql } from '@apollo/client'

export const GET_CASES = gql`
  query GetCases($workspaceId: String!, $status: CaseStatus) {
    cases(workspaceId: $workspaceId, status: $status) {
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
      slackThreadTS
      isThreadBound
      boardStatus
      createdAt
      updatedAt
      fields {
        fieldId
        value
      }
    }
  }
`

export const GET_CASE = gql`
  query GetCase($workspaceId: String!, $id: Int!, $actionsFilter: ActionArchiveFilter) {
    case(workspaceId: $workspaceId, id: $id) {
      id
      title
      description
      status
      isPrivate
      accessDenied
      channelUserCount
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
      slackChannelURL
      slackThreadTS
      isThreadBound
      boardStatus
      createdAt
      updatedAt
      fields {
        fieldId
        value
      }
      actions(filter: $actionsFilter) {
        id
        title
        status
        assigneeID
        assignee {
          id
          name
          realName
          imageUrl
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
  query GetCaseMembers($workspaceId: String!, $id: Int!, $limit: Int, $offset: Int, $filter: String) {
    case(workspaceId: $workspaceId, id: $id) {
      id
      channelUserCount
      channelUsers(limit: $limit, offset: $offset, filter: $filter) {
        items {
          id
          name
          realName
          imageUrl
        }
        totalCount
        hasMore
      }
    }
  }
`

export const CREATE_CASE = gql`
  mutation CreateCase($workspaceId: String!, $input: CreateCaseInput!) {
    createCase(workspaceId: $workspaceId, input: $input) {
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
      createdAt
      updatedAt
      fields {
        fieldId
        value
      }
    }
  }
`

export const UPDATE_CASE = gql`
  mutation UpdateCase($workspaceId: String!, $input: UpdateCaseInput!) {
    updateCase(workspaceId: $workspaceId, input: $input) {
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
      createdAt
      updatedAt
      fields {
        fieldId
        value
      }
    }
  }
`

export const DELETE_CASE = gql`
  mutation DeleteCase($workspaceId: String!, $id: Int!) {
    deleteCase(workspaceId: $workspaceId, id: $id)
  }
`

export const CLOSE_CASE = gql`
  mutation CloseCase($workspaceId: String!, $id: Int!) {
    closeCase(workspaceId: $workspaceId, id: $id) {
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
      createdAt
      updatedAt
      fields {
        fieldId
        value
      }
    }
  }
`

export const REOPEN_CASE = gql`
  mutation ReopenCase($workspaceId: String!, $id: Int!) {
    reopenCase(workspaceId: $workspaceId, id: $id) {
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
      createdAt
      updatedAt
      fields {
        fieldId
        value
      }
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
  mutation AssignCase($workspaceId: String!, $id: Int!, $userIDs: [String!]!) {
    assignCase(workspaceId: $workspaceId, id: $id, userIDs: $userIDs) {
      id
      assigneeIDs
      assignees {
        id
        name
        realName
        imageUrl
      }
      updatedAt
    }
  }
`

export const UNASSIGN_CASE = gql`
  mutation UnassignCase($workspaceId: String!, $id: Int!, $userIDs: [String!]!) {
    unassignCase(workspaceId: $workspaceId, id: $id, userIDs: $userIDs) {
      id
      assigneeIDs
      assignees {
        id
        name
        realName
        imageUrl
      }
      updatedAt
    }
  }
`
