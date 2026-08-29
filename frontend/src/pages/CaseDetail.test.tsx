import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { MockedProvider, type MockedResponse } from '@apollo/client/testing'
import { MemoryRouter, Route, Routes } from 'react-router'

import { I18nProvider } from '../i18n'
import { ARCHIVE_CASE, GET_CASE, UNARCHIVE_CASE } from '../graphql/case'
import { GET_CASE_STATUS_CONFIG } from '../graphql/caseStatus'
import { GET_CASE_LATEST_JOB_RUN } from '../graphql/caseAgent'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import { GET_MEMO_CONFIGURATION } from '../graphql/memo'
import CaseDetail from './CaseDetail'

const WORKSPACE = 'risk'
const CASE_ID = 7

vi.mock('../contexts/workspace-context', () => ({
  useWorkspace: () => ({
    currentWorkspace: { id: WORKSPACE, name: 'Risk' },
    workspaces: [{ id: WORKSPACE, name: 'Risk' }],
    isLoading: false,
    setCurrentWorkspace: vi.fn(),
    switchWorkspace: vi.fn(),
  }),
}))

// The detail page mounts several heavy children that fetch on their own; the
// archive controls live in the header and do not depend on any of them.
vi.mock('../components/memo/MemoTab', () => ({ default: () => <div /> }))
vi.mock('./CaseForm', () => ({ default: () => <div /> }))
vi.mock('../hooks/useAssigneeCandidates', () => ({
  useAssigneeCandidates: () => ({ users: [], loading: false }),
}))

function caseResult(status: 'OPEN' | 'CLOSED', archivedAt: string | null) {
  return {
    __typename: 'Case',
    id: CASE_ID,
    workspaceId: WORKSPACE,
    title: 'Suspicious login',
    description: 'body',
    status,
    archivedAt,
    isPrivate: false,
    isTest: false,
    accessDenied: false,
    reporterID: null,
    reporter: null,
    assigneeIDs: [],
    assignees: [],
    slackChannelID: null,
    slackThreadTS: null,
    isThreadBound: false,
    boardStatus: null,
    createdAt: '2026-05-01T00:00:00Z',
    updatedAt: '2026-05-01T00:00:00Z',
    fields: [],
    channelUserCount: 0,
    slackChannelURL: null,
    actions: [],
  }
}

function getCaseMock(status: 'OPEN' | 'CLOSED', archivedAt: string | null): MockedResponse {
  return {
    request: {
      query: GET_CASE,
      variables: { workspaceId: WORKSPACE, id: CASE_ID, actionsFilter: 'ACTIVE' },
    },
    // maxUsageCount lets the post-mutation refetch reuse the same shape when a
    // test does not care about the refreshed value.
    maxUsageCount: 10,
    result: { data: { case: caseResult(status, archivedAt) } },
  }
}

function supportingMocks(): MockedResponse[] {
  return [
    {
      request: { query: GET_FIELD_CONFIGURATION, variables: { workspaceId: WORKSPACE } },
      maxUsageCount: 10,
      result: {
        data: {
          fieldConfiguration: {
            __typename: 'FieldConfiguration',
            fields: [],
            labels: { __typename: 'FieldLabels', case: 'Case' },
            actionConfig: {
              __typename: 'ActionConfig',
              initial: 'BACKLOG',
              closed: ['COMPLETED'],
              statuses: [],
            },
          },
        },
      },
    },
    {
      request: { query: GET_CASE_STATUS_CONFIG, variables: { workspaceId: WORKSPACE } },
      maxUsageCount: 10,
      result: { data: { caseStatusConfig: null } },
    },
    {
      request: { query: GET_MEMO_CONFIGURATION, variables: { workspaceId: WORKSPACE } },
      maxUsageCount: 10,
      result: { data: { memoConfiguration: { __typename: 'MemoConfiguration', fields: [] } } },
    },
    {
      request: {
        query: GET_CASE_LATEST_JOB_RUN,
        variables: { workspaceId: WORKSPACE, caseId: CASE_ID },
      },
      maxUsageCount: 10,
      result: {
        data: {
          caseJobRunLogs: { __typename: 'JobRunLogConnection', items: [], totalCount: 0, hasMore: false },
        },
      },
    },
  ]
}

function renderDetail(mocks: MockedResponse[]) {
  return render(
    <MemoryRouter initialEntries={[`/ws/${WORKSPACE}/cases/${CASE_ID}`]}>
      <MockedProvider mocks={mocks} addTypename={false}>
        <I18nProvider defaultLang="en">
          <Routes>
            <Route path="/ws/:workspaceId/cases/:id" element={<CaseDetail />} />
            <Route path="/ws/:workspaceId/cases" element={<div data-testid="case-list-stub" />} />
          </Routes>
        </I18nProvider>
      </MockedProvider>
    </MemoryRouter>,
  )
}

async function openKebab() {
  fireEvent.click(await screen.findByTestId('case-menu-button'))
  return screen.findByTestId('case-menu-popover')
}

afterEach(() => {
  cleanup()
})

// Archiving only applies to a CLOSED case and restoring only to an archived
// one, so the menu never offers an item the server would refuse.
describe('CaseDetail archive controls', () => {
  it('offers Archive on a closed, non-archived case', async () => {
    renderDetail([getCaseMock('CLOSED', null), ...supportingMocks()])
    await screen.findByText('Suspicious login')
    await openKebab()

    expect(screen.getByTestId('case-archive-menu-item')).toBeInTheDocument()
    expect(screen.queryByTestId('case-unarchive-menu-item')).toBeNull()
    expect(screen.queryByTestId('case-archived-badge')).toBeNull()
  })

  it('offers neither item on an open case', async () => {
    renderDetail([getCaseMock('OPEN', null), ...supportingMocks()])
    await screen.findByText('Suspicious login')
    await openKebab()

    expect(screen.queryByTestId('case-archive-menu-item')).toBeNull()
    expect(screen.queryByTestId('case-unarchive-menu-item')).toBeNull()
    // Delete stays available regardless.
    expect(screen.getByTestId('case-delete-menu-item')).toBeInTheDocument()
  })

  it('shows the Archived badge and offers Restore on an archived case', async () => {
    renderDetail([getCaseMock('CLOSED', '2026-05-02T00:00:00Z'), ...supportingMocks()])
    await screen.findByText('Suspicious login')

    expect(screen.getByTestId('case-archived-badge')).toBeInTheDocument()
    await openKebab()
    expect(screen.getByTestId('case-unarchive-menu-item')).toBeInTheDocument()
    expect(screen.queryByTestId('case-archive-menu-item')).toBeNull()
  })

  it('calls archiveCase when Archive is chosen', async () => {
    let called = false
    const archiveMock: MockedResponse = {
      request: { query: ARCHIVE_CASE, variables: { workspaceId: WORKSPACE, id: CASE_ID } },
      result: () => {
        called = true
        return { data: { archiveCase: caseResult('CLOSED', '2026-05-02T00:00:00Z') } }
      },
    }
    renderDetail([getCaseMock('CLOSED', null), archiveMock, ...supportingMocks()])
    await screen.findByText('Suspicious login')
    await openKebab()

    fireEvent.click(screen.getByTestId('case-archive-menu-item'))
    await waitFor(() => {
      expect(called).toBe(true)
    })
  })

  it('calls unarchiveCase when Restore is chosen', async () => {
    let called = false
    const unarchiveMock: MockedResponse = {
      request: { query: UNARCHIVE_CASE, variables: { workspaceId: WORKSPACE, id: CASE_ID } },
      result: () => {
        called = true
        return { data: { unarchiveCase: caseResult('CLOSED', null) } }
      },
    }
    renderDetail([
      getCaseMock('CLOSED', '2026-05-02T00:00:00Z'),
      unarchiveMock,
      ...supportingMocks(),
    ])
    await screen.findByText('Suspicious login')
    await openKebab()

    fireEvent.click(screen.getByTestId('case-unarchive-menu-item'))
    await waitFor(() => {
      expect(called).toBe(true)
    })
  })
})
