import { useTranslation } from '../i18n'
import Button from './Button'

/** One action offered by the bar. The caller supplies the label and the
 *  test id so each tab keeps its own wording and selectors. */
export interface BulkAction {
  key: string
  label: string
  variant: 'primary' | 'danger' | 'secondary' | 'ghost'
  testId: string
  onClick: () => void
}

interface BulkSelectionBarProps {
  selectedCount: number
  /** Rendered left to right, before the always-present Clear button. */
  actions: BulkAction[]
  onClear: () => void
  disabled?: boolean
  /** Replaces the "N selected" label while a bulk action is running.
   *  Caller composes this off the hook state to avoid mounting a separate
   *  progress row (and the layout shift that comes with it). */
  progressLabel?: string
}

// BulkSelectionBar is an inline cluster — count label + the caller's actions +
// Clear — that the caller drops between the status tabs and the search input.
// The component renders nothing when no rows are selected and no progress is
// in flight, so the row height stays constant whether selection is active
// or not (no layout shift).
//
// The actions are a prop rather than fixed buttons because three tabs share
// this bar with different verbs (Drafts submits and deletes, Closed archives,
// Archived restores) while the count label, the progress label, the disabled
// handling and the no-layout-shift rule are identical for all of them.
export default function BulkSelectionBar({
  selectedCount,
  actions,
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
      {actions.map((action) => (
        <Button
          key={action.key}
          size="sm"
          variant={action.variant}
          disabled={disabled || selectedCount <= 0}
          onClick={action.onClick}
          data-testid={action.testId}
        >
          {action.label}
        </Button>
      ))}
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
