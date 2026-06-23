import { gql } from '@apollo/client'

const KNOWLEDGE_FIELDS = gql`
  fragment KnowledgeFields on Knowledge {
    id
    title
    claim
    tags {
      id
      name
    }
    createdAt
    updatedAt
  }
`

export const GET_KNOWLEDGES = gql`
  ${KNOWLEDGE_FIELDS}
  query GetKnowledges($workspaceId: String!, $tagIds: [ID!]) {
    knowledges(workspaceId: $workspaceId, tagIds: $tagIds) {
      ...KnowledgeFields
    }
  }
`

export const GET_KNOWLEDGE = gql`
  ${KNOWLEDGE_FIELDS}
  query GetKnowledge($workspaceId: String!, $id: ID!) {
    knowledge(workspaceId: $workspaceId, id: $id) {
      ...KnowledgeFields
    }
  }
`

export const SEARCH_KNOWLEDGE = gql`
  ${KNOWLEDGE_FIELDS}
  query SearchKnowledge($workspaceId: String!, $query: String!, $tagIds: [ID!], $limit: Int) {
    searchKnowledge(workspaceId: $workspaceId, query: $query, tagIds: $tagIds, limit: $limit) {
      ...KnowledgeFields
    }
  }
`

export const CREATE_KNOWLEDGE = gql`
  ${KNOWLEDGE_FIELDS}
  mutation CreateKnowledge($workspaceId: String!, $input: CreateKnowledgeInput!) {
    createKnowledge(workspaceId: $workspaceId, input: $input) {
      ...KnowledgeFields
    }
  }
`

export const UPDATE_KNOWLEDGE = gql`
  ${KNOWLEDGE_FIELDS}
  mutation UpdateKnowledge($workspaceId: String!, $input: UpdateKnowledgeInput!) {
    updateKnowledge(workspaceId: $workspaceId, input: $input) {
      ...KnowledgeFields
    }
  }
`

export const DELETE_KNOWLEDGE = gql`
  mutation DeleteKnowledge($workspaceId: String!, $id: ID!) {
    deleteKnowledge(workspaceId: $workspaceId, id: $id)
  }
`
