import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import DateField from './DateField'

afterEach(cleanup)
describe('DateField', () => {
  it.each(['2026-10-01', '2026-10-01T00:00:00Z', '2026-10-01T00:00:00+09:00'])('displays the stored calendar day from %s', (value) => {
    render(<DateField fieldId="due" label="Due" value={value} onChange={vi.fn()} />)
    expect(screen.getByLabelText('Due')).toHaveValue('2026-10-01')
  })

  it('emits the server RFC3339 representation and allows clearing', () => {
    const onChange = vi.fn()
    render(<DateField fieldId="due" label="Due" value="2026-10-01T00:00:00Z" onChange={onChange} />)
    fireEvent.change(screen.getByLabelText('Due'), { target: { value: '2026-10-02' } })
    expect(onChange).toHaveBeenLastCalledWith('2026-10-02T00:00:00Z')
    fireEvent.change(screen.getByLabelText('Due'), { target: { value: '' } })
    expect(onChange).toHaveBeenLastCalledWith('')
  })
})
