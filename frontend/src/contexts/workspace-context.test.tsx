import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { MockedProvider, type MockedResponse } from '@apollo/client/testing'
import { MemoryRouter } from 'react-router-dom'
import type { ReactNode } from 'react'
import { GraphQLError } from 'graphql'
import { GET_FAVORITE_WORKSPACE_IDS, SET_FAVORITE_WORKSPACES } from '../graphql/dashboard'
import { WorkspaceProvider, useWorkspace } from './workspace-context'

function stubWorkspacesFetch(workspaces: Array<{ id: string; name: string }> = []) {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ workspaces }),
    }),
  )
}

function favoritesMock(ids: string[]): MockedResponse {
  return {
    request: { query: GET_FAVORITE_WORKSPACE_IDS },
    result: { data: { favoriteWorkspaceIds: ids } },
  }
}

function wrapperFor(mocks: MockedResponse[]) {
  return ({ children }: { children: ReactNode }) => (
    <MemoryRouter>
      <MockedProvider mocks={mocks} addTypename={false}>
        <WorkspaceProvider>{children}</WorkspaceProvider>
      </MockedProvider>
    </MemoryRouter>
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('useWorkspace favorites', () => {
  it('loads favoriteWorkspaceIds from the GraphQL query', async () => {
    stubWorkspacesFetch()
    const { result } = renderHook(() => useWorkspace(), {
      wrapper: wrapperFor([favoritesMock(['risk', 'support'])]),
    })

    await waitFor(() => expect(result.current.favoriteWorkspaceIds).toEqual(['risk', 'support']))
  })

  it('defaults to an empty list when the query errors, instead of throwing', async () => {
    stubWorkspacesFetch()
    const errorMock: MockedResponse = {
      request: { query: GET_FAVORITE_WORKSPACE_IDS },
      result: { errors: [new GraphQLError('boom')] },
    }
    const { result } = renderHook(() => useWorkspace(), {
      wrapper: wrapperFor([errorMock]),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.favoriteWorkspaceIds).toEqual([])
  })

  it('toggleFavorite adds an id that is not yet favorited and sends the full next list', async () => {
    stubWorkspacesFetch()
    const mutationMock: MockedResponse = {
      request: {
        query: SET_FAVORITE_WORKSPACES,
        variables: { workspaceIds: ['risk', 'support'] },
      },
      result: { data: { setFavoriteWorkspaces: ['risk', 'support'] } },
    }
    const { result } = renderHook(() => useWorkspace(), {
      wrapper: wrapperFor([favoritesMock(['risk']), mutationMock]),
    })

    await waitFor(() => expect(result.current.favoriteWorkspaceIds).toEqual(['risk']))

    act(() => {
      result.current.toggleFavorite('support')
    })

    // Optimistic response applies synchronously.
    await waitFor(() => expect(result.current.favoriteWorkspaceIds).toEqual(['risk', 'support']))
  })

  it('toggleFavorite removes an id that is already favorited', async () => {
    stubWorkspacesFetch()
    const mutationMock: MockedResponse = {
      request: {
        query: SET_FAVORITE_WORKSPACES,
        variables: { workspaceIds: ['risk'] },
      },
      result: { data: { setFavoriteWorkspaces: ['risk'] } },
    }
    const { result } = renderHook(() => useWorkspace(), {
      wrapper: wrapperFor([favoritesMock(['risk', 'support']), mutationMock]),
    })

    await waitFor(() => expect(result.current.favoriteWorkspaceIds).toEqual(['risk', 'support']))

    act(() => {
      result.current.toggleFavorite('support')
    })

    await waitFor(() => expect(result.current.favoriteWorkspaceIds).toEqual(['risk']))
  })
})
