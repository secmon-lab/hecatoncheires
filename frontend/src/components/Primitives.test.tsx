import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { AssigneeNamesStack, BoardStatusBadge } from './Primitives'
import { actionStatusColor } from '../utils/actionStatusStyle'

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

  it('renders avatar + display name (carried by realName) for a single assignee', () => {
    render(
      <AssigneeNamesStack
        users={[u('U1', 'alice', 'Alice Smith')]}
        testId="row-assignees"
      />,
    )
    const stack = screen.getByTestId('row-assignees')
    expect(stack).toBeInTheDocument()
    expect(stack).toHaveTextContent('Alice Smith')
    expect(stack).not.toHaveTextContent('alice')
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
    expect(stack.textContent).toContain('Alice Smith')
    expect(stack.textContent).toContain('Bob Jones')
    expect(stack.textContent).toContain('Charlie Brown')
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
    expect(stack.textContent).toContain('Alice Smith')
    expect(stack.textContent).toContain('Bob Jones')
    expect(stack.textContent).toContain('Charlie Brown')
    expect(stack.textContent).not.toContain('Dave Wilson')
    expect(stack.textContent).not.toContain('Eve Adams')
    expect(stack.textContent).toContain('+2')
  })

  it('falls back to the Slack handle when realName is blank', () => {
    render(
      <AssigneeNamesStack
        users={[u('U1', 'alice', '')]}
        testId="row-assignees"
      />,
    )
    expect(screen.getByTestId('row-assignees')).toHaveTextContent('alice')
  })
})

describe('BoardStatusBadge', () => {
  it('renders the configured status name', () => {
    render(<BoardStatusBadge label="In Review" color="blocked" />)
    expect(screen.getByTestId('board-status-badge')).toHaveTextContent('In Review')
  })

  it('paints the pip with the configured colour', () => {
    render(<BoardStatusBadge label="Done" color="#1a7f4b" />)
    const pip = screen.getByTestId('board-status-badge').querySelector('.pip') as HTMLElement
    expect(pip).not.toBeNull()
    // jsdom normalises a hex literal to its rgb() form.
    expect(pip.style.background).toBe('rgb(26, 127, 75)')
  })

  it('falls back to the idle colour when the status carries none', () => {
    render(<BoardStatusBadge label="ghost" />)
    const pip = screen.getByTestId('board-status-badge').querySelector('.pip') as HTMLElement
    expect(pip.style.background).toBe(actionStatusColor(null))
  })
})
