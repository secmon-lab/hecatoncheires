import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { AssigneeNamesStack } from './Primitives'

const u = (id: string, name: string, realName: string) => ({
  id,
  name,
  realName,
  imageUrl: null,
})

describe('AssigneeNamesStack', () => {
  it('renders "—" when there are no assignees', () => {
    const { container } = render(<AssigneeNamesStack users={[]} />)
    expect(container.textContent).toBe('—')
  })

  it('renders avatar + display name for a single assignee', () => {
    render(
      <AssigneeNamesStack
        users={[u('U1', 'alice', 'Alice Smith')]}
        testId="row-assignees"
      />,
    )
    const stack = screen.getByTestId('row-assignees')
    expect(stack).toBeInTheDocument()
    expect(stack).toHaveTextContent('alice')
    expect(stack).not.toHaveTextContent('Alice Smith')
  })

  it('renders all visible assignees vertically up to the max', () => {
    render(
      <AssigneeNamesStack
        users={[
          u('U1', 'alice', 'Alice Smith'),
          u('U2', 'bob', 'Bob Jones'),
          u('U3', 'charlie', 'Charlie Brown'),
        ]}
        testId="row-assignees"
      />,
    )
    const stack = screen.getByTestId('row-assignees')
    expect(stack.textContent).toContain('alice')
    expect(stack.textContent).toContain('bob')
    expect(stack.textContent).toContain('charlie')
    expect(stack.textContent).not.toMatch(/\+\d+/)
  })

  it('shows "+N" when more assignees exist than the max', () => {
    render(
      <AssigneeNamesStack
        users={[
          u('U1', 'alice', 'Alice Smith'),
          u('U2', 'bob', 'Bob Jones'),
          u('U3', 'charlie', 'Charlie Brown'),
          u('U4', 'dave', 'Dave Wilson'),
          u('U5', 'eve', 'Eve Adams'),
        ]}
        max={3}
        testId="row-assignees"
      />,
    )
    const stack = screen.getByTestId('row-assignees')
    expect(stack.textContent).toContain('alice')
    expect(stack.textContent).toContain('bob')
    expect(stack.textContent).toContain('charlie')
    expect(stack.textContent).not.toContain('dave')
    expect(stack.textContent).not.toContain('eve')
    expect(stack.textContent).toContain('+2')
  })

  it('falls back to realName when display name is blank', () => {
    render(
      <AssigneeNamesStack
        users={[u('U1', '', 'Alice Smith')]}
        testId="row-assignees"
      />,
    )
    expect(screen.getByTestId('row-assignees')).toHaveTextContent('Alice Smith')
  })
})
