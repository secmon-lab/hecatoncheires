import { gql } from '@apollo/client'

export const GET_SLACK_USERS = gql`
  query GetSlackUsers {
    slackUsers {
      id
      name
      realName
      imageUrl
    }
  }
`

// Slack user IDs ranked by how often they are assigned in the workspace, most
// assigned first. Ordering-only data for the assignee picker: it is served from
// a server-side cache and comes back empty until the first refresh lands, so
// callers must treat it as a hint and keep their own default order as the
// fallback. See useAssigneeCandidates.
export const GET_FREQUENT_ASSIGNEE_IDS = gql`
  query GetFrequentAssigneeIDs($workspaceId: String!) {
    frequentAssigneeIDs(workspaceId: $workspaceId)
  }
`
