import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { MockedProvider } from '@apollo/client/testing'
import { I18nProvider } from '../../i18n'
import InlineCustomField from './InlineCustomField'
import { REFERENCEABLE_CASES, CASE_REFS_BY_IDS } from '../../graphql/caseRef'

const referenceableMock = {
  request: {
    query: REFERENCEABLE_CASES,
    variables: { workspaceId: 'ws1', query: undefined, limit: 50 },
  },
  result: {
    data: {
      referenceableCases: [
        { id: 1, title: 'Alpha case', status: 'OPEN', workspaceId: 'ws1' },
      ],
    },
  },
}

// Infinite-delay mock: CASE_REFS_BY_IDS never resolves → resolvedLoading stays true
const resolveId99LoadingMock = {
  request: {
    query: CASE_REFS_BY_IDS,
    variables: { workspaceId: 'ws1', ids: [99] },
  },
  delay: Infinity,
  result: { data: { caseRefsByIds: [] } },
}

// Resolves immediately with empty — simulates resolution completed with no result
const resolveId99EmptyMock = {
  request: {
    query: CASE_REFS_BY_IDS,
    variables: { workspaceId: 'ws1', ids: [99] },
  },
  result: {
    data: { caseRefsByIds: [] },
  },
}

// Infinite-delay mock for multi values
const resolveIds99And2LoadingMock = {
  request: {
    query: CASE_REFS_BY_IDS,
    variables: { workspaceId: 'ws1', ids: [99, 2] },
  },
  delay: Infinity,
  result: { data: { caseRefsByIds: [] } },
}

const caseRefField = {
  id: 'f1',
  name: 'Related Case',
  type: 'CASE_REF',
  referenceWorkspaceId: 'ws1',
}

const multiCaseRefField = {
  id: 'f2',
  name: 'Related Cases',
  type: 'MULTI_CASE_REF',
  referenceWorkspaceId: 'ws1',
}

function renderWithProviders(ui: React.ReactNode, mocks: any[]) {
  return render(
    <MockedProvider mocks={mocks} addTypename={false}>
      <I18nProvider>{ui}</I18nProvider>
    </MockedProvider>,
  )
}

describe('InlineCustomField — CASE_REF', () => {
  it('shows neutral #id while resolution query is in flight', () => {
    // CASE_REFS_BY_IDS has delay:Infinity so resolvedLoading stays true
    renderWithProviders(
      <InlineCustomField
        field={caseRefField}
        value="99"
        onSave={vi.fn()}
        testId="cr"
      />,
      [referenceableMock, resolveId99LoadingMock],
    )
    expect(screen.getByTestId('cr')).toHaveTextContent('#99')
    expect(screen.getByTestId('cr')).not.toHaveTextContent('Unavailable (#99)')
  })

  it('shows unavailable fallback once resolution is complete with no result', async () => {
    renderWithProviders(
      <InlineCustomField
        field={caseRefField}
        value="99"
        onSave={vi.fn()}
        testId="cr"
      />,
      [referenceableMock, resolveId99EmptyMock],
    )
    await waitFor(() => {
      expect(screen.getByTestId('cr')).toHaveTextContent('Unavailable (#99)')
    })
  })
})

describe('InlineCustomField — supplementary line', () => {
  const textField = { id: 'channel', name: 'Channel', type: 'TEXT' }
  const display = { label: '#general', url: 'https://slack.com/archives/C0123ABCD' }

  it('keeps the value editable and shows the resolved name under it', () => {
    renderWithProviders(
      <InlineCustomField field={textField} value="C0123ABCD" onSave={vi.fn()} testId="ch" display={display} />,
      [],
    )
    expect(screen.getByTestId('ch')).toHaveTextContent('C0123ABCD')
    const link = screen.getByTestId('ch-supplement').querySelector('a')
    expect(link).not.toBeNull()
    expect(link).toHaveTextContent('#general')
    expect(link).toHaveAttribute('href', display.url)
  })

  it('clicking the link does not start editing; clicking the value does', () => {
    renderWithProviders(
      <InlineCustomField field={textField} value="C0123ABCD" onSave={vi.fn()} testId="ch" display={display} />,
      [],
    )
    fireEvent.click(screen.getByTestId('ch-supplement').querySelector('a')!)
    expect(screen.queryByTestId('ch-input')).toBeNull()

    fireEvent.click(screen.getByTestId('ch'))
    expect(screen.getByTestId('ch-input')).toHaveValue('C0123ABCD')
  })

  it('renders no supplementary line without a display', () => {
    renderWithProviders(
      <InlineCustomField field={textField} value="C0123ABCD" onSave={vi.fn()} testId="ch" />,
      [],
    )
    expect(screen.queryByTestId('ch-supplement')).toBeNull()
  })
})

describe('InlineCustomField — MARKDOWN', () => {
  const markdownField = { id: 'md1', name: 'Notes', type: 'MARKDOWN' }

  it('dispatches to InlineMarkdownField: cell shows raw source, click renders it', () => {
    renderWithProviders(
      <InlineCustomField field={markdownField} value={'## Heading'} onSave={vi.fn()} testId="md" />,
      [],
    )
    const cell = screen.getByTestId('md')
    // Sidebar cell shows the raw source, not rendered Markdown.
    expect(cell.textContent).toContain('## Heading')
    expect(cell.querySelector('h2')).toBeNull()

    // Clicking opens the view modal, which renders the Markdown.
    // (The modal title is also an h2, so match by accessible name.)
    fireEvent.click(screen.getByTestId('md'))
    expect(screen.getByRole('heading', { level: 2, name: 'Heading' }).textContent).toBe('Heading')
  })
})

describe('InlineCustomField — MULTI_CASE_REF', () => {
  it('shows neutral #id for each unresolved id while resolution is in flight', () => {
    // CASE_REFS_BY_IDS has delay:Infinity so resolvedLoading stays true
    renderWithProviders(
      <InlineCustomField
        field={multiCaseRefField}
        value={['99', '2']}
        onSave={vi.fn()}
        testId="mcr"
      />,
      [referenceableMock, resolveIds99And2LoadingMock],
    )
    // Both ids are unresolved while loading — expect neutral labels
    expect(screen.getByTestId('mcr')).toHaveTextContent('#99')
    expect(screen.getByTestId('mcr')).toHaveTextContent('#2')
    expect(screen.getByTestId('mcr')).not.toHaveTextContent('Unavailable (#99)')
    expect(screen.getByTestId('mcr')).not.toHaveTextContent('Unavailable (#2)')
  })
})


describe('InlineCustomField — DATE', () => {
  it('displays and edits the stored day and saves RFC3339 rather than date-only text', async () => {
    const onSave = vi.fn()
    renderWithProviders(<InlineCustomField field={{ id: 'due', name: 'Due', type: 'DATE' }} value="2026-10-01T00:00:00+09:00" onSave={onSave} testId="date" />, [])
    expect(screen.getByTestId('date')).toHaveTextContent(new Date('2026-10-01T00:00:00Z').toLocaleDateString(undefined, { timeZone: 'UTC' }))
    fireEvent.click(screen.getByTestId('date'))
    const input = screen.getByTestId('date-input')
    expect(input).toHaveValue('2026-10-01')
    fireEvent.change(input, { target: { value: '2026-10-02' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    await waitFor(() => expect(onSave).toHaveBeenCalledWith('2026-10-02T00:00:00Z'))
    await waitFor(() => expect(screen.queryByTestId('date-input')).toBeNull())
    fireEvent.click(screen.getByTestId('date'))
    fireEvent.change(screen.getByTestId('date-input'), { target: { value: '' } })
    fireEvent.blur(screen.getByTestId('date-input'))
    await waitFor(() => expect(onSave).toHaveBeenLastCalledWith(null))
  })
})
