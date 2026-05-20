import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import CaseFilterSelect from './CaseFilterSelect'

const baseProps = {
  caseLabel: 'Case',
  allLabel: 'All Case',
  searchPlaceholder: 'Search…',
  emptyLabel: 'No matches',
  triggerAriaLabel: 'Filter actions by Case',
  searchAriaLabel: 'Search Case',
  testId: 'case-filter',
}

const cases = [
  { id: 1, title: 'GitHub leak' },
  { id: 2, title: 'Server outage' },
  { id: 3, title: 'Email phishing' },
]

describe('CaseFilterSelect', () => {
  it('renders the "all" label when nothing is selected', () => {
    render(
      <CaseFilterSelect
        {...baseProps}
        cases={cases}
        selectedCaseId={null}
        onSelect={() => {}}
      />,
    )
    const trigger = screen.getByTestId('case-filter-trigger')
    expect(trigger).toHaveTextContent('All Case')
  })

  it('renders the selected case as "#id title"', () => {
    render(
      <CaseFilterSelect
        {...baseProps}
        cases={cases}
        selectedCaseId={2}
        onSelect={() => {}}
      />,
    )
    // The trigger now renders #id and title as separate spans inside the chip;
    // assert each piece individually rather than the joined string.
    const chip = screen.getByTestId('case-filter-chip')
    expect(chip).toHaveTextContent('#2')
    expect(chip).toHaveTextContent('Server outage')
  })

  it('renders a chip with the full title-attribute fallback when a case is selected', () => {
    render(
      <CaseFilterSelect
        {...baseProps}
        cases={cases}
        selectedCaseId={2}
        onSelect={() => {}}
      />,
    )
    const trigger = screen.getByTestId('case-filter-trigger')
    const chip = screen.getByTestId('case-filter-chip')
    expect(chip).toBeInTheDocument()
    expect(chip).toHaveTextContent('#2')
    expect(chip).toHaveTextContent('Server outage')
    // Full text is mirrored to the trigger's title attribute so ellipsised
    // long titles remain inspectable via hover / focus.
    expect(trigger).toHaveAttribute('title', '#2 Server outage')
  })

  it('does not render a chip when nothing is selected and omits the title attribute', () => {
    render(
      <CaseFilterSelect
        {...baseProps}
        cases={cases}
        selectedCaseId={null}
        onSelect={() => {}}
      />,
    )
    expect(screen.queryByTestId('case-filter-chip')).toBeNull()
    expect(screen.getByTestId('case-filter-trigger')).not.toHaveAttribute('title')
  })

  it('opens the popover, lists the cases, and invokes onSelect for a clicked item', () => {
    const onSelect = vi.fn()
    render(
      <CaseFilterSelect
        {...baseProps}
        cases={cases}
        selectedCaseId={null}
        onSelect={onSelect}
      />,
    )
    fireEvent.click(screen.getByTestId('case-filter-trigger'))
    // All-item plus 3 cases.
    expect(screen.getByTestId('case-filter-item-all')).toBeInTheDocument()
    fireEvent.click(screen.getByTestId('case-filter-item-3'))
    expect(onSelect).toHaveBeenCalledWith(3)
  })

  it('emits null when the "all" item is clicked', () => {
    const onSelect = vi.fn()
    render(
      <CaseFilterSelect
        {...baseProps}
        cases={cases}
        selectedCaseId={1}
        onSelect={onSelect}
      />,
    )
    fireEvent.click(screen.getByTestId('case-filter-trigger'))
    fireEvent.click(screen.getByTestId('case-filter-item-all'))
    expect(onSelect).toHaveBeenCalledWith(null)
  })

  it('renders only "#id" on the trigger when the selected case has an empty title', () => {
    render(
      <CaseFilterSelect
        {...baseProps}
        cases={[]}
        selectedCaseId={42}
        onSelect={() => {}}
        extraOption={{ id: 42, title: '' }}
      />,
    )
    // No trailing space, no duplicated "#id #id".
    expect(screen.getByTestId('case-filter-trigger')).toHaveTextContent('Case#42')
  })

  it('shows an extraOption that is not in the main list', () => {
    const onSelect = vi.fn()
    render(
      <CaseFilterSelect
        {...baseProps}
        cases={cases}
        selectedCaseId={99}
        onSelect={onSelect}
        extraOption={{ id: 99, title: 'Closed case' }}
      />,
    )
    fireEvent.click(screen.getByTestId('case-filter-trigger'))
    expect(screen.getByTestId('case-filter-item-99')).toHaveTextContent('Closed case')
  })

  it('filters items by title substring', () => {
    render(
      <CaseFilterSelect
        {...baseProps}
        cases={cases}
        selectedCaseId={null}
        onSelect={() => {}}
      />,
    )
    fireEvent.click(screen.getByTestId('case-filter-trigger'))
    const input = screen.getByTestId('case-filter-search') as HTMLInputElement
    fireEvent.change(input, { target: { value: 'phish' } })
    expect(screen.queryByTestId('case-filter-item-1')).toBeNull()
    expect(screen.queryByTestId('case-filter-item-2')).toBeNull()
    expect(screen.getByTestId('case-filter-item-3')).toBeInTheDocument()
  })

  it('filters items by id string', () => {
    render(
      <CaseFilterSelect
        {...baseProps}
        cases={cases}
        selectedCaseId={null}
        onSelect={() => {}}
      />,
    )
    fireEvent.click(screen.getByTestId('case-filter-trigger'))
    const input = screen.getByTestId('case-filter-search') as HTMLInputElement
    fireEvent.change(input, { target: { value: '#2' } })
    expect(screen.queryByTestId('case-filter-item-1')).toBeNull()
    expect(screen.getByTestId('case-filter-item-2')).toBeInTheDocument()
  })

  it('shows the empty label when nothing matches', () => {
    render(
      <CaseFilterSelect
        {...baseProps}
        cases={cases}
        selectedCaseId={null}
        onSelect={() => {}}
      />,
    )
    fireEvent.click(screen.getByTestId('case-filter-trigger'))
    const input = screen.getByTestId('case-filter-search') as HTMLInputElement
    fireEvent.change(input, { target: { value: 'zzz' } })
    expect(screen.getByTestId('case-filter-empty')).toHaveTextContent('No matches')
  })

  it('does not select on Enter while the IME is composing', () => {
    const onSelect = vi.fn()
    render(
      <CaseFilterSelect
        {...baseProps}
        cases={cases}
        selectedCaseId={null}
        onSelect={onSelect}
      />,
    )
    fireEvent.click(screen.getByTestId('case-filter-trigger'))
    const input = screen.getByTestId('case-filter-search') as HTMLInputElement
    fireEvent.keyDown(input, { key: 'Enter', isComposing: true })
    expect(onSelect).not.toHaveBeenCalled()
  })

  it('selects the active item on Enter after composition ends', () => {
    const onSelect = vi.fn()
    render(
      <CaseFilterSelect
        {...baseProps}
        cases={cases}
        selectedCaseId={null}
        onSelect={onSelect}
      />,
    )
    fireEvent.click(screen.getByTestId('case-filter-trigger'))
    const input = screen.getByTestId('case-filter-search') as HTMLInputElement
    // Filter to a single match so the activeIndex points at it deterministically.
    fireEvent.change(input, { target: { value: 'Server' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onSelect).toHaveBeenCalledWith(2)
  })
})
