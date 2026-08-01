import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { ApolloClient, ApolloProvider, InMemoryCache } from '@apollo/client'
import { MockedProvider, MockLink, type MockedResponse } from '@apollo/client/testing'
import { GraphQLError } from 'graphql'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router'
import { I18nProvider } from '../i18n'
import { GET_CASES, GET_CASES_WITH_SLACK_LINK } from '../graphql/case'
import { GET_CASE_STATUS_CONFIG } from '../graphql/caseStatus'
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

interface FieldDef {
  id: string
  name: string
  type: string
  options?: null
}

function fieldConfigMock(workspaceId: string, fields: FieldDef[] = []): MockedResponse {
  return {
    request: {
      query: GET_FIELD_CONFIGURATION,
      variables: { workspaceId },
    },
    result: {
      data: {
        fieldConfiguration: {
          __typename: 'FieldConfiguration',
          fields,
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

// Declared explicitly rather than inferred: tests override slackChannelID and
// fields, and the inferred literal types (null / never[]) would reject them.
interface CaseRowMock {
  __typename: string
  id: number
  title: string
  description: string
  status: 'OPEN' | 'CLOSED' | 'DRAFT'
  isPrivate: boolean
  accessDenied: boolean
  reporterID: string | null
  reporter: null
  assigneeIDs: string[]
  assignees: unknown[]
  slackChannelID: string | null
  slackChannelURL: string | null
  slackThreadTS: string | null
  isThreadBound: boolean
  boardStatus: string | null
  createdAt: string
  updatedAt: string
  fields: Array<{ fieldId: string; value: unknown }>
}

const caseRow = (id: number, title: string, status: 'OPEN' | 'CLOSED' | 'DRAFT'): CaseRowMock => ({
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
  slackChannelURL: null,
  slackThreadTS: null,
  isThreadBound: false,
  boardStatus: null,
  createdAt: '2026-05-01T00:00:00Z',
  updatedAt: '2026-05-01T00:00:00Z',
  fields: [],
})

interface StatusDefMock {
  __typename: string
  id: string
  name: string
  description: string | null
  color: string | null
  emoji: string | null
}

interface StatusConfigMock {
  __typename: string
  initial: string
  closed: string[]
  statuses: StatusDefMock[]
}

const statusDef = (id: string, name: string, color: string): StatusDefMock => ({
  __typename: 'ActionStatusDef',
  id,
  name,
  description: null,
  color,
  emoji: null,
})

// A thread-mode workspace exposes a configurable Case status set; a
// channel-mode workspace resolves caseStatusConfig to null.
const THREAD_STATUS_CONFIG: StatusConfigMock = {
  __typename: 'ActionConfig',
  initial: 'triage',
  closed: ['done'],
  statuses: [
    statusDef('triage', 'Triage', 'backlog'),
    statusDef('in_review', 'In Review', 'blocked'),
    statusDef('done', 'Done', 'completed'),
  ],
}

function caseStatusConfigMock(
  workspaceId: string,
  config: StatusConfigMock | null,
): MockedResponse {
  return {
    request: {
      query: GET_CASE_STATUS_CONFIG,
      variables: { workspaceId },
    },
    result: { data: { caseStatusConfig: config } },
  }
}

function casesMock(
  workspaceId: string,
  status: 'OPEN' | 'CLOSED',
  rows?: CaseRowMock[],
): MockedResponse {
  const defaultRows =
    status === 'OPEN'
      ? [caseRow(1, 'Open Alpha', 'OPEN')]
      : [caseRow(2, 'Closed Beta', 'CLOSED')]
  return {
    request: {
      query: GET_CASES_WITH_SLACK_LINK,
      variables: { workspaceId, status },
    },
    result: { data: { cases: rows ?? defaultRows } },
  }
}

// Row titles are zero-padded so "Open 021" is findable by exact text without
// also matching "Open 0210".
function numberedOpenCases(count: number) {
  return Array.from({ length: count }, (_, i) =>
    caseRow(100 + i, `Open ${String(i + 1).padStart(3, '0')}`, 'OPEN'),
  )
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

function renderAt(
  initialPath: string,
  openRows?: CaseRowMock[],
  fields?: FieldDef[],
  // Defaults to the channel-mode shape (null), which is also the state the
  // page falls back to while the query is in flight or after it errors.
  statusConfig: StatusConfigMock | null = null,
) {
  const workspaceId = 'risk'
  const mocks: MockedResponse[] = [
    fieldConfigMock(workspaceId, fields),
    caseStatusConfigMock(workspaceId, statusConfig),
    casesMock(workspaceId, 'OPEN', openRows),
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
  // The page-size preference and the column selection persist in
  // localStorage, which jsdom keeps alive across tests in a file.
  localStorage.clear()
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

// The cell under a given column header, for the row holding the given title
// link. Resolved through the header text so the assertions do not encode the
// column order.
function cellUnderHeader(caseId: number, header: string): HTMLElement {
  const row = screen.getByTestId(`case-row-link-${caseId}`).closest('tr')
  if (!row) throw new Error(`row not found for case ${caseId}`)
  const table = row.closest('table')
  if (!table) throw new Error('table not found')
  const headers = Array.from(table.querySelectorAll('thead th')).map(
    (h) => h.textContent?.trim().toLowerCase() ?? '',
  )
  const index = headers.indexOf(header.toLowerCase())
  if (index < 0) throw new Error(`header not found: ${header} (have: ${headers.join(', ')})`)
  return row.querySelectorAll('td')[index] as HTMLElement
}

function rowLinkIn(cell: HTMLElement, caseId: number): Element | null {
  return cell.querySelector(`a[href="/ws/risk/cases/${caseId}"]`)
}

describe('CaseList row links', () => {
  it('renders the case row as an anchor pointing at the detail page', async () => {
    renderAt('/ws/risk/cases')
    const link = await screen.findByTestId('case-row-link-1')
    expect(link).toHaveAttribute('href', '/ws/risk/cases/1')
  })

  it('renders no row anchor for an access-denied case', async () => {
    const rows = [{ ...caseRow(7, 'Hidden', 'OPEN'), accessDenied: true, isPrivate: true }]
    renderAt('/ws/risk/cases', rows)
    await waitFor(() => {
      expect(screen.getByTestId('access-denied-label')).toBeInTheDocument()
    })
    expect(screen.queryByTestId('case-row-link-7')).not.toBeInTheDocument()
  })

  it('covers the Slack cell with the row link when the case has no Slack channel', async () => {
    renderAt('/ws/risk/cases', [caseRow(11, 'No Slack', 'OPEN')])
    await waitFor(() => {
      expect(screen.getByTestId('case-row-link-11')).toBeInTheDocument()
    })
    expect(rowLinkIn(cellUnderHeader(11, 'Slack'), 11)).not.toBeNull()
  })

  it('leaves the Slack cell to its own link when the case has a channel', async () => {
    const rows = [
      {
        ...caseRow(12, 'With Slack', 'OPEN'),
        slackChannelID: 'C12345',
        slackChannelURL: 'https://acme.slack.com/archives/C12345',
      },
    ]
    renderAt('/ws/risk/cases', rows)
    await waitFor(() => {
      expect(screen.getByTestId('case-row-link-12')).toBeInTheDocument()
    })
    const cell = cellUnderHeader(12, 'Slack')
    expect(rowLinkIn(cell, 12)).toBeNull()
    expect(
      cell.querySelector('a[href="https://acme.slack.com/archives/C12345"]'),
    ).not.toBeNull()
  })

  it('covers a URL field cell with the row link when the field is empty', async () => {
    localStorage.setItem(
      'caseListColumns:risk',
      JSON.stringify(['status', 'assignees', 'reporter', 'created', 'slack', 'field:doc']),
    )
    const fields = [{ id: 'doc', name: 'Doc', type: 'URL', options: null }]
    renderAt('/ws/risk/cases', [caseRow(13, 'No Doc', 'OPEN')], fields)
    await waitFor(() => {
      expect(screen.getByTestId('case-row-link-13')).toBeInTheDocument()
    })
    expect(rowLinkIn(cellUnderHeader(13, 'Doc'), 13)).not.toBeNull()
  })

  it('leaves a URL field cell to its own link when the field has a value', async () => {
    localStorage.setItem(
      'caseListColumns:risk',
      JSON.stringify(['status', 'assignees', 'reporter', 'created', 'slack', 'field:doc']),
    )
    const fields = [{ id: 'doc', name: 'Doc', type: 'URL', options: null }]
    const rows = [
      { ...caseRow(14, 'With Doc', 'OPEN'), fields: [{ fieldId: 'doc', value: 'https://example.com/x' }] },
    ]
    renderAt('/ws/risk/cases', rows, fields)
    await waitFor(() => {
      expect(screen.getByTestId('case-row-link-14')).toBeInTheDocument()
    })
    const cell = cellUnderHeader(14, 'Doc')
    expect(rowLinkIn(cell, 14)).toBeNull()
    expect(cell.querySelector('a[href="https://example.com/x"]')).not.toBeNull()
  })
})

describe('CaseList status column', () => {
  const threadRow = (id: number, title: string, boardStatus: string | null) => ({
    ...caseRow(id, title, 'OPEN' as const),
    boardStatus,
    slackThreadTS: '1700000000.123456',
    isThreadBound: true,
    slackChannelID: 'C999',
  })

  it('shows the configured board status in a thread-mode workspace', async () => {
    renderAt('/ws/risk/cases', [threadRow(30, 'Thread Case', 'in_review')], undefined, THREAD_STATUS_CONFIG)
    await waitFor(() => {
      expect(screen.getByTestId('board-status-badge')).toBeInTheDocument()
    })
    const cell = cellUnderHeader(30, 'Status')
    expect(cell).toHaveTextContent('In Review')
    // The lifecycle badge must not sit alongside it (the Open/Closed tabs
    // above the table carry those words too, so scope the check to the cell).
    expect(cell).not.toHaveTextContent('Open')
  })

  it('shows the lifecycle status in a channel-mode workspace', async () => {
    renderAt('/ws/risk/cases', [caseRow(31, 'Channel Case', 'OPEN')])
    await waitFor(() => {
      expect(screen.getByTestId('case-row-link-31')).toBeInTheDocument()
    })
    expect(cellUnderHeader(31, 'Status')).toHaveTextContent('Open')
    expect(screen.queryByTestId('board-status-badge')).not.toBeInTheDocument()
  })

  it('falls back to the lifecycle status when a thread-mode case has none set', async () => {
    renderAt('/ws/risk/cases', [threadRow(32, 'No Board Status', null)], undefined, THREAD_STATUS_CONFIG)
    await waitFor(() => {
      expect(screen.getByTestId('case-row-link-32')).toBeInTheDocument()
    })
    expect(cellUnderHeader(32, 'Status')).toHaveTextContent('Open')
    expect(screen.queryByTestId('board-status-badge')).not.toBeInTheDocument()
  })

  it('shows the raw id when the board status is no longer in the configuration', async () => {
    renderAt('/ws/risk/cases', [threadRow(33, 'Removed Status', 'ghost')], undefined, THREAD_STATUS_CONFIG)
    await waitFor(() => {
      expect(screen.getByTestId('board-status-badge')).toBeInTheDocument()
    })
    expect(cellUnderHeader(33, 'Status')).toHaveTextContent('ghost')
  })

  it('shows the Draft badge on the Drafts tab of a thread-mode workspace', async () => {
    renderAt('/ws/risk/cases?status=draft', undefined, undefined, THREAD_STATUS_CONFIG)
    await waitFor(() => {
      expect(screen.getByTestId('case-row-link-3')).toBeInTheDocument()
    })
    expect(cellUnderHeader(3, 'Status')).toHaveTextContent('Drafts')
    expect(screen.queryByTestId('board-status-badge')).not.toBeInTheDocument()
  })
})

describe('CaseList Slack link', () => {
  it('links a thread-mode case to its thread, not to the monitored channel', async () => {
    const rows = [
      {
        ...caseRow(40, 'Thread Case', 'OPEN' as const),
        slackChannelID: 'C123',
        slackChannelURL: 'https://acme.slack.com/archives/C123',
        slackThreadTS: '1700000000.123456',
        isThreadBound: true,
        boardStatus: 'triage',
      },
    ]
    renderAt('/ws/risk/cases', rows, undefined, THREAD_STATUS_CONFIG)
    await waitFor(() => {
      expect(screen.getByTestId('case-row-link-40')).toBeInTheDocument()
    })
    const cell = cellUnderHeader(40, 'Slack')
    expect(
      cell.querySelector('a[href="https://acme.slack.com/archives/C123/p1700000000123456"]'),
    ).not.toBeNull()
  })

  it('links a channel-mode case to its own channel', async () => {
    const rows = [
      {
        ...caseRow(41, 'Channel Case', 'OPEN' as const),
        slackChannelID: 'C456',
        slackChannelURL: 'https://acme.slack.com/archives/C456',
      },
    ]
    renderAt('/ws/risk/cases', rows)
    await waitFor(() => {
      expect(screen.getByTestId('case-row-link-41')).toBeInTheDocument()
    })
    const cell = cellUnderHeader(41, 'Slack')
    expect(cell.querySelector('a[href="https://acme.slack.com/archives/C456"]')).not.toBeNull()
  })

  it('falls back to the canonical slack.com host when the team URL is unavailable', async () => {
    const rows = [{ ...caseRow(42, 'No Team URL', 'OPEN' as const), slackChannelID: 'C789' }]
    renderAt('/ws/risk/cases', rows)
    await waitFor(() => {
      expect(screen.getByTestId('case-row-link-42')).toBeInTheDocument()
    })
    const cell = cellUnderHeader(42, 'Slack')
    expect(cell.querySelector('a[href="https://slack.com/archives/C789"]')).not.toBeNull()
  })

  it('renders no link when the case has no Slack channel', async () => {
    renderAt('/ws/risk/cases', [caseRow(43, 'No Slack', 'OPEN')])
    await waitFor(() => {
      expect(screen.getByTestId('case-row-link-43')).toBeInTheDocument()
    })
    const cell = cellUnderHeader(43, 'Slack')
    expect(cell.querySelector('a.slack-link')).toBeNull()
  })

  it('still renders the rows when the slackChannelURL field resolves to an error', async () => {
    // The resolver behind slackChannelURL calls Slack's auth.test and caches
    // its failure for the life of the process. Under the default errorPolicy
    // Apollo would discard the whole payload and blank the list.
    const workspaceId = 'risk'
    const row = { ...caseRow(44, 'Slack Broken', 'OPEN' as const), slackChannelID: 'C000' }
    const mocks: MockedResponse[] = [
      fieldConfigMock(workspaceId),
      caseStatusConfigMock(workspaceId, null),
      {
        request: { query: GET_CASES_WITH_SLACK_LINK, variables: { workspaceId, status: 'OPEN' } },
        result: {
          data: { cases: [row] },
          errors: [new GraphQLError('failed to get Slack team URL')],
        },
      },
      casesMock(workspaceId, 'CLOSED'),
      draftsMock(workspaceId),
    ]
    render(
      <MemoryRouter initialEntries={['/ws/risk/cases']}>
        <MockedProvider mocks={mocks} addTypename={false}>
          <I18nProvider defaultLang="en">
            <Routes>
              <Route path="/ws/:workspaceId/cases" element={<CaseList />} />
            </Routes>
          </I18nProvider>
        </MockedProvider>
      </MemoryRouter>,
    )
    expect(await screen.findByText('Slack Broken')).toBeInTheDocument()
  })
})

describe('CaseList page size', () => {
  it('shows 20 rows per page by default', async () => {
    renderAt('/ws/risk/cases', numberedOpenCases(25))
    await waitFor(() => {
      expect(screen.getByText('Open 020')).toBeInTheDocument()
    })
    expect(screen.queryByText('Open 021')).not.toBeInTheDocument()
    expect(screen.getByTestId('pagination-info')).toHaveTextContent('1 / 2')
    expect(screen.getByTestId('page-size-select')).toHaveValue('20')
  })

  it('renders more rows and persists the choice when the size changes to 50', async () => {
    renderAt('/ws/risk/cases', numberedOpenCases(25))
    const select = await screen.findByTestId('page-size-select')
    fireEvent.change(select, { target: { value: '50' } })
    await waitFor(() => {
      expect(screen.getByText('Open 025')).toBeInTheDocument()
    })
    expect(screen.getByTestId('pagination-info')).toHaveTextContent('1 / 1')
    expect(localStorage.getItem('caseListPageSize')).toBe('50')
  })

  it('restores the persisted page size on the next visit', async () => {
    localStorage.setItem('caseListPageSize', '100')
    renderAt('/ws/risk/cases', numberedOpenCases(25))
    await waitFor(() => {
      expect(screen.getByTestId('page-size-select')).toHaveValue('100')
    })
    expect(screen.getByText('Open 025')).toBeInTheDocument()
  })

  it('falls back to the default when the persisted value is not an offered option', async () => {
    localStorage.setItem('caseListPageSize', '37')
    renderAt('/ws/risk/cases', numberedOpenCases(25))
    await waitFor(() => {
      expect(screen.getByTestId('page-size-select')).toHaveValue('20')
    })
  })

  it('returns to the first page when the size changes', async () => {
    const { probeRef } = renderAt('/ws/risk/cases?page=2', numberedOpenCases(25))
    await waitFor(() => {
      expect(screen.getByText('Open 021')).toBeInTheDocument()
    })
    fireEvent.change(screen.getByTestId('page-size-select'), { target: { value: '50' } })
    await waitFor(() => {
      expect(probeRef.path).toBe('/ws/risk/cases')
    })
    expect(screen.getByText('Open 001')).toBeInTheDocument()
  })
})

describe('CaseList page URL binding', () => {
  it('restores the requested page when /cases?page=2 is opened', async () => {
    renderAt('/ws/risk/cases?page=2', numberedOpenCases(25))
    await waitFor(() => {
      expect(screen.getByText('Open 021')).toBeInTheDocument()
    })
    expect(screen.queryByText('Open 020')).not.toBeInTheDocument()
    expect(screen.getByTestId('pagination-info')).toHaveTextContent('2 / 2')
  })

  it('writes ?page=2 to the URL when the user pages forward, and drops it going back', async () => {
    const { probeRef } = renderAt('/ws/risk/cases', numberedOpenCases(25))
    await waitFor(() => {
      expect(screen.getByText('Open 001')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByTestId('pagination-next'))
    await waitFor(() => {
      expect(probeRef.path).toBe('/ws/risk/cases?page=2')
    })
    fireEvent.click(screen.getByTestId('pagination-prev'))
    await waitFor(() => {
      expect(probeRef.path).toBe('/ws/risk/cases')
    })
  })

  it('clamps a page beyond the last one to the last page', async () => {
    renderAt('/ws/risk/cases?page=99', numberedOpenCases(25))
    await waitFor(() => {
      expect(screen.getByText('Open 021')).toBeInTheDocument()
    })
    expect(screen.getByTestId('pagination-info')).toHaveTextContent('2 / 2')
  })

  it('ignores a non-numeric page value', async () => {
    renderAt('/ws/risk/cases?page=bogus', numberedOpenCases(25))
    await waitFor(() => {
      expect(screen.getByText('Open 001')).toBeInTheDocument()
    })
    expect(screen.getByTestId('pagination-info')).toHaveTextContent('1 / 2')
  })

  it('drops the page when the user switches tab', async () => {
    const { probeRef } = renderAt('/ws/risk/cases?page=2', numberedOpenCases(25))
    const closedTab = await screen.findByTestId('status-tab-closed')
    fireEvent.click(closedTab)
    await waitFor(() => {
      expect(probeRef.path).toBe('/ws/risk/cases?status=closed')
    })
  })

  it('carries the current page into the detail page state so Back can return to it', async () => {
    const { probeRef } = renderAt('/ws/risk/cases?page=2', numberedOpenCases(25))
    await waitFor(() => {
      expect(screen.getByText('Open 021')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByText('Open 021'))
    await waitFor(() => {
      expect(probeRef.path).toBe('/ws/risk/cases/120')
    })
    expect(probeRef.state).toEqual({ fromStatus: undefined, fromPage: 2 })
  })
})

describe('CaseList cache sharing with the narrower GET_CASES', () => {
  // The Case list watches GET_CASES_WITH_SLACK_LINK, but every other page —
  // and every post-mutation refetch — writes the same normalised `cases`
  // entry through the narrower GET_CASES, which carries no slackChannelURL.
  // A freshly created Case therefore lands in the cache without that field
  // and makes the list's cache read incomplete.
  it('keeps rendering rows after a GET_CASES refetch writes a Case with no slackChannelURL', async () => {
    const workspaceId = 'risk'
    const existing = {
      ...caseRow(60, 'Existing Row', 'OPEN' as const),
      slackChannelID: 'C60',
      slackChannelURL: 'https://acme.slack.com/archives/C60',
    }
    const created = { ...caseRow(61, 'Created Row', 'OPEN' as const), slackChannelID: 'C61' }
    const narrowCreated: Record<string, unknown> = { ...created }
    delete narrowCreated.slackChannelURL

    const cache = new InMemoryCache({ addTypename: false })
    const client = new ApolloClient({
      cache,
      link: new MockLink(
        [
          fieldConfigMock(workspaceId),
          caseStatusConfigMock(workspaceId, null),
          casesMock(workspaceId, 'OPEN', [existing]),
          casesMock(workspaceId, 'CLOSED'),
          draftsMock(workspaceId),
        ],
        false,
      ),
    })

    render(
      <MemoryRouter initialEntries={['/ws/risk/cases']}>
        <ApolloProvider client={client}>
          <I18nProvider defaultLang="en">
            <Routes>
              <Route path="/ws/:workspaceId/cases" element={<CaseList />} />
            </Routes>
          </I18nProvider>
        </ApolloProvider>
      </MemoryRouter>,
    )
    expect(await screen.findByText('Existing Row')).toBeInTheDocument()

    act(() => {
      cache.writeQuery({
        query: GET_CASES,
        variables: { workspaceId, status: 'OPEN' },
        data: { cases: [existing, narrowCreated] },
      })
    })

    await waitFor(() => {
      expect(screen.getByText('Created Row')).toBeInTheDocument()
    })
    expect(screen.getByText('Existing Row')).toBeInTheDocument()
  })
})
