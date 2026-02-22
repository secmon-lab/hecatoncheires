import { gql } from '@apollo/client'

export const GET_ACTIONS = gql`
  query GetActions($workspaceId: String!) {
    actions(workspaceId: $workspaceId) {
      id
      caseID
      case {
        id
        title
      }
      title
      description
      assigneeIDs
      assignees {
        id
        name
        realName
        imageUrl
      }
      slackMessageTS
      status
      dueDate
      createdAt
      updatedAt
    }
  }
`

export const GET_ACTION = gql`
  query GetAction($workspaceId: String!, $id: Int!) {
    action(workspaceId: $workspaceId, id: $id) {
      id
      caseID
      case {
        id
        title
      }
      title
      description
      assigneeIDs
      assignees {
        id
        name
        realName
        imageUrl
      }
      slackMessageTS
      status
      dueDate
      createdAt
      updatedAt
    }
  }
`

export const CREATE_ACTION = gql`
  mutation CreateAction($workspaceId: String!, $input: CreateActionInput!) {
    createAction(workspaceId: $workspaceId, input: $input) {
      id
      caseID
      title
      description
      assigneeIDs
      assignees {
        id
        name
        realName
        imageUrl
      }
      slackMessageTS
      status
      dueDate
      createdAt
      updatedAt
    }
  }
`

export const UPDATE_ACTION = gql`
  mutation UpdateAction($workspaceId: String!, $input: UpdateActionInput!) {
    updateAction(workspaceId: $workspaceId, input: $input) {
      id
      caseID
      title
      description
      assigneeIDs
      assignees {
        id
        name
        realName
        imageUrl
      }
      slackMessageTS
      status
      dueDate
      createdAt
      updatedAt
    }
  }
`

export const DELETE_ACTION = gql`
  mutation DeleteAction($workspaceId: String!, $id: Int!) {
    deleteAction(workspaceId: $workspaceId, id: $id)
  }
`

export const GET_OPEN_CASE_ACTIONS = gql`
  query GetOpenCaseActions($workspaceId: String!) {
    openCaseActions(workspaceId: $workspaceId) {
      id
      caseID
      case {
        id
        title
      }
      title
      description
      assigneeIDs
      assignees {
        id
        name
        realName
        imageUrl
      }
      slackMessageTS
      status
      dueDate
      createdAt
      updatedAt
    }
  }
`
