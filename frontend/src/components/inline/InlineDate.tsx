import { useEffect, useRef, useState } from 'react'
import InlineFieldFrame from './InlineFieldFrame'
import { commitOnEnter } from '../../utils/keyboard'
import styles from './Inline.module.css'

interface Props {
  /** ISO date (YYYY-MM-DD) or full ISO timestamp; only the date portion is used. */
  value: string | null | undefined
  onSave: (next: string | null) => Promise<void> | void
  ariaLabel: string
  placeholder?: string
  disabled?: boolean
  testId?: string
}

function toDateInputValue(v: string | null | undefined): string {
  if (!v) return ''
  // Accept "YYYY-MM-DD" or full ISO; truncate at "T".
  const m = String(v).match(/^(\d{4}-\d{2}-\d{2})/)
  return m ? m[1] : ''
}

function formatDisplay(v: string | null | undefined): string {
  if (!v) return ''
  const iso = toDateInputValue(v)
  if (!iso) return String(v)
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleDateString()
}

export default function InlineDate({
  value, onSave, ariaLabel, placeholder, disabled, testId,
}: Props) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(toDateInputValue(value))
  const [saving, setSaving] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!editing) setDraft(toDateInputValue(value))
  }, [value, editing])

  useEffect(() => {
    if (editing) {
      requestAnimationFrame(() => inputRef.current?.focus())
    }
  }, [editing])

  const commit = async (next: string) => {
    const normalized = next === '' ? null : next
    if (normalized === toDateInputValue(value)) {
      setEditing(false)
      return
    }
    setSaving(true)
    try {
      await onSave(normalized)
      setEditing(false)
    } catch {
      // stay editing
    } finally {
      setSaving(false)
    }
  }

  if (editing) {
    return (
      <input
        ref={inputRef}
        type="date"
        className={styles.input}
        value={draft}
        disabled={saving}
        aria-label={ariaLabel}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={() => void commit(draft)}
        onKeyDown={commitOnEnter({
          onCommit: () => void commit(draft),
          onCancel: () => {
            setDraft(toDateInputValue(value))
            setEditing(false)
          },
        })}
        data-testid={testId ? `${testId}-input` : undefined}
      />
    )
  }

  const display = formatDisplay(value)
  const isEmpty = !display
  return (
    <InlineFieldFrame
      ariaLabel={ariaLabel}
      disabled={disabled}
      onActivate={() => !disabled && setEditing(true)}
      testId={testId}
    >
      <span className={isEmpty ? styles.placeholder : 'mono'}>
        {isEmpty ? (placeholder || '—') : display}
      </span>
    </InlineFieldFrame>
  )
}
