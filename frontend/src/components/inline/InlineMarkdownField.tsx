import { useState } from 'react'
import Modal from '../Modal'
import Button from '../Button'
import MarkdownContent from '../markdown/MarkdownContent'
import MarkdownEditor from '../markdown/MarkdownEditor'
import { commitOnEnter, activateOnEnterOrSpace } from '../../utils/keyboard'
import { useTranslation } from '../../i18n'
import styles from './Inline.module.css'

interface Props {
  label: string
  value: string
  onSave: (next: string) => Promise<void> | void
  placeholder?: string
  disabled?: boolean
  testId?: string
}

// Case-detail sidebar renderer for a Markdown field. The sidebar cell is too
// narrow to render Markdown, so it shows the raw source clamped to a few lines;
// clicking opens a modal that renders the Markdown read-only, and an Edit
// button switches the modal to a Write/Preview editor. This keeps the long-form
// reading/editing experience out of the cramped sidebar.
export default function InlineMarkdownField({
  label,
  value,
  onSave,
  placeholder = '—',
  disabled,
  testId,
}: Props) {
  const { t } = useTranslation()
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(value)
  const [saving, setSaving] = useState(false)

  const openView = () => {
    setEditing(false)
    setModalOpen(true)
  }

  const startEdit = () => {
    setDraft(value)
    setEditing(true)
  }

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
      // Keep editing so the user can retry; surfacing the error is the
      // caller's mutation-error responsibility.
    } finally {
      setSaving(false)
    }
  }

  // Escape closes in two stages: leave edit mode first (discarding the draft),
  // then close the modal. Modal's own Escape handler calls this unconditionally.
  const handleClose = () => {
    if (editing) {
      setDraft(value)
      setEditing(false)
      return
    }
    setModalOpen(false)
  }

  const isEmpty = value.trim() === ''

  // Cmd/Ctrl+Enter saves while editing. Attached to the modal body container
  // (not the textarea) so it still fires when the Preview tab has unmounted the
  // textarea. Escape is handled by Modal (-> handleClose), so no onCancel here.
  const editKeyHandler = commitOnEnter({
    onCommit: () => void commit(),
    requireModifier: true,
  })

  const footer = editing ? (
    <div className={styles.mdModalFooter}>
      <Button
        variant="ghost"
        size="sm"
        onClick={handleClose}
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
  ) : (
    !disabled && (
      <div className={styles.mdModalFooter}>
        <Button
          variant="primary"
          size="sm"
          onClick={startEdit}
          data-testid={testId ? `${testId}-edit` : undefined}
        >
          {t('btnEdit')}
        </Button>
      </div>
    )
  )

  return (
    <>
      <div
        role="button"
        tabIndex={0}
        className={isEmpty ? `${styles.mdClamp} ${styles.mdClampEmpty}` : styles.mdClamp}
        aria-label={label}
        onClick={openView}
        onKeyDown={activateOnEnterOrSpace(openView)}
        data-testid={testId}
      >
        {isEmpty ? placeholder : value}
      </div>

      <Modal open={modalOpen} onClose={handleClose} title={label} width={640} footer={footer}>
        {editing ? (
          <div onKeyDown={editKeyHandler} data-testid={testId ? `${testId}-editor` : undefined}>
            <MarkdownEditor value={draft} onChange={setDraft} disabled={saving} testId={testId ? `${testId}-md` : undefined} />
          </div>
        ) : isEmpty ? (
          <span className={styles.mdClampEmpty} style={{ fontSize: 13 }}>{placeholder}</span>
        ) : (
          <MarkdownContent source={value} />
        )}
      </Modal>
    </>
  )
}
