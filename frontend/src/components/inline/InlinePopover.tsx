import { useEffect, useRef, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import styles from './Inline.module.css'

interface Props {
  anchor: HTMLElement | null
  open: boolean
  onClose: () => void
  children: ReactNode
  width?: number
  testId?: string
}

// Lightweight popover anchored to a trigger element. Closes on outside
// click and Escape key.
export default function InlinePopover({ anchor, open, onClose, children, width, testId }: Props) {
  const popRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onDocClick = (e: MouseEvent) => {
      const t = e.target as Node | null
      if (!t) return
      if (popRef.current?.contains(t)) return
      if (anchor?.contains(t)) return
      onClose()
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        onClose()
      }
    }
    document.addEventListener('mousedown', onDocClick)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDocClick)
      document.removeEventListener('keydown', onKey)
    }
  }, [open, anchor, onClose])

  if (!open) return null

  const rect = anchor?.getBoundingClientRect()
  const style: React.CSSProperties = rect
    ? (() => {
        const popMax = 320
        const margin = 8
        const spaceBelow = window.innerHeight - rect.bottom - margin
        const spaceAbove = rect.top - margin
        const flipAbove = spaceBelow < Math.min(popMax, 160) && spaceAbove > spaceBelow
        const top = flipAbove
          ? Math.max(margin, rect.top - 4 - Math.min(popMax, spaceAbove))
          : Math.min(rect.bottom + 4, window.innerHeight - margin - Math.min(popMax, spaceBelow))
        return {
          top,
          left: Math.min(rect.left, window.innerWidth - (width || 240) - margin),
          maxHeight: Math.max(120, flipAbove ? spaceAbove : spaceBelow),
          width,
        }
      })()
    : { top: 0, left: 0, width }

  return createPortal(
    <div
      ref={popRef}
      role="dialog"
      className={styles.popover}
      style={style}
      data-testid={testId}
    >
      {children}
    </div>,
    document.body,
  )
}
