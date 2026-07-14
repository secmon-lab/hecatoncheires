import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { I18nProvider } from '../../i18n'
import MarkdownField from './MarkdownField'

function renderWithI18n(ui: React.ReactNode) {
  return render(<I18nProvider>{ui}</I18nProvider>)
}

describe('MarkdownField', () => {
  it('renders the label and marks required fields', () => {
    renderWithI18n(
      <MarkdownField fieldId="body" label="Body" value="" onChange={vi.fn()} required />,
    )
    const label = screen.getByText('Body')
    expect(label).toBeInTheDocument()
    expect(label.querySelector('span')?.textContent).toBe('*')
  })

  it('shows the description when provided', () => {
    renderWithI18n(
      <MarkdownField fieldId="body" label="Body" value="" onChange={vi.fn()} description="Write notes" />,
    )
    expect(screen.getByText('Write notes')).toBeInTheDocument()
  })

  it('shows the error message when provided', () => {
    renderWithI18n(
      <MarkdownField fieldId="body" label="Body" value="" onChange={vi.fn()} error="Required" />,
    )
    expect(screen.getByText('Required')).toBeInTheDocument()
  })

  it('delegates editing to the MarkdownEditor and forwards onChange', () => {
    const onChange = vi.fn()
    renderWithI18n(<MarkdownField fieldId="body" label="Body" value="A" onChange={onChange} />)
    const ta = screen.getByTestId('body-textarea') as HTMLTextAreaElement
    expect(ta.value).toBe('A')
    fireEvent.change(ta, { target: { value: 'AB' } })
    expect(onChange).toHaveBeenCalledWith('AB')
  })
})
