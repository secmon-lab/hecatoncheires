import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { MemoryRouter, useLocation, useNavigate } from 'react-router'
import { useCaseFieldFilters } from './useCaseFieldFilters'

afterEach(cleanup)
function Harness() {
  const { filters, onChange, onClear } = useCaseFieldFilters()
  const location = useLocation()
  const navigate = useNavigate()
  return <>
    <span data-testid="values">{JSON.stringify([...filters])}</span>
    <span data-testid="query">{location.search}</span>
    <button onClick={() => onChange('category', ['it', 'sales'])}>Change</button>
    <button onClick={onClear}>Clear</button>
    <button onClick={() => navigate('?field.category=sales')}>Link</button>
    <button onClick={() => navigate(-1)}>Back</button>
    <button onClick={() => navigate(1)}>Forward</button>
  </>
}

describe('useCaseFieldFilters', () => {
  it('restores URL values and changes only filter parameters', () => {
    render(<MemoryRouter initialEntries={['/ws/test/cases?status=closed&page=3&field.category=it&custom=keep']}><Harness /></MemoryRouter>)
    expect(screen.getByTestId('values')).toHaveTextContent('[["category",["it"]]]')
    fireEvent.click(screen.getByText('Change'))
    expect(screen.getByTestId('query')).toHaveTextContent('?status=closed&custom=keep&field.category=it&field.category=sales')
    fireEvent.click(screen.getByText('Clear'))
    expect(screen.getByTestId('query')).toHaveTextContent('?status=closed&custom=keep')
  })

  it('follows navigation, Back and Forward without stale component state', () => {
    render(<MemoryRouter initialEntries={['/ws/test/actions?field.category=it']}><Harness /></MemoryRouter>)
    fireEvent.click(screen.getByText('Link'))
    expect(screen.getByTestId('values')).toHaveTextContent('[["category",["sales"]]]')
    fireEvent.click(screen.getByText('Back'))
    expect(screen.getByTestId('values')).toHaveTextContent('[["category",["it"]]]')
    fireEvent.click(screen.getByText('Forward'))
    expect(screen.getByTestId('values')).toHaveTextContent('[["category",["sales"]]]')
  })
})
