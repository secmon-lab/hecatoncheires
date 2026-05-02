import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { I18nProvider } from '../../i18n'
import InlineSelect from './InlineSelect'

const opts = [
  { value: 'BACKLOG', label: 'Backlog', color: '#999' },
  { value: 'TODO', label: 'Todo', color: '#aaa' },
  { value: 'IN_PROGRESS', label: 'In Progress', color: '#bbb' },
]

function renderWithI18n(ui: React.ReactNode) {
  return render(<I18nProvider>{ui}</I18nProvider>)
}

describe('InlineSelect', () => {
  it('shows the current option label', () => {
    renderWithI18n(
      <InlineSelect value="TODO" options={opts} onSave={vi.fn()} ariaLabel="status" testId="s" />,
    )
    expect(screen.getByTestId('s')).toHaveTextContent('Todo')
  })

  it('opens popover on click and shows all options', () => {
    renderWithI18n(
      <InlineSelect value="TODO" options={opts} onSave={vi.fn()} ariaLabel="status" testId="s" />,
    )
    fireEvent.click(screen.getByTestId('s'))
    expect(screen.getByTestId('s-popover')).toBeInTheDocument()
    expect(screen.getByTestId('s-option-BACKLOG')).toBeInTheDocument()
    expect(screen.getByTestId('s-option-IN_PROGRESS')).toBeInTheDocument()
  })

  it('calls onSave with picked option and closes popover', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderWithI18n(
      <InlineSelect value="TODO" options={opts} onSave={onSave} ariaLabel="status" testId="s" />,
    )
    fireEvent.click(screen.getByTestId('s'))
    fireEvent.click(screen.getByTestId('s-option-IN_PROGRESS'))
    await waitFor(() => expect(onSave).toHaveBeenCalledWith('IN_PROGRESS'))
    expect(screen.queryByTestId('s-popover')).toBeNull()
  })

  it('does not call onSave when picking the current value', () => {
    const onSave = vi.fn()
    renderWithI18n(
      <InlineSelect value="TODO" options={opts} onSave={onSave} ariaLabel="status" testId="s" />,
    )
    fireEvent.click(screen.getByTestId('s'))
    fireEvent.click(screen.getByTestId('s-option-TODO'))
    expect(onSave).not.toHaveBeenCalled()
  })

  it('closes on Escape', () => {
    renderWithI18n(
      <InlineSelect value="TODO" options={opts} onSave={vi.fn()} ariaLabel="status" testId="s" />,
    )
    fireEvent.click(screen.getByTestId('s'))
    expect(screen.getByTestId('s-popover')).toBeInTheDocument()
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByTestId('s-popover')).toBeNull()
  })

  it('shows placeholder when value is null', () => {
    renderWithI18n(
      <InlineSelect value={null} options={opts} onSave={vi.fn()} ariaLabel="status" placeholder="Add status" testId="s" />,
    )
    expect(screen.getByTestId('s')).toHaveTextContent('Add status')
  })
})
