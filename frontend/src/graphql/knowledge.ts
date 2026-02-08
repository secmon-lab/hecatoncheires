import { gql } from '@apollo/client'

// Fragment for knowledge data
const KNOWLEDGE_FIELDS = gql`
  fragment KnowledgeFields on Knowledge {
    id
    caseID
    sourceID
    sourceURL
    title
    summary
    sourcedAt
    createdAt
    updatedAt
  }
`

export const GET_KNOWLEDGE = gql`
  ${KNOWLEDGE_FIELDS}
  query GetKnowledge($workspaceId: String!, $id: String!) {
    knowledge(workspaceId: $workspaceId, id: $id) {
      ...KnowledgeFields
      case {
        id
        title
        description
      }
    }
  }
`

export const GET_KNOWLEDGES = gql`
  ${KNOWLEDGE_FIELDS}
  query GetKnowledges($workspaceId: String!, $limit: Int, $offset: Int) {
    knowledges(workspaceId: $workspaceId, limit: $limit, offset: $offset) {
      items {
        ...KnowledgeFields
        case {
          id
          title
        }
      }
      totalCount
      hasMore
    }
  }
`
