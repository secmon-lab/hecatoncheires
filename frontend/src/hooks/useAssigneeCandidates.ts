import { useQuery } from '@apollo/client'
import { useMemo } from 'react'
import { GET_FREQUENT_ASSIGNEE_IDS, GET_SLACK_USERS } from '../graphql/slackUsers'
import { orderAssigneeCandidates } from '../utils/assignees'

export interface AssigneeCandidate {
  id: string
  name: string
  realName: string
  imageUrl?: string | null
}

// useAssigneeCandidates is the single supply point for the user list every
// assignee picker renders. It returns the workspace's Slack users ordered so the
// people who most often take work there come first, then everyone else by
// display name — so no page has to sort for itself.
//
// The ranking is an ordering hint only. While it loads, when it fails, and when
// the workspace is not resolved yet, the list simply falls back to display-name
// order; `loading` therefore reflects the user list alone, so a slow or broken
// ranking never keeps a picker closed.
export function useAssigneeCandidates(workspaceId: string | null | undefined) {
  const usersQuery = useQuery<{ slackUsers: AssigneeCandidate[] }>(GET_SLACK_USERS)
  const rankQuery = useQuery<{ frequentAssigneeIDs: string[] }>(GET_FREQUENT_ASSIGNEE_IDS, {
    variables: { workspaceId: workspaceId ?? '' },
    skip: !workspaceId,
  })

  const users = useMemo(
    () =>
      orderAssigneeCandidates(
        usersQuery.data?.slackUsers ?? [],
        rankQuery.data?.frequentAssigneeIDs ?? [],
      ),
    [usersQuery.data, rankQuery.data],
  )

  return { users, loading: usersQuery.loading }
}
