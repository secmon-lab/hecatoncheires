import { useEffect, useRef, useState } from 'react'
import InlineFieldFrame from './InlineFieldFrame'
import styles from './Inline.module.css'

interface Props {
  value: string
  onSave: (next: string) => Promise<void> | void
  placeholder?: string
  ariaLabel: string
  disabled?: boolean
  /** Visual variant: title-sized large input. */
  variant?: 'default' | 'title'
  /** Allow blank value to be saved (default: keep original on empty). */
  allowEmpty?: boolean
  testId?: string
  /** Called when entering / leaving edit mode (for parent UI sync). */
  onEditingChange?: (editing: boolean) => void
}

// Single-line inline-editable text: click → input, Enter / blur → save,
// Escape → discard.
export default function InlineText({
  value,
  onSave,
  placeholder,
  ariaLabel,
  disabled,
  variant = 'default',
  allowEmpty = false,
  testId,
  onEditingChange,
}: Props) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(value)
  const [saving, setSaving] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const skipBlurSave = useRef(false)

  useEffect(() => {
    if (!editing) setDraft(value)
  }, [value, editing])

  useEffect(() => {
    onEditingChange?.(editing)
    if (editing) {
      // Focus + select on next tick to ensure the input is mounted.
      requestAnimationFrame(() => {
        inputRef.current?.focus()
        inputRef.current?.select()
      })
    }
  }, [editing, onEditingChange])

  const enterEdit = () => {
    if (disabled) return
    setDraft(value)
    setEditing(true)
  }

  const commit = async () => {
    const next = draft.trim()
    if (!allowEmpty && next === '') {
      setEditing(false)
      setDraft(value)
      return
    }
    if (next === value) {
      setEditing(false)
      return
    }
    setSaving(true)
    try {
      await onSave(next)
      setEditing(false)
    } catch {
      // Stay in edit mode for retry.
    } finally {
      setSaving(false)
    }
  }

  const cancel = () => {
    skipBlurSave.current = true
    setDraft(value)
    setEditing(false)
  }

  if (editing) {
    return (
      <input
        ref={inputRef}
        type="text"
        className={[styles.input, variant === 'title' && styles.inputTitle].filter(Boolean).join(' ')}
        value={draft}
        disabled={saving}
        aria-label={ariaLabel}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault()
            void commit()
          } else if (e.key === 'Escape') {
            e.preventDefault()
            cancel()
          }
        }}
        onBlur={() => {
          if (skipBlurSave.current) {
            skipBlurSave.current = false
            return
          }
          void commit()
        }}
        data-testid={testId ? `${testId}-input` : undefined}
      />
    )
  }

  const isEmpty = value === '' || value == null
  return (
    <InlineFieldFrame
      onActivate={enterEdit}
      ariaLabel={ariaLabel}
      disabled={disabled}
      text
      block
      testId={testId}
    >
      <span className={isEmpty ? styles.placeholder : undefined}>
        {isEmpty ? placeholder || '—' : value}
      </span>
    </InlineFieldFrame>
  )
}
