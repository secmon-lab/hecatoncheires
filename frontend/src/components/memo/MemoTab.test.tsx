import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { MockedProvider, type MockedResponse } from '@apollo/client/testing'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { I18nProvider } from '../../i18n'
import { GET_MEMOS_BY_CASE, GET_MEMO_CONFIGURATION } from '../../graphql/memo'
import MemoTab from './MemoTab'

const WS = 'risk'
const CASE_ID = 7

function configMock(): MockedResponse {
  return {
    request: {
      query: GET_MEMO_CONFIGURATION,
      variables: { workspaceId: WS },
    },
    result: {
      data: {
        memoConfiguration: {
          description: '',
          fields: [
            {
              id: 'severity',
              name: 'Severity',
              type: 'SELECT',
              required: false,
              description: null,
              options: [{ id: 'high', name: 'High', description: null, metadata: null }],
            },
          ],
        },
      },
    },
  }
}

function memosMock(): MockedResponse {
  return {
    request: {
      query: GET_MEMOS_BY_CASE,
      variables: { workspaceId: WS, caseID: CASE_ID, filter: 'ACTIVE' },
    },
    result: {
      data: {
        memosByCase: [
          {
            id: 'memo-a',
            caseID: CASE_ID,
            title: 'First memo',
            fields: [{ fieldId: 'severity', value: 'high' }],
            archivedAt: null,
            createdAt: '2026-07-13T01:00:00Z',
            updatedAt: '2026-07-13T02:00:00Z',
          },
          {
            id: 'memo-b',
            caseID: CASE_ID,
            title: 'Second memo',
            fields: [],
            archivedAt: null,
            createdAt: '2026-07-14T01:00:00Z',
            updatedAt: '2026-07-14T02:00:00Z',
          },
        ],
      },
    },
  }
}

interface LocationProbeRef {
  path: string
}

function LocationProbe({ target }: { target: LocationProbeRef }) {
  const loc = useLocation()
  target.path = loc.pathname
  return null
}

function renderTab(props: { accessDenied?: boolean } = {}) {
  const probeRef: LocationProbeRef = { path: '' }
  const utils = render(
    <MemoryRouter initialEntries={[`/ws/${WS}/cases/${CASE_ID}`]}>
      <MockedProvider mocks={[configMock(), memosMock()]} addTypename={false}>
        <I18nProvider defaultLang="en">
          <MemoTab caseId={CASE_ID} workspaceId={WS} accessDenied={props.accessDenied} />
          <LocationProbe target={probeRef} />
        </I18nProvider>
      </MockedProvider>
    </MemoryRouter>,
  )
  return { ...utils, probeRef }
}

afterEach(cleanup)

describe('MemoTab', () => {
  it('renders memo rows with their titles and summary chips', async () => {
    renderTab()

    const rows = await screen.findAllByTestId('memo-row')
    expect(rows).toHaveLength(2)
    expect(rows[0]).toHaveTextContent('First memo')
    // Summary chip resolves the SELECT option name.
    expect(rows[0]).toHaveTextContent('High')
    expect(rows[1]).toHaveTextContent('Second memo')
  })

  it('navigates to the memo detail page when a row is clicked', async () => {
    const { probeRef } = renderTab()

    const rows = await screen.findAllByTestId('memo-row')
    fireEvent.click(rows[0])

    expect(probeRef.path).toBe(`/ws/${WS}/cases/${CASE_ID}/memos/memo-a`)
  })

  it('renders nothing when access is denied', () => {
    const { container } = renderTab({ accessDenied: true })

    expect(container.textContent).toBe('')
    expect(screen.queryByTestId('memo-row')).not.toBeInTheDocument()
  })
})
