import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { MockedProvider, type MockedResponse } from '@apollo/client/testing'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { I18nProvider } from '../i18n'
import { GET_HOME_MESSAGE, GET_MY_OPEN_CASES, GET_MY_DUE_ACTIONS } from '../graphql/dashboard'
import { toRFC3339WithOffset } from '../utils/time'
import Home from './Home'

// vi.mock factories are hoisted above imports, so a plain outer `const`
// would still be in its temporal dead zone when the factory runs.
// vi.hoisted() runs alongside the hoisted vi.mock call, avoiding that.
const { toggleFavorite } = vi.hoisted(() => ({ toggleFavorite: vi.fn() }))

vi.mock('../contexts/workspace-context', () => ({
  useWorkspace: () => ({
    currentWorkspace: null,
    workspaces: [
      { id: 'risk', name: 'Risk', emoji: '🔥', color: null },
      { id: 'support', name: 'Support', emoji: null, color: '#2f6fed' },
    ],
    isLoading: false,
    setCurrentWorkspace: vi.fn(),
    switchWorkspace: vi.fn(),
    favoriteWorkspaceIds: ['support'],
    toggleFavorite,
  }),
}))

vi.mock('../contexts/auth-context', () => ({
  useAuth: () => ({
    user: { sub: 'U1', email: 'alice@example.test', name: 'Alice' },
    isLoading: false,
    isAuthenticated: true,
    login: vi.fn(),
    logout: vi.fn(),
    checkAuth: vi.fn(),
  }),
}))

const FIXED_NOW = new Date(2026, 6, 17, 12, 0, 0) // 2026-07-17 (Friday) noon local

function homeMessageMock(message: string | null, error = false): MockedResponse {
  const base = {
    request: {
      query: GET_HOME_MESSAGE,
      // Home derives clientTime from `new Date()` at mount time; with fake
      // timers pinned to FIXED_NOW this is fully deterministic.
      variables: { clientTime: toRFC3339WithOffset(FIXED_NOW), lang: 'en' },
    },
  }
  if (error) {
    return { ...base, error: new Error('boom') } as MockedResponse
  }
  return { ...base, result: { data: { homeMessage: { message: message ?? '' } } } } as MockedResponse
}

function openCasesMock(rows: unknown[]): MockedResponse {
  return {
    request: { query: GET_MY_OPEN_CASES },
    result: { data: { myOpenCases: rows } },
  }
}

function dueActionsMock(rows: unknown[]): MockedResponse {
  return {
    request: { query: GET_MY_DUE_ACTIONS },
    result: { data: { myDueActions: rows } },
  }
}

function openCasesErrorMock(): MockedResponse {
  return { request: { query: GET_MY_OPEN_CASES }, error: new Error('boom') }
}

function dueActionsErrorMock(): MockedResponse {
  return { request: { query: GET_MY_DUE_ACTIONS }, error: new Error('boom') }
}

function caseRow(overrides: Partial<{
  workspaceId: string
  workspaceName: string
  stalled: boolean
  id: number
  title: string
  updatedAt: string
  assignees: Array<{ __typename: string; id: string; name: string; realName: string; imageUrl?: string | null }>
}> = {}) {
  const {
    workspaceId = 'risk',
    workspaceName = 'Risk',
    stalled = false,
    id = 1,
    title = 'Investigate anomaly',
    updatedAt = FIXED_NOW.toISOString(),
    assignees = [{ __typename: 'SlackUser', id: 'U1', name: 'alice', realName: 'Alice Doe', imageUrl: null }],
  } = overrides
  return {
    __typename: 'MyOpenCase',
    workspaceId,
    workspaceName,
    stalled,
    case: {
      __typename: 'Case',
      id,
      title,
      status: 'OPEN',
      assigneeIDs: assignees.map((a) => a.id),
      assignees,
      updatedAt,
    },
  }
}

function actionRow(overrides: Partial<{
  workspaceId: string
  workspaceName: string
  caseId: number
  caseTitle: string
  id: number
  title: string
  dueDate: string | null
}> = {}) {
  const {
    workspaceId = 'risk',
    workspaceName = 'Risk',
    caseId = 1,
    caseTitle = 'Investigate anomaly',
    id = 10,
    title = 'Notify stakeholders',
    dueDate = null,
  } = overrides
  return {
    __typename: 'MyDueAction',
    workspaceId,
    workspaceName,
    caseId,
    caseTitle,
    action: {
      __typename: 'Action',
      id,
      title,
      status: 'TODO',
      dueDate,
    },
  }
}

interface LocationProbeRef { path: string }
function LocationProbe({ target }: { target: LocationProbeRef }) {
  const loc = useLocation()
  target.path = `${loc.pathname}${loc.search}`
  return null
}

function renderHome(mocks: MockedResponse[], probeRef?: LocationProbeRef) {
  return render(
    <MemoryRouter initialEntries={['/']}>
      <MockedProvider mocks={mocks} addTypename={false}>
        <I18nProvider defaultLang="en">
          <Routes>
            <Route path="/" element={<Home />} />
          </Routes>
          {/* Sibling of <Routes>, not nested inside the "/" route's element,
              so it keeps tracking the location even after navigating away
              from "/" (where nothing above still matches). */}
          {probeRef && <LocationProbe target={probeRef} />}
        </I18nProvider>
      </MockedProvider>
    </MemoryRouter>,
  )
}

describe('Home', () => {
  beforeEach(() => {
    // Fake ONLY `Date` (not setTimeout/setInterval) so `new Date()` inside
    // Home is pinned to FIXED_NOW for deterministic day-diff math, while
    // Testing Library's waitFor/findBy — which poll via real setTimeout —
    // keep working normally instead of hanging.
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(FIXED_NOW)
    toggleFavorite.mockClear()
  })

  afterEach(() => {
    vi.useRealTimers()
    cleanup()
  })

  it('shows skeleton placeholders while cases/actions are loading', async () => {
    renderHome([homeMessageMock('Good afternoon'), openCasesMock([]), dueActionsMock([])])
    // Skeletons render synchronously before MockedProvider resolves.
    expect(screen.getAllByTestId('home-skeleton').length).toBeGreaterThan(0)
    await waitFor(() => expect(screen.queryAllByTestId('home-skeleton')).toHaveLength(0))
  })

  it('shows the greeting message once loaded, and the fallback on error', async () => {
    renderHome([homeMessageMock('Good afternoon, Alice'), openCasesMock([]), dueActionsMock([])])
    await waitFor(() => expect(screen.getByText('Good afternoon, Alice')).toBeInTheDocument())
  })

  it('falls back to the static greeting when the homeMessage query errors', async () => {
    renderHome([homeMessageMock(null, true), openCasesMock([]), dueActionsMock([])])
    await waitFor(() => expect(screen.getByText('Welcome back')).toBeInTheDocument())
  })

  it('falls back to the static greeting when the message is empty, without hiding the sections', async () => {
    renderHome([homeMessageMock(''), openCasesMock([caseRow()]), dueActionsMock([])])
    await waitFor(() => expect(screen.getByText('Welcome back')).toBeInTheDocument())
    await waitFor(() => expect(screen.getByText('Investigate anomaly')).toBeInTheDocument())
  })

  it('renders open-case rows with status, avatar initial, updated-ago text, workspace badge, and a stalled pill', async () => {
    renderHome([
      homeMessageMock('Hi'),
      openCasesMock([
        caseRow({
          id: 5,
          title: 'Stalled investigation',
          workspaceId: 'support',
          workspaceName: 'Support',
          stalled: true,
          updatedAt: new Date(2026, 6, 15).toISOString(), // 2 days ago
          assignees: [
            { __typename: 'SlackUser', id: 'U1', name: 'alice', realName: 'Alice Doe', imageUrl: null },
            { __typename: 'SlackUser', id: 'U2', name: 'bob', realName: 'Bob Roe', imageUrl: null },
            { __typename: 'SlackUser', id: 'U3', name: 'carol', realName: 'Carol Roe', imageUrl: null },
            { __typename: 'SlackUser', id: 'U4', name: 'dave', realName: 'Dave Roe', imageUrl: null },
          ],
        }),
      ]),
      dueActionsMock([]),
    ])

    const row = await screen.findByTestId('home-case-row')
    expect(within(row).getByText('Stalled investigation')).toBeInTheDocument()
    expect(within(row).getByText('Open')).toBeInTheDocument()
    expect(within(row).getByText('2d ago')).toBeInTheDocument()
    expect(within(row).getByText('Support')).toBeInTheDocument()
    expect(within(row).getByText('Stalled')).toBeInTheDocument()
    // 4 assignees, max 3 avatars shown + "+1" overflow
    expect(within(row).getByText('A')).toBeInTheDocument()
    expect(within(row).getByText('+1')).toBeInTheDocument()
  })

  it('shows the open-cases empty state when there are none', async () => {
    renderHome([homeMessageMock('Hi'), openCasesMock([]), dueActionsMock([])])
    await waitFor(() => expect(screen.getByText('No open cases')).toBeInTheDocument())
  })

  it('shows the due-actions empty state when there are none', async () => {
    renderHome([homeMessageMock('Hi'), openCasesMock([]), dueActionsMock([])])
    await waitFor(() => expect(screen.getByText('No due actions')).toBeInTheDocument())
  })

  it('shows an error state instead of the empty state when the open-cases query fails', async () => {
    renderHome([homeMessageMock('Hi'), openCasesErrorMock(), dueActionsMock([])])
    await waitFor(() => expect(screen.getByTestId('home-cases-error')).toBeInTheDocument())
    expect(within(screen.getByTestId('home-cases-error')).getByText('Failed to load. Please retry.')).toBeInTheDocument()
    expect(screen.queryByText('No open cases')).not.toBeInTheDocument()
  })

  it('shows an error state instead of the empty state when the due-actions query fails', async () => {
    renderHome([homeMessageMock('Hi'), openCasesMock([]), dueActionsErrorMock()])
    await waitFor(() => expect(screen.getByTestId('home-actions-error')).toBeInTheDocument())
    expect(within(screen.getByTestId('home-actions-error')).getByText('Failed to load. Please retry.')).toBeInTheDocument()
    expect(screen.queryByText('No due actions')).not.toBeInTheDocument()
  })

  it('retries the open-cases query when the retry button is clicked after a failure', async () => {
    renderHome([
      homeMessageMock('Hi'),
      openCasesErrorMock(),
      openCasesMock([caseRow({ id: 3, title: 'Recovered case' })]),
      dueActionsMock([]),
    ])
    const errorBox = await screen.findByTestId('home-cases-error')
    fireEvent.click(within(errorBox).getByText('Retry'))
    await waitFor(() => expect(screen.getByText('Recovered case')).toBeInTheDocument())
    expect(screen.queryByTestId('home-cases-error')).not.toBeInTheDocument()
  })

  it('retries the due-actions query when the retry button is clicked after a failure', async () => {
    renderHome([
      homeMessageMock('Hi'),
      openCasesMock([]),
      dueActionsErrorMock(),
      dueActionsMock([actionRow({ id: 11, title: 'Recovered action' })]),
    ])
    const errorBox = await screen.findByTestId('home-actions-error')
    fireEvent.click(within(errorBox).getByText('Retry'))
    await waitFor(() => expect(screen.getByText('Recovered action')).toBeInTheDocument())
    expect(screen.queryByTestId('home-actions-error')).not.toBeInTheDocument()
  })

  it('sorts due actions overdue-first, then today, then future, then no-due-date, and formats each accordingly', async () => {
    const overdue = new Date(2026, 6, 14).toISOString() // 3 days overdue
    const today = new Date(2026, 6, 17).toISOString()
    const future = new Date(2026, 6, 22).toISOString() // in 5 days

    renderHome([
      homeMessageMock('Hi'),
      openCasesMock([]),
      dueActionsMock([
        actionRow({ id: 1, title: 'Future task', dueDate: future }),
        actionRow({ id: 2, title: 'No due task', dueDate: null }),
        actionRow({ id: 3, title: 'Overdue task', dueDate: overdue }),
        actionRow({ id: 4, title: 'Today task', dueDate: today }),
      ]),
    ])

    const rows = await screen.findAllByTestId('home-action-row')
    expect(rows).toHaveLength(4)
    const titles = rows.map((r) => within(r).getByText(/task$/).textContent)
    expect(titles).toEqual(['Overdue task', 'Today task', 'Future task', 'No due task'])

    expect(within(rows[0]).getByText('3d overdue')).toBeInTheDocument()
    expect(within(rows[1]).getByText('Today')).toBeInTheDocument()
    expect(within(rows[2]).getByText('07/22')).toBeInTheDocument()
    expect(within(rows[3]).getByText('No due date')).toBeInTheDocument()
  })

  it('navigates to the case page when an open-case row is clicked', async () => {
    const probeRef: LocationProbeRef = { path: '' }
    renderHome(
      [homeMessageMock('Hi'), openCasesMock([caseRow({ id: 42, workspaceId: 'risk' })]), dueActionsMock([])],
      probeRef,
    )
    const row = await screen.findByTestId('home-case-row')
    fireEvent.click(row)
    expect(probeRef.path).toBe('/ws/risk/cases/42')
  })

  it('navigates to the case+action deep link when a due-action row is clicked', async () => {
    const probeRef: LocationProbeRef = { path: '' }
    renderHome(
      [
        homeMessageMock('Hi'),
        openCasesMock([]),
        dueActionsMock([actionRow({ id: 7, caseId: 9, workspaceId: 'support' })]),
      ],
      probeRef,
    )
    const row = await screen.findByTestId('home-action-row')
    fireEvent.click(row)
    expect(probeRef.path).toBe('/ws/support/cases/9/actions/7')
  })

  it('renders the workspace chooser with favorites first and wires the star toggle', async () => {
    renderHome([homeMessageMock('Hi'), openCasesMock([]), dueActionsMock([])])
    await waitFor(() => expect(screen.getByTestId('workspace-chooser')).toBeInTheDocument())

    const cards = screen.getAllByTestId(/^workspace-card-/)
    // "support" is favorited in the mocked context, so it sorts first even
    // though "risk" appears first in the raw workspace list.
    expect(cards[0]).toHaveAttribute('data-testid', 'workspace-card-support')
    expect(cards[1]).toHaveAttribute('data-testid', 'workspace-card-risk')

    fireEvent.click(screen.getByTestId('workspace-favorite-risk'))
    expect(toggleFavorite).toHaveBeenCalledWith('risk')
  })
})
