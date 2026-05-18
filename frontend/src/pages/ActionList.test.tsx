import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within, cleanup } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { MockedProvider } from '@apollo/client/testing'
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom'
import { I18nProvider } from '../i18n'
import { GET_OPEN_CASE_ACTIONS } from '../graphql/action'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import ActionList from './ActionList'

// Pin the workspace context so the page renders without a real fetch.
vi.mock('../contexts/workspace-context', () => ({
  useWorkspace: () => ({
    currentWorkspace: { id: 'risk', name: 'Risk' },
    workspaces: [{ id: 'risk', name: 'Risk' }],
    isLoading: false,
    setCurrentWorkspace: vi.fn(),
    switchWorkspace: vi.fn(),
  }),
}))

// ActionModal is not under test here and pulls in heavier queries; stub it.
vi.mock('./ActionModal', () => ({
  default: ({ actionId }: { actionId: number }) => (
    <div data-testid="action-modal">action-modal:{actionId}</div>
  ),
}))

// ActionForm is not under test here.
vi.mock('./ActionForm', () => ({
  default: () => <div data-testid="action-form" />,
}))

const fieldConfigMock = {
  request: {
    query: GET_FIELD_CONFIGURATION,
    variables: { workspaceId: 'risk' },
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
          statuses: [
            { __typename: 'ActionStatus', id: 'BACKLOG', name: 'Backlog', description: null, color: 'idle', emoji: null },
            { __typename: 'ActionStatus', id: 'TODO', name: 'To Do', description: null, color: 'idle', emoji: null },
            { __typename: 'ActionStatus', id: 'IN_PROGRESS', name: 'In Progress', description: null, color: 'active', emoji: null },
            { __typename: 'ActionStatus', id: 'BLOCKED', name: 'Blocked', description: null, color: 'blocked', emoji: null },
            { __typename: 'ActionStatus', id: 'COMPLETED', name: 'Completed', description: null, color: 'success', emoji: null },
          ],
        },
      },
    },
  },
}

const actionRow = (
  id: number,
  caseID: number,
  caseTitle: string,
  title: string,
  status = 'IN_PROGRESS',
) => ({
  __typename: 'Action',
  id,
  caseID,
  case: { __typename: 'Case', id: caseID, title: caseTitle, slackChannelID: null, slackChannelURL: null },
  title,
  description: '',
  assigneeID: null,
  assignee: null,
  slackMessageTS: null,
  status,
  dueDate: null,
  archived: false,
  archivedAt: null,
  createdAt: '2026-05-01T00:00:00Z',
  updatedAt: '2026-05-01T00:00:00Z',
  stepProgress: { __typename: 'StepProgress', done: 0, total: 0 },
})

const actionsMock = {
  request: {
    query: GET_OPEN_CASE_ACTIONS,
    variables: { workspaceId: 'risk' },
  },
  result: {
    data: {
      openCaseActions: [
        actionRow(101, 3, 'GitHub incident', 'Build detection pipeline'),
        actionRow(102, 3, 'GitHub incident', 'Update WIF policy', 'TODO'),
        actionRow(103, 4, 'Other case', 'Unrelated action'),
      ],
    },
  },
}

let lastLocation = ''
function LocationProbe() {
  const loc = useLocation()
  lastLocation = `${loc.pathname}${loc.search}`
  return null
}

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <MockedProvider mocks={[fieldConfigMock, actionsMock]}>
        <I18nProvider defaultLang="en">
          <Routes>
            <Route path="/ws/:workspaceId/actions" element={<ActionList />} />
            <Route path="/ws/:workspaceId/actions/:actionId" element={<ActionList />} />
            <Route path="/ws/:workspaceId/actions/case/:caseId" element={<ActionList />} />
            <Route path="/ws/:workspaceId/actions/case/:caseId/:actionId" element={<ActionList />} />
          </Routes>
          <LocationProbe />
        </I18nProvider>
      </MockedProvider>
    </MemoryRouter>,
  )
}

beforeEach(() => {
  lastLocation = ''
})

afterEach(() => {
  cleanup()
})

describe('ActionList case filter', () => {
  it('renders every action when no case filter is in the URL', async () => {
    renderAt('/ws/risk/actions')
    await waitFor(() => {
      expect(screen.getAllByTestId('action-card')).toHaveLength(3)
    })
    expect(screen.queryByTestId('action-case-filter-chip')).not.toBeInTheDocument()
  })

  it('restricts the board to the matching case when /actions/case/:caseId is opened', async () => {
    renderAt('/ws/risk/actions/case/3')
    await waitFor(() => {
      expect(screen.getAllByTestId('action-card')).toHaveLength(2)
    })
    const chip = screen.getByTestId('action-case-filter-chip')
    expect(chip).toHaveTextContent('Case: #3 GitHub incident')
  })

  it('navigates to the filtered URL when the case label on a card is clicked', async () => {
    renderAt('/ws/risk/actions')
    await waitFor(() => {
      expect(screen.getAllByTestId('action-card')).toHaveLength(3)
    })
    const card = screen.getAllByTestId('action-card')[0]
    const caseLink = within(card).getByTestId('action-card-case-link')
    fireEvent.click(caseLink)
    await waitFor(() => {
      expect(lastLocation).toBe('/ws/risk/actions/case/3')
    })
    // Modal should not have opened — propagation was stopped.
    expect(screen.queryByTestId('action-modal')).not.toBeInTheDocument()
  })

  it('returns to the unfiltered URL when the chip clear button is clicked', async () => {
    renderAt('/ws/risk/actions/case/3')
    const clearBtn = await screen.findByTestId('action-case-filter-clear')
    fireEvent.click(clearBtn)
    await waitFor(() => {
      expect(lastLocation).toBe('/ws/risk/actions')
    })
  })

  it('ignores a non-numeric caseId and shows every action', async () => {
    renderAt('/ws/risk/actions/case/not-a-number')
    await waitFor(() => {
      expect(screen.getAllByTestId('action-card')).toHaveLength(3)
    })
    expect(screen.queryByTestId('action-case-filter-chip')).not.toBeInTheDocument()
  })

  it('combines text search with case filter (AND)', async () => {
    renderAt('/ws/risk/actions/case/3')
    await waitFor(() => {
      expect(screen.getAllByTestId('action-card')).toHaveLength(2)
    })
    const search = screen.getByTestId('action-search-input') as HTMLInputElement
    fireEvent.change(search, { target: { value: 'WIF' } })
    await waitFor(() => {
      expect(screen.getAllByTestId('action-card')).toHaveLength(1)
    })
    expect(screen.getByText('Update WIF policy')).toBeInTheDocument()
  })
})
