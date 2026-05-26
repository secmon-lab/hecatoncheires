import { useTranslation } from '../i18n'
import Button from './Button'

interface BulkSelectionBarProps {
  selectedCount: number
  onSubmit: () => void
  onDelete: () => void
  onClear: () => void
  disabled?: boolean
  /** Replaces the "N selected" label while a bulk action is running.
   *  Caller composes this off the hook state to avoid mounting a separate
   *  progress row (and the layout shift that comes with it). */
  progressLabel?: string
}

// BulkSelectionBar is an inline cluster — count label + 3 actions — that
// the caller drops between the status tabs and the search input. The
// component renders nothing when no rows are selected and no progress is
// in flight, so the row height stays constant whether selection is active
// or not (no layout shift).
export default function BulkSelectionBar({
  selectedCount,
  onSubmit,
  onDelete,
  onClear,
  disabled = false,
  progressLabel,
}: BulkSelectionBarProps) {
  const { t } = useTranslation()
  if (selectedCount <= 0 && !progressLabel) return null

  const label = progressLabel ?? t('bulkSelectionBarCount', { count: selectedCount })

  return (
    <div
      data-testid="bulk-selection-bar"
      role="toolbar"
      aria-label={label}
      className="row"
      style={{ gap: 'var(--spacing-sm)', alignItems: 'center' }}
    >
      <span
        data-testid="bulk-selected-count"
        style={{ fontSize: 12.5, fontWeight: 600, color: 'var(--text-heading)' }}
      >
        {label}
      </span>
      <Button
        size="sm"
        variant="primary"
        disabled={disabled || selectedCount <= 0}
        onClick={onSubmit}
        data-testid="bulk-submit-button"
      >
        {t('bulkSelectionBarSubmit')}
      </Button>
      <Button
        size="sm"
        variant="danger"
        disabled={disabled || selectedCount <= 0}
        onClick={onDelete}
        data-testid="bulk-delete-button"
      >
        {t('bulkSelectionBarDelete')}
      </Button>
      <Button
        size="sm"
        variant="ghost"
        disabled={disabled || selectedCount <= 0}
        onClick={onClear}
        data-testid="bulk-clear-button"
      >
        {t('bulkSelectionBarClear')}
      </Button>
    </div>
  )
}
