import ReactMarkdown, { type Components } from 'react-markdown'
import styles from './MarkdownContent.module.css'

// Open Markdown links in a new tab so navigating an embedded link never
// discards surrounding editor / modal state. `noopener` / `noreferrer` are
// required whenever target is `_blank`. No `rehype-raw` is configured, so raw
// HTML in the source is rendered as text (not executed) — this is the shared
// XSS-safe render path for Markdown field values.
const components: Components = {
  a: ({ node: _node, ...rest }) => <a {...rest} target="_blank" rel="noopener noreferrer" />,
}

interface Props {
  source: string
}

// Read-only Markdown renderer shared by the Markdown field's display, form
// preview, and Case detail view modal. Callers guard the empty state.
export default function MarkdownContent({ source }: Props) {
  return (
    <div className={styles.body}>
      <ReactMarkdown components={components}>{source}</ReactMarkdown>
    </div>
  )
}
