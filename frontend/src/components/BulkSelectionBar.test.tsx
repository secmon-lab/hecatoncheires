import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'

import { I18nProvider } from '../i18n'
import BulkSelectionBar from './BulkSelectionBar'

const renderBar = (props: Partial<React.ComponentProps<typeof BulkSelectionBar>> = {}) => {
  const onSubmit = vi.fn()
  const onDelete = vi.fn()
  const onClear = vi.fn()
  render(
    <I18nProvider>
      <BulkSelectionBar
        selectedCount={3}
        onSubmit={onSubmit}
        onDelete={onDelete}
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
})
