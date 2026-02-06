import { gql } from '@apollo/client'

export const GET_ACTIONS = gql`
  query GetActions {
    actions {
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
      createdAt
      updatedAt
    }
  }
`

export const GET_ACTION = gql`
  query GetAction($id: Int!) {
    action(id: $id) {
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
      createdAt
      updatedAt
    }
  }
`

export const CREATE_ACTION = gql`
  mutation CreateAction($input: CreateActionInput!) {
    createAction(input: $input) {
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
      createdAt
      updatedAt
    }
  }
`

export const UPDATE_ACTION = gql`
  mutation UpdateAction($input: UpdateActionInput!) {
    updateAction(input: $input) {
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
      createdAt
      updatedAt
    }
  }
`

export const DELETE_ACTION = gql`
  mutation DeleteAction($id: Int!) {
    deleteAction(id: $id)
  }
`
