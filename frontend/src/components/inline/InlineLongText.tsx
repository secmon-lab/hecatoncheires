import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import styles from './Inline.module.css'
import Button from '../Button'
import { useTranslation } from '../../i18n'
import { commitOnEnter, activateOnEnterOrSpace } from '../../utils/keyboard'

interface Props {
  value: string
  onSave: (next: string) => Promise<void> | void
  placeholder?: string
  ariaLabel: string
  disabled?: boolean
  testId?: string
  // When true, render the (read-only) value as Markdown. The textarea
  // used in edit mode is unchanged — users still author plain Markdown.
  renderMarkdown?: boolean
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
  renderMarkdown,
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

  // For the Markdown-enabled long-text editor, auto-grow the textarea so
  // entering edit mode never collapses a tall description back to the
  // default min-height. (Plain-text mode keeps its original behavior.)
  useLayoutEffect(() => {
    if (!editing || !renderMarkdown) return
    const ta = taRef.current
    if (!ta) return
    ta.style.height = 'auto'
    ta.style.height = `${ta.scrollHeight}px`
  }, [draft, editing, renderMarkdown])

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
    const textareaClass = renderMarkdown
      ? `${styles.textarea} ${styles.textareaTall}`
      : styles.textarea
    const previewIsEmpty = draft.trim() === ''
    return (
      <div data-testid={testId ? `${testId}-editor` : undefined}>
        <div className={renderMarkdown ? styles.editSplit : undefined}>
          <textarea
            ref={taRef}
            className={textareaClass}
            value={draft}
            aria-label={ariaLabel}
            disabled={saving}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={commitOnEnter({
              onCommit: () => void commit(),
              onCancel: cancel,
              requireModifier: true,
            })}
            data-testid={testId ? `${testId}-input` : undefined}
          />
          {renderMarkdown && (
            <div
              className={`${styles.editPreview} ${styles.longTextMarkdown} ${previewIsEmpty ? styles.placeholder : ''}`}
              aria-label={t('labelPreview')}
              data-testid={testId ? `${testId}-preview` : undefined}
            >
              {previewIsEmpty ? (
                t('labelPreviewEmpty')
              ) : (
                <ReactMarkdown>{draft}</ReactMarkdown>
              )}
            </div>
          )}
        </div>
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
  const showMarkdown = renderMarkdown && !isEmpty
  const classes = [
    styles.longTextDisplay,
    disabled ? styles.disabled : '',
    isEmpty ? styles.placeholder : '',
    showMarkdown ? styles.longTextMarkdown : '',
  ]
    .filter(Boolean)
    .join(' ')
  return (
    <div
      role="button"
      tabIndex={disabled ? -1 : 0}
      aria-label={ariaLabel}
      aria-disabled={disabled || undefined}
      className={classes}
      onClick={() => {
        if (!disabled) setEditing(true)
      }}
      onKeyDown={
        disabled
          ? undefined
          : activateOnEnterOrSpace(() => setEditing(true))
      }
      data-testid={testId}
    >
      {isEmpty ? (
        placeholder || '—'
      ) : showMarkdown ? (
        <ReactMarkdown>{value}</ReactMarkdown>
      ) : (
        value
      )}
    </div>
  )
}
