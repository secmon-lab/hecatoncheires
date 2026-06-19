import { gql } from '@apollo/client'

const KNOWLEDGE_FIELDS = gql`
  fragment KnowledgeFields on Knowledge {
    id
    title
    claim
    tags
    createdAt
    updatedAt
  }
`

export const GET_KNOWLEDGES = gql`
  ${KNOWLEDGE_FIELDS}
  query GetKnowledges($workspaceId: String!, $tags: [String!]) {
    knowledges(workspaceId: $workspaceId, tags: $tags) {
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

export const GET_KNOWLEDGE_TAGS = gql`
  query GetKnowledgeTags($workspaceId: String!) {
    knowledgeTags(workspaceId: $workspaceId)
  }
`

export const SEARCH_KNOWLEDGE = gql`
  ${KNOWLEDGE_FIELDS}
  query SearchKnowledge($workspaceId: String!, $query: String!, $tags: [String!], $limit: Int) {
    searchKnowledge(workspaceId: $workspaceId, query: $query, tags: $tags, limit: $limit) {
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
