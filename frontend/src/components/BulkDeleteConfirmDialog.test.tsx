import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'

import { I18nProvider } from '../i18n'
import BulkDeleteConfirmDialog from './BulkDeleteConfirmDialog'

const renderDialog = (
  props: Partial<React.ComponentProps<typeof BulkDeleteConfirmDialog>> = {},
) => {
  const onConfirm = vi.fn()
  const onCancel = vi.fn()
  render(
    <I18nProvider>
      <BulkDeleteConfirmDialog
        open
        count={3}
        previewTitles={['draft-A', 'draft-B', 'draft-C']}
        onConfirm={onConfirm}
        onCancel={onCancel}
        {...props}
      />
    </I18nProvider>,
  )
  return { onConfirm, onCancel }
}

describe('BulkDeleteConfirmDialog', () => {
  it('renders nothing when open=false', () => {
    renderDialog({ open: false })
    expect(screen.queryByTestId('bulk-delete-confirm-body')).toBeNull()
  })

  it('renders the body and preview titles', () => {
    renderDialog()
    expect(screen.getByTestId('bulk-delete-confirm-body')).toBeInTheDocument()
    const list = screen.getByTestId('bulk-delete-preview-list')
    expect(list).toHaveTextContent('draft-A')
    expect(list).toHaveTextContent('draft-B')
    expect(list).toHaveTextContent('draft-C')
  })

  it('caps the preview list at 5 and shows a +N more entry', () => {
    renderDialog({
      count: 7,
      previewTitles: ['t1', 't2', 't3', 't4', 't5', 't6', 't7'],
    })
    const list = screen.getByTestId('bulk-delete-preview-list')
    expect(list).toHaveTextContent('t5')
    expect(list).not.toHaveTextContent('t6')
    expect(screen.getByTestId('bulk-delete-preview-more')).toHaveTextContent('+2 more')
  })

  it('fires onConfirm when the danger button is clicked', () => {
    const { onConfirm } = renderDialog()
    fireEvent.click(screen.getByTestId('bulk-delete-confirm-confirm'))
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })

  it('fires onCancel when the cancel button is clicked', () => {
    const { onCancel } = renderDialog()
    fireEvent.click(screen.getByTestId('bulk-delete-confirm-cancel'))
    expect(onCancel).toHaveBeenCalledTimes(1)
  })

  it('disables both buttons when disabled=true', () => {
    const { onConfirm, onCancel } = renderDialog({ disabled: true })
    const confirm = screen.getByTestId('bulk-delete-confirm-confirm') as HTMLButtonElement
    const cancel = screen.getByTestId('bulk-delete-confirm-cancel') as HTMLButtonElement
    expect(confirm.disabled).toBe(true)
    expect(cancel.disabled).toBe(true)
    fireEvent.click(confirm)
    fireEvent.click(cancel)
    expect(onConfirm).not.toHaveBeenCalled()
    expect(onCancel).not.toHaveBeenCalled()
  })
})
