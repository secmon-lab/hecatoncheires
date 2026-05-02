import { useEffect, useRef, useState } from 'react'
import InlineFieldFrame from './InlineFieldFrame'
import styles from './Inline.module.css'

interface Props {
  value: number | string | null | undefined
  onSave: (next: number | null) => Promise<void> | void
  ariaLabel: string
  placeholder?: string
  disabled?: boolean
  testId?: string
}

export default function InlineNumber({
  value, onSave, ariaLabel, placeholder, disabled, testId,
}: Props) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(value == null ? '' : String(value))
  const [saving, setSaving] = useState(false)
  const ref = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!editing) setDraft(value == null ? '' : String(value))
  }, [value, editing])

  useEffect(() => {
    if (editing) {
      requestAnimationFrame(() => {
        ref.current?.focus()
        ref.current?.select()
      })
    }
  }, [editing])

  const commit = async () => {
    const trimmed = draft.trim()
    let next: number | null
    if (trimmed === '') {
      next = null
    } else {
      const n = Number(trimmed)
      if (!Number.isFinite(n)) {
        // Invalid: revert.
        setDraft(value == null ? '' : String(value))
        setEditing(false)
        return
      }
      next = n
    }
    const cur = value == null ? null : Number(value)
    if (next === cur) {
      setEditing(false)
      return
    }
    setSaving(true)
    try {
      await onSave(next)
      setEditing(false)
    } catch {
      // stay
    } finally {
      setSaving(false)
    }
  }

  if (editing) {
    return (
      <input
        ref={ref}
        type="number"
        className={`${styles.input} mono`}
        value={draft}
        disabled={saving}
        aria-label={ariaLabel}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={() => void commit()}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault()
            void commit()
          } else if (e.key === 'Escape') {
            e.preventDefault()
            setDraft(value == null ? '' : String(value))
            setEditing(false)
          }
        }}
        data-testid={testId ? `${testId}-input` : undefined}
      />
    )
  }

  const isEmpty = value == null || value === ''
  return (
    <InlineFieldFrame
      ariaLabel={ariaLabel}
      disabled={disabled}
      onActivate={() => !disabled && setEditing(true)}
      testId={testId}
    >
      <span className={isEmpty ? styles.placeholder : 'mono'}>
        {isEmpty ? (placeholder || '—') : String(value)}
      </span>
    </InlineFieldFrame>
  )
}
