import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import ValueDescTooltip from './ValueDescTooltip'

describe('ValueDescTooltip', () => {
  it('renders children only when description is empty', () => {
    render(
      <ValueDescTooltip name="P1" description="">
        <span data-testid="child">P1</span>
      </ValueDescTooltip>,
    )
    expect(screen.getByTestId('child')).toBeInTheDocument()
    expect(screen.queryByRole('tooltip')).toBeNull()
  })

  it('shows tooltip on mouse enter and hides on mouse leave', () => {
    render(
      <ValueDescTooltip name="P1" description="Production-impacting" testId="t">
        <span data-testid="child">P1</span>
      </ValueDescTooltip>,
    )
    const anchor = screen.getByTestId('t')
    expect(screen.queryByRole('tooltip')).toBeNull()
    fireEvent.mouseEnter(anchor)
    expect(screen.getByRole('tooltip')).toHaveTextContent('Production-impacting')
    expect(screen.getByRole('tooltip')).toHaveTextContent('P1')
    fireEvent.mouseLeave(anchor)
    expect(screen.queryByRole('tooltip')).toBeNull()
  })

  it('shows tooltip on keyboard focus', () => {
    render(
      <ValueDescTooltip name="P1" description="Production-impacting" testId="t">
        <span data-testid="child">P1</span>
      </ValueDescTooltip>,
    )
    const anchor = screen.getByTestId('t')
    fireEvent.focus(anchor)
    expect(screen.getByRole('tooltip')).toBeInTheDocument()
    fireEvent.blur(anchor)
    expect(screen.queryByRole('tooltip')).toBeNull()
  })
})
