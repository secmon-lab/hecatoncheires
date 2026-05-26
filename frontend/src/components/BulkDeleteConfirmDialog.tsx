import { useTranslation } from '../i18n'
import Button from './Button'
import Modal from './Modal'

interface BulkDeleteConfirmDialogProps {
  open: boolean
  count: number
  /** First few draft titles to show as a preview list. Caller decides how
   *  many to pass; rendering tops it at five and shows a "+N more" tail. */
  previewTitles: string[]
  onConfirm: () => void
  onCancel: () => void
  disabled?: boolean
}

const PREVIEW_LIMIT = 5

export default function BulkDeleteConfirmDialog({
  open,
  count,
  previewTitles,
  onConfirm,
  onCancel,
  disabled = false,
}: BulkDeleteConfirmDialogProps) {
  const { t } = useTranslation()

  const shown = previewTitles.slice(0, PREVIEW_LIMIT)
  const more = previewTitles.length - shown.length

  return (
    <Modal
      open={open}
      onClose={onCancel}
      title={t('bulkDeleteConfirmTitle', { count })}
      footer={
        <div className="row" style={{ gap: 'var(--spacing-sm)', justifyContent: 'flex-end' }}>
          <Button
            variant="ghost"
            onClick={onCancel}
            disabled={disabled}
            data-testid="bulk-delete-confirm-cancel"
          >
            {t('bulkDeleteConfirmCancel')}
          </Button>
          <Button
            variant="danger"
            onClick={onConfirm}
            disabled={disabled}
            data-testid="bulk-delete-confirm-confirm"
          >
            {t('bulkDeleteConfirmConfirm')}
          </Button>
        </div>
      }
    >
      <div data-testid="bulk-delete-confirm-body">
        <p style={{ marginTop: 0, color: 'var(--text-body)', fontSize: 13 }}>
          {t('bulkDeleteConfirmBody')}
        </p>
        {shown.length > 0 && (
          <ul
            data-testid="bulk-delete-preview-list"
            style={{
              margin: 0,
              padding: 'var(--spacing-sm) var(--spacing-md)',
              background: 'var(--bg-subtle)',
              borderRadius: 4,
              fontSize: 12.5,
              color: 'var(--text-body)',
            }}
          >
            {shown.map((title, idx) => (
              <li key={idx} style={{ listStyle: 'disc', marginLeft: 'var(--spacing-md)' }}>
                {title || <span className="soft">—</span>}
              </li>
            ))}
            {more > 0 && (
              <li
                data-testid="bulk-delete-preview-more"
                style={{ listStyle: 'none', color: 'var(--text-muted)', marginTop: 'var(--spacing-xs)' }}
              >
                +{more} more
              </li>
            )}
          </ul>
        )}
      </div>
    </Modal>
  )
}
