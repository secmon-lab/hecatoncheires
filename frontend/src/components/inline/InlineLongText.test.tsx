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

  it('renders the value as Markdown when renderMarkdown is set', () => {
    const md = '## Heading\n\n- one\n- **two**\n'
    renderWithI18n(
      <InlineLongText
        value={md}
        onSave={vi.fn()}
        ariaLabel="desc"
        testId="d"
        renderMarkdown
      />,
    )
    const display = screen.getByTestId('d')
    // Heading and list items rendered as proper elements, not raw text.
    expect(display.querySelector('h2')?.textContent).toBe('Heading')
    expect(display.querySelectorAll('li')).toHaveLength(2)
    expect(display.querySelector('strong')?.textContent).toBe('two')
    // No raw markdown markers leak through.
    expect(display.textContent).not.toContain('##')
    expect(display.textContent).not.toContain('**')
  })

  it('still enters edit mode on click when renderMarkdown is set', async () => {
    renderWithI18n(
      <InlineLongText
        value="## Title"
        onSave={vi.fn()}
        ariaLabel="desc"
        testId="d"
        renderMarkdown
      />,
    )
    fireEvent.click(screen.getByTestId('d'))
    // Editor shows the *raw* markdown source, not the rendered HTML.
    const ta = (await screen.findByTestId('d-input')) as HTMLTextAreaElement
    expect(ta.value).toBe('## Title')
  })

  it('shows placeholder (not rendered markdown) when value is empty', () => {
    renderWithI18n(
      <InlineLongText
        value=""
        onSave={vi.fn()}
        ariaLabel="desc"
        placeholder="Add desc"
        testId="d"
        renderMarkdown
      />,
    )
    const display = screen.getByTestId('d')
    expect(display).toHaveTextContent('Add desc')
    expect(display.querySelector('h1, h2, h3, p')).toBeNull()
  })

  it('shows a live Markdown preview alongside the editor in markdown mode', async () => {
    renderWithI18n(
      <InlineLongText
        value="## Old"
        onSave={vi.fn()}
        ariaLabel="desc"
        testId="d"
        renderMarkdown
      />,
    )
    fireEvent.click(screen.getByTestId('d'))
    const ta = (await screen.findByTestId('d-input')) as HTMLTextAreaElement
    // Initial preview reflects the existing value.
    const preview = screen.getByTestId('d-preview')
    expect(preview.querySelector('h2')?.textContent).toBe('Old')

    fireEvent.change(ta, { target: { value: '# New title\n\n- item' } })

    await waitFor(() => {
      expect(preview.querySelector('h1')?.textContent).toBe('New title')
    })
    expect(preview.querySelectorAll('li')).toHaveLength(1)
  })

  it('does NOT render a preview pane in plain-text mode', async () => {
    renderWithI18n(
      <InlineLongText value="hello" onSave={vi.fn()} ariaLabel="desc" testId="d" />,
    )
    fireEvent.click(screen.getByTestId('d'))
    await screen.findByTestId('d-input')
    expect(screen.queryByTestId('d-preview')).toBeNull()
  })

  it('grows the textarea to fit its content in markdown mode', async () => {
    const long = Array.from({ length: 30 }, (_, i) => `line ${i + 1}`).join('\n')
    renderWithI18n(
      <InlineLongText
        value={long}
        onSave={vi.fn()}
        ariaLabel="desc"
        testId="d"
        renderMarkdown
      />,
    )
    fireEvent.click(screen.getByTestId('d'))
    const ta = (await screen.findByTestId('d-input')) as HTMLTextAreaElement
    // jsdom reports scrollHeight as 0, so we can't assert on real pixels.
    // What we can assert: the auto-grow effect set an explicit inline
    // height (instead of leaving it at the CSS default).
    expect(ta.style.height).not.toBe('')
  })
})
