import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { I18nProvider } from '../../i18n'
import InlineCaseSelect from './InlineCaseSelect'
import type { CaseRefItem } from './InlineCaseSelect'

const cases: CaseRefItem[] = [
  { id: 1, title: 'Alpha case', status: 'OPEN', workspaceId: 'ws1' },
  { id: 2, title: 'Beta case', status: 'OPEN', workspaceId: 'ws1' },
  { id: 3, title: 'Gamma case', status: 'CLOSED', workspaceId: 'ws1' },
]

function renderWithI18n(ui: React.ReactNode) {
  return render(<I18nProvider>{ui}</I18nProvider>)
}

describe('InlineCaseSelect', () => {
  it('shows the selected case label in the trigger', () => {
    renderWithI18n(
      <InlineCaseSelect cases={cases} value="2" onSave={vi.fn()} ariaLabel="case ref" testId="cr" />,
    )
    expect(screen.getByTestId('cr')).toHaveTextContent('Beta case (#2)')
  })

  it('shows placeholder when value is null', () => {
    renderWithI18n(
      <InlineCaseSelect cases={cases} value={null} onSave={vi.fn()} ariaLabel="case ref" placeholder="Select a case..." testId="cr" />,
    )
    expect(screen.getByTestId('cr')).toHaveTextContent('Select a case...')
  })

  it('opens popover on click and shows all options', () => {
    renderWithI18n(
      <InlineCaseSelect cases={cases} value={null} onSave={vi.fn()} ariaLabel="case ref" testId="cr" />,
    )
    fireEvent.click(screen.getByTestId('cr'))
    expect(screen.getByTestId('cr-popover')).toBeInTheDocument()
    expect(screen.getByTestId('cr-option-1')).toBeInTheDocument()
    expect(screen.getByTestId('cr-option-2')).toBeInTheDocument()
    expect(screen.getByTestId('cr-option-3')).toBeInTheDocument()
  })

  it('calls onSave with id string when option picked', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderWithI18n(
      <InlineCaseSelect cases={cases} value={null} onSave={onSave} ariaLabel="case ref" testId="cr" />,
    )
    fireEvent.click(screen.getByTestId('cr'))
    fireEvent.click(screen.getByTestId('cr-option-2'))
    await waitFor(() => expect(onSave).toHaveBeenCalledWith('2'))
    expect(screen.queryByTestId('cr-popover')).toBeNull()
  })

  it('does not call onSave when picking the current value', async () => {
    const onSave = vi.fn()
    renderWithI18n(
      <InlineCaseSelect cases={cases} value="1" onSave={onSave} ariaLabel="case ref" testId="cr" />,
    )
    fireEvent.click(screen.getByTestId('cr'))
    fireEvent.click(screen.getByTestId('cr-option-1'))
    await waitFor(() => expect(screen.queryByTestId('cr-popover')).toBeNull())
    expect(onSave).not.toHaveBeenCalled()
  })

  it('filters options by search query (client-side)', () => {
    renderWithI18n(
      <InlineCaseSelect cases={cases} value={null} onSave={vi.fn()} ariaLabel="case ref" testId="cr" />,
    )
    fireEvent.click(screen.getByTestId('cr'))
    const search = screen.getByTestId('cr-search')
    fireEvent.change(search, { target: { value: 'gamma' } })
    expect(screen.queryByTestId('cr-option-1')).toBeNull()
    expect(screen.queryByTestId('cr-option-2')).toBeNull()
    expect(screen.getByTestId('cr-option-3')).toBeInTheDocument()
  })

  it('calls onSearchChange when search input changes', () => {
    const onSearchChange = vi.fn()
    renderWithI18n(
      <InlineCaseSelect cases={cases} value={null} onSave={vi.fn()} ariaLabel="case ref" testId="cr" onSearchChange={onSearchChange} />,
    )
    fireEvent.click(screen.getByTestId('cr'))
    const search = screen.getByTestId('cr-search')
    fireEvent.change(search, { target: { value: 'al' } })
    expect(onSearchChange).toHaveBeenCalledWith('al')
  })

  it('shows loading message when loading=true', () => {
    renderWithI18n(
      <InlineCaseSelect cases={[]} value={null} onSave={vi.fn()} ariaLabel="case ref" testId="cr" loading />,
    )
    fireEvent.click(screen.getByTestId('cr'))
    expect(screen.getByTestId('cr-popover')).toHaveTextContent('Loading')
  })
})
