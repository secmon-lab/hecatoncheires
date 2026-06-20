import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { I18nProvider } from '../../i18n'
import InlineMultiCaseSelect from './InlineMultiCaseSelect'
import type { CaseRefItem } from './InlineCaseSelect'

const cases: CaseRefItem[] = [
  { id: 10, title: 'Alpha', status: 'OPEN', workspaceId: 'ws1' },
  { id: 20, title: 'Beta', status: 'OPEN', workspaceId: 'ws1' },
  { id: 30, title: 'Gamma', status: 'CLOSED', workspaceId: 'ws1' },
]

function renderWithI18n(ui: React.ReactNode) {
  return render(<I18nProvider>{ui}</I18nProvider>)
}

describe('InlineMultiCaseSelect', () => {
  it('renders selected case labels in the trigger', () => {
    renderWithI18n(
      <InlineMultiCaseSelect cases={cases} values={['10', '20']} onSave={vi.fn()} ariaLabel="cases" testId="mc" />,
    )
    expect(screen.getByTestId('mc')).toHaveTextContent('Alpha (#10)')
    expect(screen.getByTestId('mc')).toHaveTextContent('Beta (#20)')
  })

  it('shows placeholder when values is empty', () => {
    renderWithI18n(
      <InlineMultiCaseSelect cases={cases} values={[]} onSave={vi.fn()} ariaLabel="cases" placeholder="Pick cases..." testId="mc" />,
    )
    expect(screen.getByTestId('mc')).toHaveTextContent('Pick cases...')
  })

  it('toggles a case on (adds to list)', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderWithI18n(
      <InlineMultiCaseSelect cases={cases} values={['10']} onSave={onSave} ariaLabel="cases" testId="mc" />,
    )
    fireEvent.click(screen.getByTestId('mc'))
    fireEvent.click(screen.getByTestId('mc-option-20'))
    await waitFor(() => expect(onSave).toHaveBeenCalledWith(['10', '20']))
  })

  it('toggles a case off (removes from list)', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderWithI18n(
      <InlineMultiCaseSelect cases={cases} values={['10', '20']} onSave={onSave} ariaLabel="cases" testId="mc" />,
    )
    fireEvent.click(screen.getByTestId('mc'))
    fireEvent.click(screen.getByTestId('mc-option-10'))
    await waitFor(() => expect(onSave).toHaveBeenCalledWith(['20']))
  })

  it('filters options by search query', () => {
    renderWithI18n(
      <InlineMultiCaseSelect cases={cases} values={[]} onSave={vi.fn()} ariaLabel="cases" testId="mc" />,
    )
    fireEvent.click(screen.getByTestId('mc'))
    const search = screen.getByTestId('mc-search')
    fireEvent.change(search, { target: { value: 'beta' } })
    expect(screen.queryByTestId('mc-option-10')).toBeNull()
    expect(screen.getByTestId('mc-option-20')).toBeInTheDocument()
    expect(screen.queryByTestId('mc-option-30')).toBeNull()
  })

  it('shows loading state', () => {
    renderWithI18n(
      <InlineMultiCaseSelect cases={[]} values={[]} onSave={vi.fn()} ariaLabel="cases" testId="mc" loading />,
    )
    fireEvent.click(screen.getByTestId('mc'))
    expect(screen.getByTestId('mc-popover')).toHaveTextContent('Loading')
  })

  it('shows resolved title from resolvedCases when a value is not in the picker list', () => {
    // cases (picker) has ids 10,20,30; stored value "99" is in resolvedCases only
    const resolvedCases: CaseRefItem[] = [
      { id: 99, title: 'Old closed case', status: 'CLOSED', workspaceId: 'ws1' },
    ]
    renderWithI18n(
      <InlineMultiCaseSelect
        cases={cases}
        resolvedCases={resolvedCases}
        values={['10', '99']}
        onSave={vi.fn()}
        ariaLabel="cases"
        testId="mc"
      />,
    )
    expect(screen.getByTestId('mc')).toHaveTextContent('Alpha (#10)')
    expect(screen.getByTestId('mc')).toHaveTextContent('Old closed case (#99)')
  })

  it('shows unavailable fallback for unresolvable id in multi mode', () => {
    renderWithI18n(
      <InlineMultiCaseSelect
        cases={cases}
        resolvedCases={[]}
        values={['10', '888']}
        onSave={vi.fn()}
        ariaLabel="cases"
        testId="mc"
      />,
    )
    expect(screen.getByTestId('mc')).toHaveTextContent('Alpha (#10)')
    expect(screen.getByTestId('mc')).toHaveTextContent('Unavailable (#888)')
  })

  it('shows neutral #id while resolvedLoading=true (not yet resolved)', () => {
    // resolvedLoading=true means CASE_REFS_BY_IDS is still in flight
    renderWithI18n(
      <InlineMultiCaseSelect
        cases={cases}
        resolvedCases={[]}
        resolvedLoading={true}
        values={['10', '888']}
        onSave={vi.fn()}
        ariaLabel="cases"
        testId="mc"
      />,
    )
    // id "10" is in the picker list — shows normally
    expect(screen.getByTestId('mc')).toHaveTextContent('Alpha (#10)')
    // id "888" is unresolvable but still loading — shows neutral "#888"
    expect(screen.getByTestId('mc')).toHaveTextContent('#888')
    expect(screen.getByTestId('mc')).not.toHaveTextContent('Unavailable (#888)')
  })

  it('shows unavailable fallback once resolvedLoading=false and id is not resolved', () => {
    renderWithI18n(
      <InlineMultiCaseSelect
        cases={cases}
        resolvedCases={[]}
        resolvedLoading={false}
        values={['10', '888']}
        onSave={vi.fn()}
        ariaLabel="cases"
        testId="mc"
      />,
    )
    expect(screen.getByTestId('mc')).toHaveTextContent('Alpha (#10)')
    expect(screen.getByTestId('mc')).toHaveTextContent('Unavailable (#888)')
  })
})
