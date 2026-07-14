import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import { I18nProvider } from '../../i18n'
import FieldDisplay from './FieldDisplay'

function renderWithI18n(ui: React.ReactNode) {
  return render(<I18nProvider>{ui}</I18nProvider>)
}

const markdownField = { id: 'body', name: 'Body', type: 'MARKDOWN' }

describe('FieldDisplay MARKDOWN', () => {
  it('renders the value as Markdown', () => {
    const { container } = renderWithI18n(
      <FieldDisplay field={markdownField} value={'## Head\n\n- one\n- **two**'} />,
    )
    expect(container.querySelector('h2')?.textContent).toBe('Head')
    expect(container.querySelectorAll('li')).toHaveLength(2)
    expect(container.querySelector('strong')?.textContent).toBe('two')
    expect(container.textContent).not.toContain('##')
    expect(container.textContent).not.toContain('**')
  })

  it('shows the em dash placeholder for an empty value', () => {
    renderWithI18n(<FieldDisplay field={markdownField} value="" />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('opens Markdown links in a new tab', () => {
    const { container } = renderWithI18n(
      <FieldDisplay field={markdownField} value="See [docs](https://example.com)." />,
    )
    const link = container.querySelector('a')
    expect(link).not.toBeNull()
    expect(link?.getAttribute('href')).toBe('https://example.com')
    expect(link?.getAttribute('target')).toBe('_blank')
    expect(link?.getAttribute('rel')).toBe('noopener noreferrer')
  })

  it('does not execute raw HTML in the source', () => {
    const { container } = renderWithI18n(
      <FieldDisplay field={markdownField} value={'<script>alert(1)</script>\n\ntext'} />,
    )
    expect(container.querySelector('script')).toBeNull()
  })

  it('neutralizes dangerous link protocols (javascript:/data:/vbscript:)', () => {
    // react-markdown's defaultUrlTransform strips any href whose protocol is
    // not in its safe allowlist (http/https/irc/mailto/xmpp), so a
    // javascript: link cannot produce an executable href.
    const { container } = renderWithI18n(
      <FieldDisplay
        field={markdownField}
        value={'[x](javascript:alert(document.cookie)) [y](data:text/html,<script>1</script>)'}
      />,
    )
    for (const a of Array.from(container.querySelectorAll('a'))) {
      const href = a.getAttribute('href') ?? ''
      expect(href.startsWith('javascript:')).toBe(false)
      expect(href.startsWith('data:')).toBe(false)
      expect(href.startsWith('vbscript:')).toBe(false)
    }
  })
})
