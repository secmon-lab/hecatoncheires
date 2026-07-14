import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { I18nProvider } from '../../i18n'
import MarkdownEditor from './MarkdownEditor'

function renderWithI18n(ui: React.ReactNode) {
  return render(<I18nProvider>{ui}</I18nProvider>)
}

describe('MarkdownEditor', () => {
  it('shows the textarea with the raw source on the Write tab', () => {
    renderWithI18n(<MarkdownEditor value="## Title" onChange={vi.fn()} testId="md" />)
    const ta = screen.getByTestId('md-textarea') as HTMLTextAreaElement
    expect(ta.value).toBe('## Title')
    expect(screen.queryByTestId('md-preview')).toBeNull()
  })

  it('fires onChange as the user types', () => {
    const onChange = vi.fn()
    renderWithI18n(<MarkdownEditor value="" onChange={onChange} testId="md" />)
    fireEvent.change(screen.getByTestId('md-textarea'), { target: { value: '# New' } })
    expect(onChange).toHaveBeenCalledWith('# New')
  })

  it('renders Markdown on the Preview tab', () => {
    renderWithI18n(<MarkdownEditor value={'## Head\n\n- one\n- **two**'} onChange={vi.fn()} testId="md" />)
    fireEvent.click(screen.getByTestId('md-tab-preview'))
    const preview = screen.getByTestId('md-preview')
    expect(preview.querySelector('h2')?.textContent).toBe('Head')
    expect(preview.querySelectorAll('li')).toHaveLength(2)
    expect(preview.querySelector('strong')?.textContent).toBe('two')
    expect(preview.textContent).not.toContain('##')
    expect(preview.textContent).not.toContain('**')
  })

  it('shows the empty-preview placeholder when the value is blank', () => {
    renderWithI18n(<MarkdownEditor value="   " onChange={vi.fn()} testId="md" />)
    fireEvent.click(screen.getByTestId('md-tab-preview'))
    const preview = screen.getByTestId('md-preview')
    expect(preview.querySelector('h1, h2, h3, p')).toBeNull()
    expect(preview.textContent?.trim()).not.toBe('')
  })

  it('does not execute raw HTML in the source (no rehype-raw)', () => {
    renderWithI18n(
      <MarkdownEditor value={'<script>alert(1)</script>\n\ntext'} onChange={vi.fn()} testId="md" />,
    )
    fireEvent.click(screen.getByTestId('md-tab-preview'))
    const preview = screen.getByTestId('md-preview')
    // The script tag must not become a live DOM element.
    expect(preview.querySelector('script')).toBeNull()
  })

  it('carries ARIA tab roles and aria-selected state', () => {
    renderWithI18n(<MarkdownEditor value="hello" onChange={vi.fn()} testId="md" />)
    const writeTab = screen.getByTestId('md-tab-write')
    const previewTab = screen.getByTestId('md-tab-preview')
    expect(writeTab.getAttribute('role')).toBe('tab')
    expect(previewTab.getAttribute('role')).toBe('tab')
    expect(writeTab.getAttribute('aria-selected')).toBe('true')
    expect(previewTab.getAttribute('aria-selected')).toBe('false')

    fireEvent.click(previewTab)
    expect(writeTab.getAttribute('aria-selected')).toBe('false')
    expect(previewTab.getAttribute('aria-selected')).toBe('true')
  })
})
