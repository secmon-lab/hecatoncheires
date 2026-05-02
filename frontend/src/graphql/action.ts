import { gql } from '@apollo/client'

const ACTION_FIELDS = `
  id
  caseID
  case {
    id
    title
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

export const DELETE_ACTION = gql`
  mutation DeleteAction($workspaceId: String!, $id: Int!) {
    deleteAction(workspaceId: $workspaceId, id: $id)
  }
`

export const GET_OPEN_CASE_ACTIONS = gql`
  query GetOpenCaseActions($workspaceId: String!) {
    openCaseActions(workspaceId: $workspaceId) {
      ${ACTION_FIELDS}
    }
  }
`
