import { gql } from '@apollo/client'

export const GET_ASSIST_LOGS = gql`
  query GetAssistLogs($workspaceId: String!, $caseId: Int!, $limit: Int, $offset: Int) {
    assistLogs(workspaceId: $workspaceId, caseId: $caseId, limit: $limit, offset: $offset) {
      items {
        id
        caseId
        summary
        actions
        reasoning
        nextSteps
        createdAt
      }
      totalCount
      hasMore
    }
  }
`
