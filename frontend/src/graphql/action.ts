import { gql } from '@apollo/client'

const ACTION_FIELDS = `
  id
  caseID
  case {
    id
    title
    slackChannelID
    slackChannelURL
  }
  title
  description
  assigneeID
  assignee {
    id
    name
    realName
    imageUrl
  }
  slackMessageTS
  status
  dueDate
  archived
  archivedAt
  createdAt
  updatedAt
`

export const GET_ACTIONS = gql`
  query GetActions($workspaceId: String!) {
    actions(workspaceId: $workspaceId) {
      ${ACTION_FIELDS}
    }
  }
`

export const GET_ACTION = gql`
  query GetAction($workspaceId: String!, $id: Int!) {
    action(workspaceId: $workspaceId, id: $id) {
      ${ACTION_FIELDS}
    }
  }
`

export const GET_ACTION_MESSAGES = gql`
  query GetActionMessages($workspaceId: String!, $id: Int!, $limit: Int, $cursor: String) {
    action(workspaceId: $workspaceId, id: $id) {
      id
      messages(limit: $limit, cursor: $cursor) {
        items {
          id
          channelID
          threadTS
          teamID
          userID
          userName
          text
          createdAt
          files {
            id
            name
            mimetype
            filetype
            size
            urlPrivate
            permalink
            thumbURL
          }
        }
        nextCursor
      }
    }
  }
`

export const GET_ACTION_EVENTS = gql`
  query GetActionEvents($workspaceId: String!, $id: Int!, $limit: Int, $cursor: String) {
    action(workspaceId: $workspaceId, id: $id) {
      id
      events(limit: $limit, cursor: $cursor) {
        items {
          id
          actionID
          kind
          actorID
          actor {
            id
            name
            realName
            imageUrl
          }
          oldValue
          newValue
          createdAt
        }
        nextCursor
      }
    }
  }
`

export const CREATE_ACTION = gql`
  mutation CreateAction($workspaceId: String!, $input: CreateActionInput!) {
    createAction(workspaceId: $workspaceId, input: $input) {
      ${ACTION_FIELDS}
    }
  }
`

export const UPDATE_ACTION = gql`
  mutation UpdateAction($workspaceId: String!, $input: UpdateActionInput!) {
    updateAction(workspaceId: $workspaceId, input: $input) {
      ${ACTION_FIELDS}
    }
  }
`

export const ARCHIVE_ACTION = gql`
  mutation ArchiveAction($workspaceId: String!, $id: Int!) {
    archiveAction(workspaceId: $workspaceId, id: $id) {
      ${ACTION_FIELDS}
    }
  }
`

export const UNARCHIVE_ACTION = gql`
  mutation UnarchiveAction($workspaceId: String!, $id: Int!) {
    unarchiveAction(workspaceId: $workspaceId, id: $id) {
      ${ACTION_FIELDS}
    }
  }
`

export const POST_ACTION_SLACK_MESSAGE = gql`
  mutation PostActionSlackMessage($workspaceId: String!, $id: Int!) {
    postActionSlackMessage(workspaceId: $workspaceId, id: $id) {
      ${ACTION_FIELDS}
    }
  }
`

export const GET_OPEN_CASE_ACTIONS = gql`
  query GetOpenCaseActions($workspaceId: String!) {
    openCaseActions(workspaceId: $workspaceId) {
      ${ACTION_FIELDS}
    }
  }
`
