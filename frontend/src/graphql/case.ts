import { gql } from '@apollo/client'

export const GET_CASES = gql`
  query GetCases($workspaceId: String!) {
    cases(workspaceId: $workspaceId) {
      id
      title
      description
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

export const GET_CASE = gql`
  query GetCase($workspaceId: String!, $id: Int!) {
    case(workspaceId: $workspaceId, id: $id) {
      id
      title
      description
      assigneeIDs
      assignees {
        id
        name
        realName
        imageUrl
      }
      slackChannelID
      slackChannelName
      slackChannelURL
      createdAt
      updatedAt
      fields {
        fieldId
        value
      }
      actions {
        id
        title
        status
        createdAt
      }
      knowledges {
        id
        title
        summary
        sourcedAt
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
