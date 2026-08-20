import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { MockedProvider, type MockedResponse } from '@apollo/client/testing'
import { I18nProvider } from '../i18n'
import { CREATE_ACTION_COMMENT, GET_ACTION_COMMENTS } from '../graphql/action'
import ActionCommentComposer from './ActionCommentComposer'

const WS = 'risk'
const ACTION_ID = 7
const PAGE_SIZE = 20

function createMock(body: string, fail = false): MockedResponse {
  const base = {
    request: {
      query: CREATE_ACTION_COMMENT,
      variables: { workspaceId: WS, input: { actionId: ACTION_ID, body } },
    },
  }
  if (fail) {
    return { ...base, error: new Error('boom') }
  }
  return {
    ...base,
    result: {
      data: {
        createActionComment: {
          __typename: 'ActionComment',
          id: 'comment-1',
          actionID: ACTION_ID,
          authorID: 'U1',
          body: body.trim(),
          createdAt: '2026-08-20T01:00:00Z',
          updatedAt: '2026-08-20T01:00:00Z',
          edited: false,
          author: null,
        },
      },
    },
  }
}

function commentsMock(): MockedResponse {
  return {
    request: {
      query: GET_ACTION_COMMENTS,
      variables: { workspaceId: WS, id: ACTION_ID, limit: PAGE_SIZE, cursor: null },
    },
    result: {
      data: {
        action: {
          __typename: 'Action',
          id: ACTION_ID,
          workspaceId: WS,
          comments: { __typename: 'ActionCommentConnection', items: [], nextCursor: '' },
        },
      },
    },
  }
}

function renderComposer(mocks: MockedResponse[]) {
  return render(
    <MockedProvider mocks={mocks} addTypename={false}>
      <I18nProvider>
        <ActionCommentComposer workspaceId={WS} actionId={ACTION_ID} pageSize={PAGE_SIZE} />
      </I18nProvider>
    </MockedProvider>,
  )
}

function textarea(): HTMLTextAreaElement {
  return screen.getByTestId('action-comment-body-textarea') as HTMLTextAreaElement
}

describe('ActionCommentComposer', () => {
  afterEach(cleanup)

  it('disables the submit button until the body is non-blank', () => {
    renderComposer([])
    const submit = screen.getByTestId('action-comment-submit')
    expect(submit).toBeDisabled()

    fireEvent.change(textarea(), { target: { value: '   ' } })
    expect(submit).toBeDisabled()

    fireEvent.change(textarea(), { target: { value: 'a real comment' } })
    expect(submit).toBeEnabled()
  })

  it('posts the comment and clears the draft on success', async () => {
    renderComposer([createMock('looks fine'), commentsMock()])

    fireEvent.change(textarea(), { target: { value: 'looks fine' } })
    fireEvent.click(screen.getByTestId('action-comment-submit'))

    await waitFor(() => expect(textarea().value).toBe(''))
    expect(screen.queryByTestId('action-comment-post-error')).toBeNull()
  })

  it('keeps the draft and shows an error when posting fails', async () => {
    renderComposer([createMock('will fail', true)])

    fireEvent.change(textarea(), { target: { value: 'will fail' } })
    fireEvent.click(screen.getByTestId('action-comment-submit'))

    const error = await screen.findByTestId('action-comment-post-error')
    expect(error).toBeInTheDocument()
    expect(textarea().value).toBe('will fail')
  })

  it('posts on Cmd+Enter but not on bare Enter', async () => {
    renderComposer([createMock('via shortcut'), commentsMock()])

    fireEvent.change(textarea(), { target: { value: 'via shortcut' } })

    // Bare Enter must insert a newline, not submit.
    fireEvent.keyDown(textarea(), { key: 'Enter' })
    expect(textarea().value).toBe('via shortcut')

    fireEvent.keyDown(textarea(), { key: 'Enter', metaKey: true })
    await waitFor(() => expect(textarea().value).toBe(''))
  })

  it('does not post on Cmd+Enter while the IME is composing', async () => {
    renderComposer([createMock('composing', true)])

    fireEvent.change(textarea(), { target: { value: 'composing' } })
    fireEvent.keyDown(textarea(), { key: 'Enter', metaKey: true, isComposing: true })

    // No mutation ran, so neither the draft nor the error state changed.
    await waitFor(() => expect(textarea().value).toBe('composing'))
    expect(screen.queryByTestId('action-comment-post-error')).toBeNull()
  })
})
