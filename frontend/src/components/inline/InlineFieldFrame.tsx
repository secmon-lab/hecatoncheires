import type { ReactNode, KeyboardEvent, MouseEvent } from 'react'
import styles from './Inline.module.css'

interface Props {
  children: ReactNode
  onActivate: () => void
  ariaLabel: string
  disabled?: boolean
  block?: boolean
  text?: boolean
  testId?: string
  className?: string
}

// Shared frame for selectable / single-line editable inline fields. Renders
// a button-like wrapper that becomes the interactive trigger to enter editing.
export default function InlineFieldFrame({
  children,
  onActivate,
  ariaLabel,
  disabled,
  block,
  text,
  testId,
  className,
}: Props) {
  const handleKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (disabled) return
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      onActivate()
    }
  }

  const handleClick = (e: MouseEvent<HTMLDivElement>) => {
    if (disabled) return
    e.preventDefault()
    onActivate()
  }

  const cls = [
    styles.frame,
    block && styles.frameBlock,
    text && styles.frameText,
    disabled && styles.disabled,
    className,
  ]
    .filter(Boolean)
    .join(' ')

  return (
    <div
      role="button"
      tabIndex={disabled ? -1 : 0}
      aria-label={ariaLabel}
      aria-disabled={disabled || undefined}
      className={cls}
      onClick={handleClick}
      onKeyDown={handleKeyDown}
      data-testid={testId}
    >
      {children}
    </div>
  )
}
