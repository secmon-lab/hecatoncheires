import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'

import { I18nProvider } from '../i18n'
import BulkSelectionBar, { type BulkAction } from './BulkSelectionBar'

const renderBar = (props: Partial<React.ComponentProps<typeof BulkSelectionBar>> = {}) => {
  const onSubmit = vi.fn()
  const onDelete = vi.fn()
  const onClear = vi.fn()
  const actions: BulkAction[] = [
    { key: 'submit', label: 'Submit selected', variant: 'primary', testId: 'bulk-submit-button', onClick: onSubmit },
    { key: 'delete', label: 'Delete selected', variant: 'danger', testId: 'bulk-delete-button', onClick: onDelete },
  ]
  render(
    <I18nProvider>
      <BulkSelectionBar
        selectedCount={3}
        actions={actions}
        onClear={onClear}
        {...props}
      />
    </I18nProvider>,
  )
  return { onSubmit, onDelete, onClear }
}

describe('BulkSelectionBar', () => {
  it('renders when selectedCount > 0', () => {
    renderBar({ selectedCount: 2 })
    expect(screen.getByTestId('bulk-selection-bar')).toBeInTheDocument()
    expect(screen.getByTestId('bulk-selected-count')).toHaveTextContent('2')
  })

  it('renders nothing when selectedCount is 0 and no progress is active', () => {
    renderBar({ selectedCount: 0 })
    expect(screen.queryByTestId('bulk-selection-bar')).toBeNull()
  })

  it('renders the progress label and disables actions when a bulk run is in flight', () => {
    renderBar({ selectedCount: 0, progressLabel: 'Processing 3 / 5…' })
    expect(screen.getByTestId('bulk-selected-count')).toHaveTextContent('Processing 3 / 5…')
    expect((screen.getByTestId('bulk-submit-button') as HTMLButtonElement).disabled).toBe(true)
    expect((screen.getByTestId('bulk-delete-button') as HTMLButtonElement).disabled).toBe(true)
    expect((screen.getByTestId('bulk-clear-button') as HTMLButtonElement).disabled).toBe(true)
  })

  it('fires onSubmit when the Submit button is clicked', () => {
    const { onSubmit } = renderBar()
    fireEvent.click(screen.getByTestId('bulk-submit-button'))
    expect(onSubmit).toHaveBeenCalledTimes(1)
  })

  it('fires onDelete when the Delete button is clicked', () => {
    const { onDelete } = renderBar()
    fireEvent.click(screen.getByTestId('bulk-delete-button'))
    expect(onDelete).toHaveBeenCalledTimes(1)
  })

  it('fires onClear when the Clear button is clicked', () => {
    const { onClear } = renderBar()
    fireEvent.click(screen.getByTestId('bulk-clear-button'))
    expect(onClear).toHaveBeenCalledTimes(1)
  })

  it('disables all action buttons when disabled', () => {
    const { onSubmit, onDelete } = renderBar({ disabled: true })
    const submit = screen.getByTestId('bulk-submit-button') as HTMLButtonElement
    const del = screen.getByTestId('bulk-delete-button') as HTMLButtonElement
    expect(submit.disabled).toBe(true)
    expect(del.disabled).toBe(true)
    fireEvent.click(submit)
    fireEvent.click(del)
    expect(onSubmit).not.toHaveBeenCalled()
    expect(onDelete).not.toHaveBeenCalled()
  })

  // The bar is shared by the Drafts, Closed and Archived tabs, which offer
  // different verbs. Only what the caller passes may be rendered — a leftover
  // hard-coded button would show a Delete on the Archived tab.
  it('renders exactly the actions it is given, in order, plus Clear', () => {
    const onArchive = vi.fn()
    const onClear = vi.fn()
    render(
      <I18nProvider>
        <BulkSelectionBar
          selectedCount={2}
          actions={[
            {
              key: 'archive',
              label: 'Archive selected',
              variant: 'primary',
              testId: 'bulk-archive-button',
              onClick: onArchive,
            },
          ]}
          onClear={onClear}
        />
      </I18nProvider>,
    )

    const bar = screen.getByTestId('bulk-selection-bar')
    const buttons = Array.from(bar.querySelectorAll('button'))
    expect(buttons).toHaveLength(2)
    expect(buttons[0]).toHaveAttribute('data-testid', 'bulk-archive-button')
    expect(buttons[0]).toHaveTextContent('Archive selected')
    expect(buttons[1]).toHaveAttribute('data-testid', 'bulk-clear-button')

    expect(screen.queryByTestId('bulk-submit-button')).toBeNull()
    expect(screen.queryByTestId('bulk-delete-button')).toBeNull()

    fireEvent.click(buttons[0])
    expect(onArchive).toHaveBeenCalledTimes(1)
  })

  it('renders only Clear when the caller passes no actions', () => {
    const onClear = vi.fn()
    render(
      <I18nProvider>
        <BulkSelectionBar selectedCount={1} actions={[]} onClear={onClear} />
      </I18nProvider>,
    )
    const buttons = Array.from(screen.getByTestId('bulk-selection-bar').querySelectorAll('button'))
    expect(buttons).toHaveLength(1)
    expect(buttons[0]).toHaveAttribute('data-testid', 'bulk-clear-button')
  })
})
