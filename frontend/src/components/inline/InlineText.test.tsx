import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { I18nProvider } from '../../i18n'
import InlineText from './InlineText'

function renderWithI18n(ui: React.ReactNode) {
  return render(<I18nProvider>{ui}</I18nProvider>)
}

describe('InlineText', () => {
  it('renders the value as plain text by default', () => {
    renderWithI18n(
      <InlineText value="Hello" onSave={vi.fn()} ariaLabel="title" testId="t" />,
    )
    expect(screen.getByTestId('t')).toHaveTextContent('Hello')
    expect(screen.queryByTestId('t-input')).toBeNull()
  })

  it('shows placeholder when empty', () => {
    renderWithI18n(
      <InlineText value="" onSave={vi.fn()} ariaLabel="title" placeholder="Add title" testId="t" />,
    )
    expect(screen.getByTestId('t')).toHaveTextContent('Add title')
  })

  it('enters edit mode on click and saves on Enter', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderWithI18n(<InlineText value="Hello" onSave={onSave} ariaLabel="title" testId="t" />)

    fireEvent.click(screen.getByTestId('t'))
    const input = await screen.findByTestId('t-input') as HTMLInputElement
    fireEvent.change(input, { target: { value: 'World' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => expect(onSave).toHaveBeenCalledWith('World'))
  })

  it('discards changes on Escape', async () => {
    const onSave = vi.fn()
    renderWithI18n(<InlineText value="Hello" onSave={onSave} ariaLabel="title" testId="t" />)

    fireEvent.click(screen.getByTestId('t'))
    const input = await screen.findByTestId('t-input') as HTMLInputElement
    fireEvent.change(input, { target: { value: 'World' } })
    fireEvent.keyDown(input, { key: 'Escape' })

    await waitFor(() => expect(screen.queryByTestId('t-input')).toBeNull())
    expect(onSave).not.toHaveBeenCalled()
  })

  it('does not call onSave when value is unchanged', async () => {
    const onSave = vi.fn()
    renderWithI18n(<InlineText value="Hello" onSave={onSave} ariaLabel="title" testId="t" />)

    fireEvent.click(screen.getByTestId('t'))
    const input = await screen.findByTestId('t-input') as HTMLInputElement
    fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => expect(screen.queryByTestId('t-input')).toBeNull())
    expect(onSave).not.toHaveBeenCalled()
  })

  it('saves on blur', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderWithI18n(<InlineText value="A" onSave={onSave} ariaLabel="title" testId="t" />)

    fireEvent.click(screen.getByTestId('t'))
    const input = await screen.findByTestId('t-input') as HTMLInputElement
    fireEvent.change(input, { target: { value: 'B' } })
    fireEvent.blur(input)

    await waitFor(() => expect(onSave).toHaveBeenCalledWith('B'))
  })

  it('respects disabled', () => {
    const onSave = vi.fn()
    renderWithI18n(
      <InlineText value="Hello" onSave={onSave} ariaLabel="title" disabled testId="t" />,
    )
    fireEvent.click(screen.getByTestId('t'))
    expect(screen.queryByTestId('t-input')).toBeNull()
  })

  it('does NOT save on Enter while IME is composing (isComposing=true)', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderWithI18n(<InlineText value="あ" onSave={onSave} ariaLabel="title" testId="t" />)

    fireEvent.click(screen.getByTestId('t'))
    const input = await screen.findByTestId('t-input') as HTMLInputElement
    fireEvent.change(input, { target: { value: 'あい' } })
    // The Enter that confirms the IME conversion — must NOT trigger save.
    fireEvent.keyDown(input, { key: 'Enter', isComposing: true })

    expect(onSave).not.toHaveBeenCalled()
    // Still in edit mode — user can keep typing.
    expect(screen.queryByTestId('t-input')).not.toBeNull()
  })

  it('does NOT save on Enter when keyCode=229 (legacy Safari IME)', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderWithI18n(<InlineText value="A" onSave={onSave} ariaLabel="title" testId="t" />)

    fireEvent.click(screen.getByTestId('t'))
    const input = await screen.findByTestId('t-input') as HTMLInputElement
    fireEvent.change(input, { target: { value: 'B' } })
    fireEvent.keyDown(input, { key: 'Enter', keyCode: 229 })

    expect(onSave).not.toHaveBeenCalled()
  })

  it('saves on Enter after IME composition ends', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    renderWithI18n(<InlineText value="あ" onSave={onSave} ariaLabel="title" testId="t" />)

    fireEvent.click(screen.getByTestId('t'))
    const input = await screen.findByTestId('t-input') as HTMLInputElement
    fireEvent.change(input, { target: { value: '愛' } })
    // First Enter confirms IME — no save.
    fireEvent.keyDown(input, { key: 'Enter', isComposing: true })
    expect(onSave).not.toHaveBeenCalled()
    // Second Enter (post-composition) — should save.
    fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => expect(onSave).toHaveBeenCalledWith('愛'))
  })
})
