import { useState, type ReactNode } from 'react'
import styles from './FieldHelp.module.css'

interface Props {
  name: string
  description?: string | null
  children: ReactNode
  testId?: string
  /** When true, draw a dotted underline cue on the wrapped value. Default true. */
  decorate?: boolean
}

// Wrap a SELECT / MULTI_SELECT value with a hover/focus tooltip showing the
// option's description. When `description` is empty, renders `children` as-is
// without an extra wrapper element.
export default function ValueDescTooltip({
  name, description, children, testId, decorate = true,
}: Props) {
  const [open, setOpen] = useState(false)
  if (!description) {
    return <>{children}</>
  }
  const className = decorate
    ? `${styles.tooltipAnchor} ${styles.tooltipHint}`
    : styles.tooltipAnchor
  return (
    <span
      className={className}
      onMouseEnter={() => setOpen(true)}
      onMouseLeave={() => setOpen(false)}
      onFocus={() => setOpen(true)}
      onBlur={() => setOpen(false)}
      tabIndex={0}
      data-testid={testId}
    >
      {children}
      {open && (
        <span role="tooltip" className={styles.tooltipBody} data-testid={testId ? `${testId}-body` : undefined}>
          <span className={styles.tooltipName}>{name}</span>
          <span className={styles.tooltipDesc}>{description}</span>
        </span>
      )}
    </span>
  )
}
