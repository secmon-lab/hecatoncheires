import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import MarkdownContent from './MarkdownContent'

describe('MarkdownContent', () => {
  it('renders a GFM table as a <table> with cell values, not raw pipes', () => {
    const source = [
      '| Item | Verdict |',
      '| --- | --- |',
      '| G2-B1 | INSUFFICIENT_DATA |',
      '| G2-B2 | INSUFFICIENT_DATA |',
    ].join('\n')
    const { container } = render(<MarkdownContent source={source} />)

    const table = container.querySelector('table')
    expect(table).not.toBeNull()

    // Header cells come from the first row.
    const headers = Array.from(table!.querySelectorAll('th')).map((th) => th.textContent)
    expect(headers).toEqual(['Item', 'Verdict'])

    // Body cells carry the data rows.
    const cells = Array.from(table!.querySelectorAll('tbody td')).map((td) => td.textContent)
    expect(cells).toEqual(['G2-B1', 'INSUFFICIENT_DATA', 'G2-B2', 'INSUFFICIENT_DATA'])

    // The pipe-delimited source must not leak through as literal text.
    expect(container.textContent).not.toContain('| Item |')
    expect(container.textContent).not.toContain('---')
  })

  it('renders GFM strikethrough as <del>', () => {
    const { container } = render(<MarkdownContent source={'~~gone~~'} />)
    const del = container.querySelector('del')
    expect(del?.textContent).toBe('gone')
  })

  it('renders a GFM task list with checkboxes', () => {
    const { container } = render(<MarkdownContent source={'- [x] done\n- [ ] todo'} />)
    const checkboxes = container.querySelectorAll('input[type="checkbox"]')
    expect(checkboxes).toHaveLength(2)
    expect((checkboxes[0] as HTMLInputElement).checked).toBe(true)
    expect((checkboxes[1] as HTMLInputElement).checked).toBe(false)
  })

  it('still renders CommonMark (heading, list, bold) without regression', () => {
    const { container } = render(
      <MarkdownContent source={'# Heading\n\n- alpha\n- **bold item**'} />,
    )
    expect(container.querySelector('h1')?.textContent).toBe('Heading')
    expect(container.querySelectorAll('li')).toHaveLength(2)
    expect(container.querySelector('strong')?.textContent).toBe('bold item')
    expect(container.textContent).not.toContain('**')
  })

  it('opens links in a new tab with safe rel attributes', () => {
    const { container } = render(<MarkdownContent source={'[link](https://example.test)'} />)
    const a = container.querySelector('a')
    expect(a?.getAttribute('target')).toBe('_blank')
    expect(a?.getAttribute('rel')).toContain('noopener')
    expect(a?.getAttribute('rel')).toContain('noreferrer')
  })

  it('does not execute raw HTML in the source (XSS-safe: rehype-raw stays disabled)', () => {
    const { container } = render(
      <MarkdownContent source={'before <script>window.__pwned = true</script> after'} />,
    )
    // The <script> must never become a real element in the rendered tree.
    expect(container.querySelector('script')).toBeNull()
  })
})
