import { describe, expect, it } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { MockedProvider, type MockedResponse } from '@apollo/client/testing'
import type { ReactNode } from 'react'
import { GET_CASE_STATUS_CONFIG } from '../graphql/caseStatus'
import { useCaseStatuses } from './useCaseStatuses'

const threadModeMock: MockedResponse = {
  request: { query: GET_CASE_STATUS_CONFIG, variables: { workspaceId: 'support' } },
  result: {
    data: {
      caseStatusConfig: {
        __typename: 'ActionConfig',
        initial: 'TRIAGE',
        closed: ['DONE'],
        statuses: [
          { __typename: 'ActionStatusDefinition', id: 'TRIAGE', name: 'Triage', description: null, color: 'active', emoji: null },
          { __typename: 'ActionStatusDefinition', id: 'DONE', name: 'Done', description: null, color: 'success', emoji: null },
        ],
      },
    },
  },
}

const channelModeMock: MockedResponse = {
  request: { query: GET_CASE_STATUS_CONFIG, variables: { workspaceId: 'risk' } },
  result: { data: { caseStatusConfig: null } },
}

function wrapperFor(mocks: MockedResponse[]) {
  return ({ children }: { children: ReactNode }) => (
    <MockedProvider mocks={mocks} addTypename={false}>
      {children}
    </MockedProvider>
  )
}

describe('useCaseStatuses', () => {
  it('exposes thread-mode config when the workspace has a case status set', async () => {
    const { result } = renderHook(() => useCaseStatuses('support'), {
      wrapper: wrapperFor([threadModeMock]),
    })

    await waitFor(() => expect(result.current.isThreadMode).toBe(true))
    expect(result.current.initialId).toBe('TRIAGE')
    expect(result.current.statuses.map((s) => s.id)).toEqual(['TRIAGE', 'DONE'])
    expect(result.current.isClosed('DONE')).toBe(true)
    expect(result.current.isClosed('TRIAGE')).toBe(false)
    expect(result.current.label('TRIAGE')).toBe('Triage')
  })

  it('reports channel mode (no case status set) when config is null', async () => {
    const { result } = renderHook(() => useCaseStatuses('risk'), {
      wrapper: wrapperFor([channelModeMock]),
    })

    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current.isThreadMode).toBe(false)
    expect(result.current.statuses).toEqual([])
  })
})
