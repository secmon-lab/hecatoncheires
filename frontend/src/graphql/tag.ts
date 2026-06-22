import { gql } from '@apollo/client'

export const GET_TAGS = gql`
  query GetTags($workspaceId: String!) {
    tags(workspaceId: $workspaceId) {
      id
      name
      createdAt
      updatedAt
    }
  }
`

export const CREATE_TAG = gql`
  mutation CreateTag($workspaceId: String!, $name: String!) {
    createTag(workspaceId: $workspaceId, name: $name) {
      id
      name
      createdAt
      updatedAt
    }
  }
`

export const UPDATE_TAG = gql`
  mutation UpdateTag($workspaceId: String!, $id: ID!, $name: String!) {
    updateTag(workspaceId: $workspaceId, id: $id, name: $name) {
      id
      name
      createdAt
      updatedAt
    }
  }
`

export const DELETE_TAG = gql`
  mutation DeleteTag($workspaceId: String!, $id: ID!) {
    deleteTag(workspaceId: $workspaceId, id: $id)
  }
`
