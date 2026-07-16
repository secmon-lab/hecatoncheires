import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { MockedProvider, type MockedResponse } from '@apollo/client/testing'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { I18nProvider } from '../i18n'
import { GET_MEMO, GET_MEMO_CONFIGURATION, ARCHIVE_MEMO } from '../graphql/memo'
import MemoDetail from './MemoDetail'

const WS = 'risk'
const CASE_ID = 1
const MEMO_ID = 'memo-1'
const MEMO_PATH = `/ws/${WS}/cases/${CASE_ID}/memos/${MEMO_ID}`

function memoData(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: MEMO_ID,
    caseID: CASE_ID,
    title: 'Phase check 2026-07-13',
    fields: [
      { fieldId: 'severity', value: 'high' },
      { fieldId: 'note', value: 'plain text note' },
    ],
    archivedAt: null,
    createdAt: '2026-07-13T01:15:41Z',
    updatedAt: '2026-07-14T02:00:00Z',
    ...overrides,
  }
}

function memoMock(overrides: Partial<Record<string, unknown>> = {}): MockedResponse {
  return {
    request: {
      query: GET_MEMO,
      variables: { workspaceId: WS, caseID: CASE_ID, id: MEMO_ID },
    },
    result: { data: { memo: memoData(overrides) } },
  }
}

function memoErrorMock(): MockedResponse {
  return {
    request: {
      query: GET_MEMO,
      variables: { workspaceId: WS, caseID: CASE_ID, id: MEMO_ID },
    },
    error: new Error('memo not found'),
  }
}

function configMock(): MockedResponse {
  return {
    request: {
      query: GET_MEMO_CONFIGURATION,
      variables: { workspaceId: WS },
    },
    result: {
      data: {
        memoConfiguration: {
          description: 'Memo config',
          fields: [
            {
              id: 'severity',
              name: 'Severity',
              type: 'SELECT',
              required: false,
              description: null,
              options: [
                { id: 'high', name: 'High', description: null, metadata: null },
                { id: 'low', name: 'Low', description: null, metadata: null },
              ],
            },
            {
              id: 'note',
              name: 'Note',
              type: 'TEXT',
              required: false,
              description: null,
              options: null,
            },
          ],
        },
      },
    },
  }
}

function archiveMock(): MockedResponse {
  return {
    request: {
      query: ARCHIVE_MEMO,
      variables: { workspaceId: WS, caseID: CASE_ID, id: MEMO_ID },
    },
    result: {
      data: { archiveMemo: memoData({ archivedAt: '2026-07-15T00:00:00Z' }) },
    },
  }
}

function archiveErrorMock(): MockedResponse {
  return {
    request: {
      query: ARCHIVE_MEMO,
      variables: { workspaceId: WS, caseID: CASE_ID, id: MEMO_ID },
    },
    error: new Error('network down'),
  }
}

function renderAt(path: string, mocks: MockedResponse[]) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <MockedProvider mocks={mocks} addTypename={false}>
        <I18nProvider defaultLang="en">
          <Routes>
            <Route path="/ws/:workspaceId/cases/:id/memos/:memoId" element={<MemoDetail />} />
          </Routes>
        </I18nProvider>
      </MockedProvider>
    </MemoryRouter>,
  )
}

afterEach(cleanup)

describe('MemoDetail', () => {
  it('renders title, full id, field values, and active-memo actions', async () => {
    renderAt(MEMO_PATH, [memoMock(), configMock()])

    await screen.findByTestId('memo-detail-page')

    expect(screen.getByTestId('memo-detail-title')).toHaveTextContent('Phase check 2026-07-13')
    // Full memo id (not the 8-char truncation the old modal used).
    expect(screen.getByText(MEMO_ID)).toBeInTheDocument()
    // Field labels and rendered values (SELECT resolves the option name).
    expect(screen.getByText('Severity')).toBeInTheDocument()
    expect(screen.getByText('High')).toBeInTheDocument()
    expect(screen.getByText('Note')).toBeInTheDocument()
    expect(screen.getByText('plain text note')).toBeInTheDocument()
    // Active memo: edit + archive, no unarchive, no archived badge/notice.
    expect(screen.getByTestId('memo-detail-edit-button')).toBeInTheDocument()
    expect(screen.getByTestId('memo-detail-archive-button')).toBeInTheDocument()
    expect(screen.queryByTestId('memo-detail-unarchive-button')).not.toBeInTheDocument()
    expect(screen.queryByText('Archived')).not.toBeInTheDocument()
    // Back link points at the parent case.
    expect(screen.getByTestId('memo-detail-back-link')).toHaveAttribute(
      'href',
      `/ws/${WS}/cases/${CASE_ID}`,
    )
  })

  it('renders archived state with badge, notice, and unarchive action', async () => {
    renderAt(MEMO_PATH, [memoMock({ archivedAt: '2026-07-15T00:00:00Z' }), configMock()])

    await screen.findByTestId('memo-detail-page')

    expect(screen.getByText('Archived')).toBeInTheDocument()
    expect(
      screen.getByText('This memo is archived and cannot be edited. Restore it to make it editable again.'),
    ).toBeInTheDocument()
    expect(screen.getByTestId('memo-detail-unarchive-button')).toBeInTheDocument()
    expect(screen.queryByTestId('memo-detail-edit-button')).not.toBeInTheDocument()
    expect(screen.queryByTestId('memo-detail-archive-button')).not.toBeInTheDocument()
  })

  it('shows the load-error state with a retry button on a GraphQL error', async () => {
    renderAt(MEMO_PATH, [memoErrorMock(), configMock()])

    await screen.findByTestId('memo-detail-error')

    expect(screen.getByText('Failed to load memos')).toBeInTheDocument()
    expect(screen.getByText('Retry')).toBeInTheDocument()
    expect(screen.queryByTestId('memo-detail-page')).not.toBeInTheDocument()
    expect(screen.queryByTestId('memo-detail-title')).not.toBeInTheDocument()
  })

  it('archives through the confirm dialog and closes it on success', async () => {
    renderAt(MEMO_PATH, [memoMock(), configMock(), archiveMock()])

    await screen.findByTestId('memo-detail-page')

    fireEvent.click(screen.getByTestId('memo-detail-archive-button'))
    const confirm = await screen.findByTestId('memo-archive-confirm-button')
    fireEvent.click(confirm)

    await waitFor(() => {
      expect(screen.queryByTestId('memo-archive-confirm-button')).not.toBeInTheDocument()
    })
    // No mutation-error notice on success.
    expect(screen.queryByText('Operation failed. Please try again.')).not.toBeInTheDocument()
  })

  it('surfaces a mutation failure as an inline error message', async () => {
    renderAt(MEMO_PATH, [memoMock(), configMock(), archiveErrorMock()])

    await screen.findByTestId('memo-detail-page')

    fireEvent.click(screen.getByTestId('memo-detail-archive-button'))
    fireEvent.click(await screen.findByTestId('memo-archive-confirm-button'))

    expect(await screen.findByText('Operation failed. Please try again.')).toBeInTheDocument()
    // The dialog is closed; the page itself stays rendered.
    expect(screen.queryByTestId('memo-archive-confirm-button')).not.toBeInTheDocument()
    expect(screen.getByTestId('memo-detail-page')).toBeInTheDocument()
  })

  it('renders nothing for a non-numeric case id', () => {
    const { container } = renderAt(`/ws/${WS}/cases/abc/memos/${MEMO_ID}`, [])

    expect(screen.queryByTestId('memo-detail-page')).not.toBeInTheDocument()
    expect(screen.queryByTestId('memo-detail-error')).not.toBeInTheDocument()
    expect(container.textContent).toBe('')
  })
})
