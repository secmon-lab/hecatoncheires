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
    ? {
        top: Math.min(rect.bottom + 4, window.innerHeight - 16),
        left: Math.min(rect.left, window.innerWidth - (width || 240) - 8),
        width,
      }
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
