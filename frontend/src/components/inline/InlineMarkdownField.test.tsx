import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { I18nProvider } from '../../i18n'
import InlineMarkdownField from './InlineMarkdownField'

function renderWithI18n(ui: React.ReactNode) {
  return render(<I18nProvider>{ui}</I18nProvider>)
}

describe('InlineMarkdownField', () => {
  it('shows the raw source in the sidebar cell without rendering it', () => {
    renderWithI18n(
      <InlineMarkdownField label="Body" value={'## Heading'} onSave={vi.fn()} testId="f" />,
    )
    const cell = screen.getByTestId('f')
    expect(cell.textContent).toContain('## Heading')
    expect(cell.querySelector('h2')).toBeNull()
  })

  it('shows the placeholder when empty', () => {
    renderWithI18n(
      <InlineMarkdownField label="Body" value="" onSave={vi.fn()} placeholder="Add notes" testId="f" />,
    )
    expect(screen.getByTestId('f')).toHaveTextContent('Add notes')
  })

  it('opens a modal that renders the Markdown when the cell is clicked', () => {
    renderWithI18n(
      <InlineMarkdownField label="Body" value={'## Heading\n\n- one'} onSave={vi.fn()} testId="f" />,
    )
    fireEvent.click(screen.getByTestId('f'))
    // Modal view mode renders the Markdown (heading + list), not raw markers.
    // (The modal title is also an h2, so match by accessible name.)
    const heading = screen.getByRole('heading', { level: 2, name: 'Heading' })
    expect(heading.textContent).toBe('Heading')
    expect(screen.queryByTestId('f-md-textarea')).toBeNull()
  })

  it('switches to the editor when Edit is clicked and shows the raw source', () => {
    renderWithI18n(
      <InlineMarkdownField label="Body" value={'## Heading'} onSave={vi.fn()} testId="f" />,
    )
    fireEvent.click(screen.getByTestId('f'))
    fireEvent.click(screen.getByTestId('f-edit'))
    const ta = screen.getByTestId('f-md-textarea') as HTMLTextAreaElement
    expect(ta.value).toBe('## Heading')
  })

  it('saves the edited draft and returns to view mode', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderWithI18n(<InlineMarkdownField label="Body" value="A" onSave={onSave} testId="f" />)

    fireEvent.click(screen.getByTestId('f'))
    fireEvent.click(screen.getByTestId('f-edit'))
    fireEvent.change(screen.getByTestId('f-md-textarea'), { target: { value: 'B' } })
    fireEvent.click(screen.getByTestId('f-save'))

    await waitFor(() => expect(onSave).toHaveBeenCalledWith('B'))
    // Back in view mode: editor gone.
    await waitFor(() => expect(screen.queryByTestId('f-md-textarea')).toBeNull())
  })

  it('keeps editing and the draft when the save fails', async () => {
    const onSave = vi.fn().mockRejectedValue(new Error('boom'))
    renderWithI18n(<InlineMarkdownField label="Body" value="A" onSave={onSave} testId="f" />)

    fireEvent.click(screen.getByTestId('f'))
    fireEvent.click(screen.getByTestId('f-edit'))
    fireEvent.change(screen.getByTestId('f-md-textarea'), { target: { value: 'B' } })
    fireEvent.click(screen.getByTestId('f-save'))

    await waitFor(() => expect(onSave).toHaveBeenCalledWith('B'))
    // Still editing, draft preserved for retry.
    const ta = screen.getByTestId('f-md-textarea') as HTMLTextAreaElement
    expect(ta.value).toBe('B')
  })

  it('discards the draft on Cancel', () => {
    const onSave = vi.fn()
    renderWithI18n(<InlineMarkdownField label="Body" value="A" onSave={onSave} testId="f" />)

    fireEvent.click(screen.getByTestId('f'))
    fireEvent.click(screen.getByTestId('f-edit'))
    fireEvent.change(screen.getByTestId('f-md-textarea'), { target: { value: 'B' } })
    fireEvent.click(screen.getByTestId('f-cancel'))

    expect(onSave).not.toHaveBeenCalled()
    expect(screen.queryByTestId('f-md-textarea')).toBeNull()
  })

  it('saves on Cmd/Ctrl+Enter while editing', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderWithI18n(<InlineMarkdownField label="Body" value="A" onSave={onSave} testId="f" />)

    fireEvent.click(screen.getByTestId('f'))
    fireEvent.click(screen.getByTestId('f-edit'))
    fireEvent.change(screen.getByTestId('f-md-textarea'), { target: { value: 'B' } })
    fireEvent.keyDown(screen.getByTestId('f-editor'), { key: 'Enter', metaKey: true })

    await waitFor(() => expect(onSave).toHaveBeenCalledWith('B'))
  })

  it('does NOT save on Cmd+Enter while IME is composing', () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderWithI18n(<InlineMarkdownField label="Body" value="あ" onSave={onSave} testId="f" />)

    fireEvent.click(screen.getByTestId('f'))
    fireEvent.click(screen.getByTestId('f-edit'))
    fireEvent.change(screen.getByTestId('f-md-textarea'), { target: { value: 'あい' } })
    fireEvent.keyDown(screen.getByTestId('f-editor'), { key: 'Enter', metaKey: true, isComposing: true })

    expect(onSave).not.toHaveBeenCalled()
    expect(screen.queryByTestId('f-md-textarea')).not.toBeNull()
  })

  it('Escape leaves edit mode first, then closes the modal', async () => {
    const onSave = vi.fn()
    renderWithI18n(<InlineMarkdownField label="Body" value="A" onSave={onSave} testId="f" />)

    fireEvent.click(screen.getByTestId('f'))
    fireEvent.click(screen.getByTestId('f-edit'))
    fireEvent.change(screen.getByTestId('f-md-textarea'), { target: { value: 'B' } })

    // First Escape: leave edit mode (draft discarded), modal stays open.
    fireEvent.keyDown(document, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByTestId('f-md-textarea')).toBeNull())
    expect(screen.getByTestId('f-edit')).toBeInTheDocument()
    expect(onSave).not.toHaveBeenCalled()

    // Second Escape: close the modal.
    fireEvent.keyDown(document, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByTestId('f-edit')).toBeNull())
  })

  it('does not offer an Edit button when disabled', () => {
    renderWithI18n(<InlineMarkdownField label="Body" value="A" onSave={vi.fn()} disabled testId="f" />)
    fireEvent.click(screen.getByTestId('f'))
    expect(screen.queryByTestId('f-edit')).toBeNull()
  })
})
