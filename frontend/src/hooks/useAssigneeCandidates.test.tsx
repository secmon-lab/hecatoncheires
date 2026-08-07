import { describe, expect, it } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { MockedProvider, type MockedResponse } from '@apollo/client/testing'
import type { ReactNode } from 'react'
import { GET_FREQUENT_ASSIGNEE_IDS, GET_SLACK_USERS } from '../graphql/slackUsers'
import { useAssigneeCandidates } from './useAssigneeCandidates'

const slackUsersMock: MockedResponse = {
  request: { query: GET_SLACK_USERS },
  result: {
    data: {
      slackUsers: [
        { id: 'U1', name: 'carol', realName: 'Carol', imageUrl: null },
        { id: 'U2', name: 'alice', realName: 'Alice', imageUrl: null },
        { id: 'U3', name: 'bob', realName: 'Bob', imageUrl: null },
      ],
    },
  },
}

const rankingMock: MockedResponse = {
  request: { query: GET_FREQUENT_ASSIGNEE_IDS, variables: { workspaceId: 'support' } },
  result: { data: { frequentAssigneeIDs: ['U3', 'U1'] } },
}

const rankingErrorMock: MockedResponse = {
  request: { query: GET_FREQUENT_ASSIGNEE_IDS, variables: { workspaceId: 'support' } },
  error: new Error('ranking unavailable'),
}

function wrapperFor(mocks: MockedResponse[]) {
  return ({ children }: { children: ReactNode }) => (
    <MockedProvider mocks={mocks} addTypename={false}>
      {children}
    </MockedProvider>
  )
}

describe('useAssigneeCandidates', () => {
  it('puts the frequently assigned users first, then the rest by display name', async () => {
    const { result } = renderHook(() => useAssigneeCandidates('support'), {
      wrapper: wrapperFor([slackUsersMock, rankingMock]),
    })

    await waitFor(() => expect(result.current.users.map((u) => u.id)).toEqual(['U3', 'U1', 'U2']))
    expect(result.current.loading).toBe(false)
  })

  it('falls back to display-name order when no workspace is resolved yet', async () => {
    // The ranking query is workspace-scoped, so it must not be issued at all —
    // only slackUsers is mocked, and an unexpected request would error the hook.
    const { result } = renderHook(() => useAssigneeCandidates(undefined), {
      wrapper: wrapperFor([slackUsersMock]),
    })

    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current.users.map((u) => u.id)).toEqual(['U2', 'U3', 'U1'])
  })

  it('falls back to display-name order when the ranking query fails', async () => {
    const { result } = renderHook(() => useAssigneeCandidates('support'), {
      wrapper: wrapperFor([slackUsersMock, rankingErrorMock]),
    })

    await waitFor(() => expect(result.current.users).toHaveLength(3))
    expect(result.current.users.map((u) => u.id)).toEqual(['U2', 'U3', 'U1'])
    expect(result.current.loading).toBe(false)
  })
})
