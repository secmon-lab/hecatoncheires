import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { MockedProvider } from '@apollo/client/testing'
import { I18nProvider } from '../../i18n'
import CaseRefField from './CaseRefField'
import { REFERENCEABLE_CASES, CASE_REFS_BY_IDS } from '../../graphql/caseRef'

// Picker results only contain cases 1 & 2 (case 99 is "outside top-50")
const referenceableMock = {
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
}

// Resolved by CASE_REFS_BY_IDS for the stored single value "99"
const resolveId99Mock = {
  request: {
    query: CASE_REFS_BY_IDS,
    variables: { workspaceId: 'ws1', ids: [99] },
  },
  result: {
    data: {
      caseRefsByIds: [
        { id: 99, title: 'Old closed case', status: 'CLOSED', workspaceId: 'ws1' },
      ],
    },
  },
}

// CASE_REFS_BY_IDS for stored multi values ["99","2"]
const resolveIds99And2Mock = {
  request: {
    query: CASE_REFS_BY_IDS,
    variables: { workspaceId: 'ws1', ids: [99, 2] },
  },
  result: {
    data: {
      caseRefsByIds: [
        { id: 99, title: 'Old closed case', status: 'CLOSED', workspaceId: 'ws1' },
        { id: 2, title: 'Beta case', status: 'OPEN', workspaceId: 'ws1' },
      ],
    },
  },
}

// CASE_REFS_BY_IDS for an unresolvable id "888" (returns empty)
const resolveId888EmptyMock = {
  request: {
    query: CASE_REFS_BY_IDS,
    variables: { workspaceId: 'ws1', ids: [888] },
  },
  result: {
    data: {
      caseRefsByIds: [],
    },
  },
}

function renderWithProviders(ui: React.ReactNode, mocks: any[]) {
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
      [referenceableMock],
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
      [referenceableMock],
    )
    await waitFor(() => {
      expect(screen.getByRole('combobox')).toBeInTheDocument()
    })
  })

  it('shows resolved title for a stored value not in the picker results', async () => {
    // value="99" is not in the REFERENCEABLE_CASES mock (top-50), but
    // CASE_REFS_BY_IDS resolves it to "Old closed case (#99)".
    renderWithProviders(
      <CaseRefField
        fieldId="f1"
        label="Related Case"
        value="99"
        onChange={vi.fn()}
        referenceWorkspaceId="ws1"
      />,
      [referenceableMock, resolveId99Mock],
    )
    await waitFor(() => {
      expect(screen.getByText('Old closed case (#99)')).toBeInTheDocument()
    })
  })

  it('shows unavailable fallback when stored id cannot be resolved', async () => {
    renderWithProviders(
      <CaseRefField
        fieldId="f1"
        label="Related Case"
        value="888"
        onChange={vi.fn()}
        referenceWorkspaceId="ws1"
      />,
      [referenceableMock, resolveId888EmptyMock],
    )
    await waitFor(() => {
      expect(screen.getByText('Unavailable (#888)')).toBeInTheDocument()
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
      [referenceableMock],
    )
    expect(screen.getByText('Related Cases')).toBeInTheDocument()
  })

  it('shows resolved titles for stored values not in the picker results', async () => {
    // values=["99","2"]: case 99 is outside top-50 picker, case 2 is in picker
    renderWithProviders(
      <CaseRefField
        fieldId="f2"
        label="Related Cases"
        value={['99', '2']}
        onChange={vi.fn()}
        referenceWorkspaceId="ws1"
        multi
      />,
      [referenceableMock, resolveIds99And2Mock],
    )
    await waitFor(() => {
      expect(screen.getByText('Old closed case (#99)')).toBeInTheDocument()
      expect(screen.getByText('Beta case (#2)')).toBeInTheDocument()
    })
  })

  it('shows unavailable fallback for unresolvable id in multi mode', async () => {
    renderWithProviders(
      <CaseRefField
        fieldId="f2"
        label="Related Cases"
        value={['888']}
        onChange={vi.fn()}
        referenceWorkspaceId="ws1"
        multi
      />,
      [referenceableMock, resolveId888EmptyMock],
    )
    await waitFor(() => {
      expect(screen.getByText('Unavailable (#888)')).toBeInTheDocument()
    })
  })
})
