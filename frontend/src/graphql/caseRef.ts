import { gql } from '@apollo/client'

export const REFERENCEABLE_CASES = gql`
  query ReferenceableCases($workspaceId: String!, $query: String, $limit: Int) {
    referenceableCases(workspaceId: $workspaceId, query: $query, limit: $limit) {
      id
      title
      status
      workspaceId
    }
  }
`

export const CASE_REFS_BY_IDS = gql`
  query CaseRefsByIds($workspaceId: String!, $ids: [Int!]!) {
    caseRefsByIds(workspaceId: $workspaceId, ids: $ids) {
      id
      title
      status
      workspaceId
    }
  }
`
