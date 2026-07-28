import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { MockedProvider, type MockedResponse } from '@apollo/client/testing'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router'
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
  createdAt: '2026-05-01T00:00:00Z',
  updatedAt: '2026-05-01T00:00:00Z',
  fields: [],
})

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
      query: GET_CASES,
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
) {
  const workspaceId = 'risk'
  const mocks: MockedResponse[] = [
    fieldConfigMock(workspaceId, fields),
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

  it('leaves the Slack cell to its own deep link when the case has a channel', async () => {
    const rows = [{ ...caseRow(12, 'With Slack', 'OPEN'), slackChannelID: 'C12345' }]
    renderAt('/ws/risk/cases', rows)
    await waitFor(() => {
      expect(screen.getByTestId('case-row-link-12')).toBeInTheDocument()
    })
    const cell = cellUnderHeader(12, 'Slack')
    expect(rowLinkIn(cell, 12)).toBeNull()
    expect(cell.querySelector('a[href="slack://channel?id=C12345"]')).not.toBeNull()
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
