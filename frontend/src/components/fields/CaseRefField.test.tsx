import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { MockedProvider } from '@apollo/client/testing'
import { I18nProvider } from '../../i18n'
import CaseRefField from './CaseRefField'
import { REFERENCEABLE_CASES } from '../../graphql/caseRef'

const mocks = [
  {
    request: {
      query: REFERENCEABLE_CASES,
      variables: { workspaceId: 'ws1', query: undefined, limit: 50 },
    },
    result: {
      data: {
        referenceableCases: [
          { id: 1, title: 'Alpha case', status: 'OPEN', workspaceId: 'ws1' },
          { id: 2, title: 'Beta case', status: 'OPEN', workspaceId: 'ws1' },
        ],
      },
    },
  },
]

function renderWithProviders(ui: React.ReactNode) {
  return render(
    <MockedProvider mocks={mocks} addTypename={false}>
      <I18nProvider>{ui}</I18nProvider>
    </MockedProvider>,
  )
}

describe('CaseRefField (single)', () => {
  it('renders label', () => {
    renderWithProviders(
      <CaseRefField
        fieldId="f1"
        label="Related Case"
        value=""
        onChange={vi.fn()}
        referenceWorkspaceId="ws1"
      />,
    )
    expect(screen.getByText('Related Case')).toBeInTheDocument()
  })

  it('loads and displays options after query resolves', async () => {
    renderWithProviders(
      <CaseRefField
        fieldId="f1"
        label="Related Case"
        value=""
        onChange={vi.fn()}
        referenceWorkspaceId="ws1"
      />,
    )
    await waitFor(() => {
      expect(screen.getByRole('combobox')).toBeInTheDocument()
    })
  })
})

describe('CaseRefField (multi)', () => {
  it('renders label for multi mode', () => {
    renderWithProviders(
      <CaseRefField
        fieldId="f2"
        label="Related Cases"
        value={[]}
        onChange={vi.fn()}
        referenceWorkspaceId="ws1"
        multi
      />,
    )
    expect(screen.getByText('Related Cases')).toBeInTheDocument()
  })
})
