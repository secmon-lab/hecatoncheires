import { gql } from '@apollo/client'

export const GET_CASES = gql`
  query GetCases {
    cases {
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
  query GetCase($id: Int!) {
    case(id: $id) {
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
  mutation CreateCase($input: CreateCaseInput!) {
    createCase(input: $input) {
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
  mutation UpdateCase($input: UpdateCaseInput!) {
    updateCase(input: $input) {
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
  mutation DeleteCase($id: Int!) {
    deleteCase(id: $id)
  }
`
