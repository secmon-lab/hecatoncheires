import { useEffect, useRef, useState } from 'react'
import styles from './Inline.module.css'
import Button from '../Button'
import { useTranslation } from '../../i18n'

interface Props {
  value: string
  onSave: (next: string) => Promise<void> | void
  placeholder?: string
  ariaLabel: string
  disabled?: boolean
  testId?: string
}

// Multi-line inline-editable text. Click to enter edit mode (textarea +
// Save / Cancel buttons). Cmd/Ctrl+Enter saves; Escape cancels.
export default function InlineLongText({
  value,
  onSave,
  placeholder,
  ariaLabel,
  disabled,
  testId,
}: Props) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(value)
  const [saving, setSaving] = useState(false)
  const taRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    if (!editing) setDraft(value)
  }, [value, editing])

  useEffect(() => {
    if (editing) {
      requestAnimationFrame(() => {
        taRef.current?.focus()
        const len = taRef.current?.value.length ?? 0
        taRef.current?.setSelectionRange(len, len)
      })
    }
  }, [editing])

  const commit = async () => {
    if (draft === value) {
      setEditing(false)
      return
    }
    setSaving(true)
    try {
      await onSave(draft)
      setEditing(false)
    } catch {
      // Keep editing for retry.
    } finally {
      setSaving(false)
    }
  }

  const cancel = () => {
    setDraft(value)
    setEditing(false)
  }

  if (editing) {
    return (
      <div data-testid={testId ? `${testId}-editor` : undefined}>
        <textarea
          ref={taRef}
          className={styles.textarea}
          value={draft}
          aria-label={ariaLabel}
          disabled={saving}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
              e.preventDefault()
              void commit()
            } else if (e.key === 'Escape') {
              e.preventDefault()
              cancel()
            }
          }}
          data-testid={testId ? `${testId}-input` : undefined}
        />
        <div className={styles.editFooter}>
          <Button
            variant="ghost"
            size="sm"
            onClick={cancel}
            disabled={saving}
            data-testid={testId ? `${testId}-cancel` : undefined}
          >
            {t('btnCancel')}
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={() => void commit()}
            disabled={saving}
            data-testid={testId ? `${testId}-save` : undefined}
          >
            {saving ? t('btnSaving') : t('btnSave')}
          </Button>
        </div>
      </div>
    )
  }

  const isEmpty = !value
  return (
    <div
      role="button"
      tabIndex={disabled ? -1 : 0}
      aria-label={ariaLabel}
      aria-disabled={disabled || undefined}
      className={`${styles.longTextDisplay} ${disabled ? styles.disabled : ''} ${isEmpty ? styles.placeholder : ''}`}
      onClick={() => {
        if (!disabled) setEditing(true)
      }}
      onKeyDown={(e) => {
        if (disabled) return
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          setEditing(true)
        }
      }}
      data-testid={testId}
    >
      {isEmpty ? placeholder || '—' : value}
    </div>
  )
}
