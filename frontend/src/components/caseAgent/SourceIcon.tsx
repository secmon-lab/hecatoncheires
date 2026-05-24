import { IconGitHub, IconNotion, IconSlack } from '../Icons'
import styles from '../../pages/CaseAgent.module.css'

interface Props {
  kind: string
  size?: number
}

// SourceIcon maps the GraphQL SourceType enum (uppercase: SLACK /
// GITHUB / NOTION_DB / NOTION_PAGE) onto a recognisable brand glyph.
// Unknown kinds fall back to a neutral grey "?" chip so the row still
// aligns and operators can tell something is off.
export default function SourceIcon({ kind, size = 14 }: Props) {
  const normalised = (kind || '').toUpperCase()
  const wrapStyle = {
    width: size + 4,
    height: size + 4,
    background: 'var(--bg-elev)',
    color: 'var(--fg-muted)',
    border: '1px solid var(--line)',
    fontSize: size - 4,
  }
  switch (normalised) {
    case 'SLACK':
      return (
        <span className={styles.sourceIcon} style={wrapStyle} aria-hidden>
          <IconSlack size={size} />
        </span>
      )
    case 'GITHUB':
      return (
        <span className={styles.sourceIcon} style={wrapStyle} aria-hidden>
          <IconGitHub size={size} />
        </span>
      )
    case 'NOTION_DB':
    case 'NOTION_PAGE':
      return (
        <span className={styles.sourceIcon} style={wrapStyle} aria-hidden>
          <IconNotion size={size} />
        </span>
      )
    default:
      return (
        <span
          className={styles.sourceIcon}
          style={{
            ...wrapStyle,
            background: 'var(--fg-soft)',
            color: '#fff',
            border: 'none',
            fontWeight: 700,
          }}
          aria-hidden
        >
          ?
        </span>
      )
  }
}
