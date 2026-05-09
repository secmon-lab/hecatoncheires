import { forwardRef, type MouseEvent } from 'react'
import styles from './FieldHelp.module.css'

interface Props {
  ariaLabel: string
  expanded: boolean
  onToggle: () => void
  testId?: string
}

const FieldHelpButton = forwardRef<HTMLButtonElement, Props>(function FieldHelpButton(
  { ariaLabel, expanded, onToggle, testId },
  ref,
) {
  const handleClick = (e: MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation()
    onToggle()
  }
  return (
    <button
      ref={ref}
      type="button"
      onClick={handleClick}
      aria-label={ariaLabel}
      aria-expanded={expanded}
      className={`${styles.helpBtn} ${expanded ? styles.helpBtnExpanded : ''}`}
      data-testid={testId}
    >
      ?
    </button>
  )
})

export default FieldHelpButton
