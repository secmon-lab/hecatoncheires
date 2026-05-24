import ReactMarkdown, { type Components } from 'react-markdown'
import styles from '../../pages/CaseAgent.module.css'

const components: Components = {
  a: ({ node: _node, ...rest }) => <a {...rest} target="_blank" rel="noopener noreferrer" />,
}

interface Props {
  source: string
}

// Lightweight Markdown view used by the CaseAgent prompt preview. We
// rely on react-markdown rather than the InlineLongText component
// because the page renders the preview separately from the editor.
export default function MarkdownView({ source }: Props) {
  if (!source.trim()) {
    return <div className={styles.promptPlaceholder}>—</div>
  }
  return (
    <div className={styles.promptBody}>
      <ReactMarkdown components={components}>{source}</ReactMarkdown>
    </div>
  )
}
