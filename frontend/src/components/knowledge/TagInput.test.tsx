import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import TagInput from './TagInput'

// Minimal i18n mock — the component only uses placeholderKnowledgeTagInput
vi.mock('../../i18n', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    lang: 'en',
    setLang: vi.fn(),
  }),
}))

describe('TagInput', () => {
  it('adds a tag on Enter when not composing', () => {
    const onChange = vi.fn()
    render(<TagInput tags={[]} onChange={onChange} />)
    const input = screen.getByTestId('tag-input')

    fireEvent.change(input, { target: { value: 'security' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(onChange).toHaveBeenCalledWith(['security'])
  })

  it('does NOT add a tag on Enter while IME is composing (keyCode 229 — legacy Safari signal, also tested by commitOnEnter)', () => {
    const onChange = vi.fn()
    render(<TagInput tags={[]} onChange={onChange} />)
    const input = screen.getByTestId('tag-input')

    fireEvent.change(input, { target: { value: 'セキュリティ' } })
    // keyCode 229 is the legacy Safari IME-composition marker checked by commitOnEnter.
    // jsdom does not propagate nativeEvent.isComposing through fireEvent, so we
    // use the keyCode path — commitOnEnter checks both, ensuring full coverage.
    fireEvent.keyDown(input, { key: 'Enter', keyCode: 229 })

    expect(onChange).not.toHaveBeenCalled()
  })

  it('adds a tag on comma input', () => {
    const onChange = vi.fn()
    render(<TagInput tags={[]} onChange={onChange} />)
    const input = screen.getByTestId('tag-input')

    fireEvent.change(input, { target: { value: 'security,' } })

    expect(onChange).toHaveBeenCalledWith(['security'])
  })

  it('does not add a duplicate tag', () => {
    const onChange = vi.fn()
    render(<TagInput tags={['security']} onChange={onChange} />)
    const input = screen.getByTestId('tag-input')

    fireEvent.change(input, { target: { value: 'security' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    // onChange should not be called for a duplicate
    expect(onChange).not.toHaveBeenCalled()
  })

  it('does not add an empty or whitespace-only tag', () => {
    const onChange = vi.fn()
    render(<TagInput tags={[]} onChange={onChange} />)
    const input = screen.getByTestId('tag-input')

    fireEvent.change(input, { target: { value: '   ' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(onChange).not.toHaveBeenCalled()
  })

  it('communicates zero-tags state (renders no chips when tags is empty)', () => {
    const { container } = render(<TagInput tags={[]} onChange={vi.fn()} />)
    const chips = container.querySelectorAll('.chip')
    expect(chips.length).toBe(0)
  })

  it('renders chips for each provided tag and removes on click', () => {
    const onChange = vi.fn()
    render(<TagInput tags={['alpha', 'beta']} onChange={onChange} />)

    expect(screen.getByText('alpha')).toBeInTheDocument()
    expect(screen.getByText('beta')).toBeInTheDocument()

    const removeAlpha = screen.getByLabelText('Remove alpha')
    fireEvent.click(removeAlpha)

    expect(onChange).toHaveBeenCalledWith(['beta'])
  })

  it('applies error styling when error prop is true', () => {
    const { container } = render(<TagInput tags={[]} onChange={vi.fn()} error={true} />)
    const root = container.firstChild as HTMLElement
    expect(root.className).toMatch(/error/)
  })
})
