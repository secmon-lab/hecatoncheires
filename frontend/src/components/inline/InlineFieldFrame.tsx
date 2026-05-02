import { forwardRef, type ReactNode, type KeyboardEvent, type MouseEvent } from 'react'
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

// Shared frame for selectable / single-line editable inline fields.
const InlineFieldFrame = forwardRef<HTMLDivElement, Props>(function InlineFieldFrame(
  { children, onActivate, ariaLabel, disabled, block, text, testId, className },
  ref,
) {
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
      ref={ref}
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
})

export default InlineFieldFrame
