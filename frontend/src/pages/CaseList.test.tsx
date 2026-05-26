import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { MockedProvider, type MockedResponse } from '@apollo/client/testing'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { I18nProvider } from '../i18n'
import { GET_CASES } from '../graphql/case'
import { GET_DRAFTS } from '../graphql/drafts'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import CaseList from './CaseList'

vi.mock('../contexts/workspace-context', () => ({
  useWorkspace: () => ({
    currentWorkspace: { id: 'risk', name: 'Risk' },
    workspaces: [{ id: 'risk', name: 'Risk' }],
    isLoading: false,
    setCurrentWorkspace: vi.fn(),
    switchWorkspace: vi.fn(),
  }),
}))

vi.mock('./CaseForm', () => ({
  default: () => <div data-testid="case-form" />,
}))

function fieldConfigMock(workspaceId: string): MockedResponse {
  return {
    request: {
      query: GET_FIELD_CONFIGURATION,
      variables: { workspaceId },
    },
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
  }
}

const caseRow = (id: number, title: string, status: 'OPEN' | 'CLOSED' | 'DRAFT') => ({
  __typename: 'Case',
  id,
  title,
  description: '',
  status,
  isPrivate: false,
  accessDenied: false,
  reporterID: null,
  reporter: null,
  assigneeIDs: [],
  assignees: [],
  slackChannelID: null,
  createdAt: '2026-05-01T00:00:00Z',
  updatedAt: '2026-05-01T00:00:00Z',
  fields: [],
})

function casesMock(workspaceId: string, status: 'OPEN' | 'CLOSED'): MockedResponse {
  const rows =
    status === 'OPEN'
      ? [caseRow(1, 'Open Alpha', 'OPEN')]
      : [caseRow(2, 'Closed Beta', 'CLOSED')]
  return {
    request: {
      query: GET_CASES,
      variables: { workspaceId, status },
    },
    result: { data: { cases: rows } },
  }
}

function draftsMock(workspaceId: string): MockedResponse {
  return {
    request: {
      query: GET_DRAFTS,
      variables: { workspaceId },
    },
    result: { data: { drafts: [caseRow(3, 'Draft Gamma', 'DRAFT')] } },
  }
}

interface LocationProbeRef {
  path: string
  state: unknown
}

function LocationProbe({ target }: { target: LocationProbeRef }) {
  const loc = useLocation()
  target.path = `${loc.pathname}${loc.search}`
  target.state = loc.state
  return null
}

function renderAt(initialPath: string) {
  const workspaceId = 'risk'
  const mocks: MockedResponse[] = [
    fieldConfigMock(workspaceId),
    casesMock(workspaceId, 'OPEN'),
    casesMock(workspaceId, 'CLOSED'),
    draftsMock(workspaceId),
  ]
  const probeRef: LocationProbeRef = { path: '', state: null }
  const utils = render(
    <MemoryRouter initialEntries={[initialPath]}>
      <MockedProvider mocks={mocks} addTypename={false}>
        <I18nProvider defaultLang="en">
          <Routes>
            <Route path="/ws/:workspaceId/cases" element={<CaseList />} />
            <Route path="/ws/:workspaceId/cases/:id" element={<div data-testid="detail-stub" />} />
          </Routes>
          <LocationProbe target={probeRef} />
        </I18nProvider>
      </MockedProvider>
    </MemoryRouter>,
  )
  return { ...utils, probeRef }
}

function activeTabTestId(): string | null {
  const candidates = ['status-tab-open', 'status-tab-closed', 'status-tab-draft']
  for (const id of candidates) {
    const el = screen.queryByTestId(id)
    if (el && el.className.includes('on')) return id
  }
  const segButtons = document.querySelectorAll('.seg button')
  for (const btn of Array.from(segButtons)) {
    if (btn.className.includes('on')) {
      return btn.getAttribute('data-testid') ?? 'status-tab-all'
    }
  }
  return null
}

afterEach(() => {
  cleanup()
})

describe('CaseList status tab URL binding', () => {
  it('defaults to the Open tab when no status query is present', async () => {
    renderAt('/ws/risk/cases')
    await waitFor(() => {
      expect(screen.getByTestId('status-tab-open')).toBeInTheDocument()
    })
    expect(activeTabTestId()).toBe('status-tab-open')
  })

  it('restores the Drafts tab when /cases?status=draft is opened', async () => {
    renderAt('/ws/risk/cases?status=draft')
    await waitFor(() => {
      expect(activeTabTestId()).toBe('status-tab-draft')
    })
  })

  it('restores the Closed tab when /cases?status=closed is opened', async () => {
    renderAt('/ws/risk/cases?status=closed')
    await waitFor(() => {
      expect(activeTabTestId()).toBe('status-tab-closed')
    })
  })

  it('writes ?status=closed to the URL when the user clicks the Closed tab', async () => {
    const { probeRef } = renderAt('/ws/risk/cases')
    const closedTab = await screen.findByTestId('status-tab-closed')
    fireEvent.click(closedTab)
    await waitFor(() => {
      expect(probeRef.path).toBe('/ws/risk/cases?status=closed')
    })
    expect(activeTabTestId()).toBe('status-tab-closed')
  })

  it('drops the status query when the user returns to the Open tab', async () => {
    const { probeRef } = renderAt('/ws/risk/cases?status=closed')
    const openTab = await screen.findByTestId('status-tab-open')
    fireEvent.click(openTab)
    await waitFor(() => {
      expect(probeRef.path).toBe('/ws/risk/cases')
    })
    expect(activeTabTestId()).toBe('status-tab-open')
  })

  it('falls back to the Open tab when the query value is unknown', async () => {
    renderAt('/ws/risk/cases?status=bogus')
    await waitFor(() => {
      expect(screen.getByTestId('status-tab-open')).toBeInTheDocument()
    })
    expect(activeTabTestId()).toBe('status-tab-open')
  })

  it('passes the current status through location.state on row click', async () => {
    const { probeRef } = renderAt('/ws/risk/cases?status=closed')
    await waitFor(() => {
      expect(screen.getByText('Closed Beta')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('Closed Beta'))
    await waitFor(() => {
      expect(probeRef.path).toBe('/ws/risk/cases/2')
    })
    expect(probeRef.state).toEqual({ fromStatus: 'closed' })
  })

  it('passes fromStatus=undefined when navigating from the default Open tab', async () => {
    const { probeRef } = renderAt('/ws/risk/cases')
    await waitFor(() => {
      expect(screen.getByText('Open Alpha')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('Open Alpha'))
    await waitFor(() => {
      expect(probeRef.path).toBe('/ws/risk/cases/1')
    })
    expect(probeRef.state).toEqual({ fromStatus: undefined })
  })
})
