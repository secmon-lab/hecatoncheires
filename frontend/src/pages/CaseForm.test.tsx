import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, render, screen } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { MockedProvider, type MockedResponse } from '@apollo/client/testing'
import { MemoryRouter } from 'react-router-dom'
import { I18nProvider } from '../i18n'
import { GET_FIELD_CONFIGURATION } from '../graphql/fieldConfiguration'
import { GET_SLACK_USERS } from '../graphql/slackUsers'
import { GET_CASE_STATUS_CONFIG } from '../graphql/caseStatus'
import CaseForm from './CaseForm'

const WORKSPACE_ID = 'risk'

vi.mock('../contexts/workspace-context', () => ({
  useWorkspace: () => ({
    currentWorkspace: { id: WORKSPACE_ID, name: 'Risk' },
    workspaces: [{ id: WORKSPACE_ID, name: 'Risk' }],
    isLoading: false,
    setCurrentWorkspace: vi.fn(),
    switchWorkspace: vi.fn(),
  }),
}))

function fieldConfigMock(): MockedResponse {
  return {
    request: { query: GET_FIELD_CONFIGURATION, variables: { workspaceId: WORKSPACE_ID } },
    result: {
      data: {
        fieldConfiguration: {
          fields: [],
          labels: { case: 'Case' },
          actionConfig: { initial: 'BACKLOG', closed: ['COMPLETED'], statuses: [] },
        },
      },
    },
  }
}

function slackUsersMock(): MockedResponse {
  return {
    request: { query: GET_SLACK_USERS },
    result: { data: { slackUsers: [] } },
  }
}

// channelStatusMock resolves caseStatusConfig to null — the shape a channel-mode
// workspace returns (no configurable Case status set / Kanban).
function channelStatusMock(): MockedResponse {
  return {
    request: { query: GET_CASE_STATUS_CONFIG, variables: { workspaceId: WORKSPACE_ID } },
    result: { data: { caseStatusConfig: null } },
  }
}

// threadStatusMock resolves caseStatusConfig to a populated status set — only a
// thread-mode workspace exposes one.
function threadStatusMock(): MockedResponse {
  return {
    request: { query: GET_CASE_STATUS_CONFIG, variables: { workspaceId: WORKSPACE_ID } },
    result: {
      data: {
        caseStatusConfig: {
          initial: 'triage',
          closed: ['done'],
          statuses: [
            { id: 'triage', name: 'Triage', description: '', color: '#888', emoji: null },
            { id: 'done', name: 'Done', description: '', color: '#0a0', emoji: null },
          ],
        },
      },
    },
  }
}

// erroredStatusMock makes the status query fail — the mode is then UNKNOWN.
function erroredStatusMock(): MockedResponse {
  return {
    request: { query: GET_CASE_STATUS_CONFIG, variables: { workspaceId: WORKSPACE_ID } },
    error: new Error('status config query failed'),
  }
}

// pendingStatusMock never resolves within the test window, keeping the status
// query in its loading state.
function pendingStatusMock(): MockedResponse {
  return {
    request: { query: GET_CASE_STATUS_CONFIG, variables: { workspaceId: WORKSPACE_ID } },
    result: { data: { caseStatusConfig: null } },
    delay: 100_000,
  }
}

function renderForm(statusMock: MockedResponse) {
  return render(
    <MemoryRouter initialEntries={[`/ws/${WORKSPACE_ID}/cases`]}>
      <MockedProvider mocks={[fieldConfigMock(), slackUsersMock(), statusMock]} addTypename={false}>
        <I18nProvider defaultLang="en">
          <CaseForm caseItem={null} onClose={vi.fn()} />
        </I18nProvider>
      </MockedProvider>
    </MemoryRouter>,
  )
}

// flush lets MockedProvider deliver its (macrotask-scheduled) query results and
// React apply the resulting state updates, all inside act().
async function flush() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0))
  })
}

afterEach(() => {
  cleanup()
})

describe('CaseForm private-case toggle visibility', () => {
  it('shows the Private toggle only once channel mode is confirmed', async () => {
    renderForm(channelStatusMock())
    // The form renders immediately; the toggle appears after the status query
    // resolves and confirms channel mode.
    expect(await screen.findByTestId('private-case-checkbox')).toBeInTheDocument()
  })

  it('hides the Private toggle in thread mode', async () => {
    renderForm(threadStatusMock())
    await screen.findByTestId('case-title-input')
    await flush()
    expect(screen.queryByTestId('private-case-checkbox')).not.toBeInTheDocument()
  })

  it('hides the Private toggle while the workspace mode is still loading', async () => {
    renderForm(pendingStatusMock())
    await screen.findByTestId('case-title-input')
    await flush()
    // The status query is still in flight, so the mode is not yet known.
    expect(screen.queryByTestId('private-case-checkbox')).not.toBeInTheDocument()
  })

  it('hides the Private toggle when the mode query errors (mode unknown)', async () => {
    renderForm(erroredStatusMock())
    await screen.findByTestId('case-title-input')
    await flush()
    expect(screen.queryByTestId('private-case-checkbox')).not.toBeInTheDocument()
  })
})
