import { gql } from '@apollo/client'

// Fragment for knowledge data
const KNOWLEDGE_FIELDS = gql`
  fragment KnowledgeFields on Knowledge {
    id
    riskID
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
  query GetKnowledge($id: String!) {
    knowledge(id: $id) {
      ...KnowledgeFields
      risk {
        id
        name
        description
      }
    }
  }
`

export const GET_KNOWLEDGES = gql`
  ${KNOWLEDGE_FIELDS}
  query GetKnowledges($limit: Int, $offset: Int) {
    knowledges(limit: $limit, offset: $offset) {
      items {
        ...KnowledgeFields
        risk {
          id
          name
        }
      }
      totalCount
      hasMore
    }
  }
`
