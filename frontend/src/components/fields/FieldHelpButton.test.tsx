import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import FieldHelpButton from './FieldHelpButton'

describe('FieldHelpButton', () => {
  it('renders ? glyph and starts collapsed', () => {
    render(<FieldHelpButton ariaLabel="show help" expanded={false} onToggle={vi.fn()} testId="b" />)
    const btn = screen.getByTestId('b')
    expect(btn).toHaveTextContent('?')
    expect(btn).toHaveAttribute('aria-expanded', 'false')
    expect(btn).toHaveAttribute('aria-label', 'show help')
  })

  it('calls onToggle when clicked', () => {
    const onToggle = vi.fn()
    render(<FieldHelpButton ariaLabel="show help" expanded={false} onToggle={onToggle} testId="b" />)
    fireEvent.click(screen.getByTestId('b'))
    expect(onToggle).toHaveBeenCalledTimes(1)
  })

  it('stops click propagation so the parent onClick is not invoked', () => {
    const parentClick = vi.fn()
    const onToggle = vi.fn()
    render(
      <div onClick={parentClick} data-testid="parent">
        <FieldHelpButton ariaLabel="show help" expanded={false} onToggle={onToggle} testId="b" />
      </div>,
    )
    fireEvent.click(screen.getByTestId('b'))
    expect(onToggle).toHaveBeenCalledTimes(1)
    expect(parentClick).not.toHaveBeenCalled()
  })

  it('reflects expanded state via aria-expanded', () => {
    render(<FieldHelpButton ariaLabel="show help" expanded={true} onToggle={vi.fn()} testId="b" />)
    expect(screen.getByTestId('b')).toHaveAttribute('aria-expanded', 'true')
  })
})
