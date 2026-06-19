import ReactMarkdown, { type Components } from 'react-markdown'
import styles from './KnowledgeMarkdownView.module.css'

const components: Components = {
  a: ({ node: _node, ...rest }) => <a {...rest} target="_blank" rel="noopener noreferrer" />,
}

interface Props {
  source: string
  placeholder?: string
}

export default function KnowledgeMarkdownView({ source, placeholder = '—' }: Props) {
  if (!source.trim()) {
    return <div className={styles.placeholder}>{placeholder}</div>
  }
  return (
    <div className={styles.body}>
      <ReactMarkdown components={components}>{source}</ReactMarkdown>
    </div>
  )
}
