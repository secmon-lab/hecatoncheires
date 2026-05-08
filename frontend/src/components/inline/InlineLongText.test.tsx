import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { I18nProvider } from '../../i18n'
import InlineLongText from './InlineLongText'

function renderWithI18n(ui: React.ReactNode) {
  return render(<I18nProvider>{ui}</I18nProvider>)
}

describe('InlineLongText', () => {
  it('renders the value as plain text by default', () => {
    renderWithI18n(
      <InlineLongText value="hello\nworld" onSave={vi.fn()} ariaLabel="desc" testId="d" />,
    )
    expect(screen.getByTestId('d')).toBeInTheDocument()
    expect(screen.queryByTestId('d-input')).toBeNull()
  })

  it('shows placeholder when empty', () => {
    renderWithI18n(
      <InlineLongText value="" onSave={vi.fn()} ariaLabel="desc" placeholder="Add desc" testId="d" />,
    )
    expect(screen.getByTestId('d')).toHaveTextContent('Add desc')
  })

  it('enters edit mode on click and saves via Save button', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderWithI18n(<InlineLongText value="A" onSave={onSave} ariaLabel="desc" testId="d" />)

    fireEvent.click(screen.getByTestId('d'))
    const ta = await screen.findByTestId('d-input') as HTMLTextAreaElement
    fireEvent.change(ta, { target: { value: 'B' } })
    fireEvent.click(screen.getByTestId('d-save'))

    await waitFor(() => expect(onSave).toHaveBeenCalledWith('B'))
  })

  it('cancels editing on Cancel button without saving', async () => {
    const onSave = vi.fn()
    renderWithI18n(<InlineLongText value="A" onSave={onSave} ariaLabel="desc" testId="d" />)

    fireEvent.click(screen.getByTestId('d'))
    const ta = await screen.findByTestId('d-input') as HTMLTextAreaElement
    fireEvent.change(ta, { target: { value: 'B' } })
    fireEvent.click(screen.getByTestId('d-cancel'))

    await waitFor(() => expect(screen.queryByTestId('d-input')).toBeNull())
    expect(onSave).not.toHaveBeenCalled()
  })

  it('saves on Cmd/Ctrl+Enter', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderWithI18n(<InlineLongText value="A" onSave={onSave} ariaLabel="desc" testId="d" />)

    fireEvent.click(screen.getByTestId('d'))
    const ta = await screen.findByTestId('d-input') as HTMLTextAreaElement
    fireEvent.change(ta, { target: { value: 'B' } })
    fireEvent.keyDown(ta, { key: 'Enter', metaKey: true })

    await waitFor(() => expect(onSave).toHaveBeenCalledWith('B'))
  })

  it('cancels editing on Escape', async () => {
    const onSave = vi.fn()
    renderWithI18n(<InlineLongText value="A" onSave={onSave} ariaLabel="desc" testId="d" />)

    fireEvent.click(screen.getByTestId('d'))
    const ta = await screen.findByTestId('d-input') as HTMLTextAreaElement
    fireEvent.change(ta, { target: { value: 'B' } })
    fireEvent.keyDown(ta, { key: 'Escape' })

    await waitFor(() => expect(screen.queryByTestId('d-input')).toBeNull())
    expect(onSave).not.toHaveBeenCalled()
  })

  it('does NOT save on Cmd+Enter while IME is composing', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderWithI18n(<InlineLongText value="あ" onSave={onSave} ariaLabel="desc" testId="d" />)

    fireEvent.click(screen.getByTestId('d'))
    const ta = await screen.findByTestId('d-input') as HTMLTextAreaElement
    fireEvent.change(ta, { target: { value: 'あい' } })
    fireEvent.keyDown(ta, { key: 'Enter', metaKey: true, isComposing: true })

    expect(onSave).not.toHaveBeenCalled()
    expect(screen.queryByTestId('d-input')).not.toBeNull()
  })

  it('does NOT enter edit mode when display div receives Enter while IME is composing', () => {
    const onSave = vi.fn()
    renderWithI18n(<InlineLongText value="A" onSave={onSave} ariaLabel="desc" testId="d" />)

    const display = screen.getByTestId('d')
    fireEvent.keyDown(display, { key: 'Enter', isComposing: true })

    // Still in display mode — IME composition must not trigger edit activation.
    expect(screen.queryByTestId('d-input')).toBeNull()
  })
})
