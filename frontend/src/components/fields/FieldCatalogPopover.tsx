import { useMemo } from 'react'
import InlinePopover from '../inline/InlinePopover'
import { useTranslation } from '../../i18n'
import styles from './FieldHelp.module.css'

export interface FieldOptionDef {
  id: string
  name: string
  description?: string | null
}

export interface FieldDefinitionForHelp {
  id: string
  name: string
  type: string
  description?: string | null
  options?: FieldOptionDef[] | null
}

interface Props {
  field: FieldDefinitionForHelp
  /** Current value. SELECT: option id (string|null). MULTI_SELECT: string[]. Other: ignored. */
  value: unknown
  anchor: HTMLElement | null
  open: boolean
  onClose: () => void
  testId?: string
}

function selectedIds(field: FieldDefinitionForHelp, value: unknown): Set<string> {
  if (field.type === 'SELECT') {
    return typeof value === 'string' && value ? new Set([value]) : new Set()
  }
  if (field.type === 'MULTI_SELECT') {
    return Array.isArray(value) ? new Set(value as string[]) : new Set()
  }
  return new Set()
}

export default function FieldCatalogPopover({
  field, value, anchor, open, onClose, testId,
}: Props) {
  const { t } = useTranslation()

  const selected = useMemo(() => selectedIds(field, value), [field, value])
  const options = field.options ?? []
  const showOptions = (field.type === 'SELECT' || field.type === 'MULTI_SELECT') && options.length > 0
  const typeLabel = field.type === 'MULTI_SELECT'
    ? t('fieldHelpTypeMultiSelect')
    : field.type === 'SELECT'
      ? t('fieldHelpTypeSelect')
      : ''

  return (
    <InlinePopover
      anchor={anchor}
      open={open}
      onClose={onClose}
      width={340}
      testId={testId}
    >
      <div className={styles.catalog}>
        <div className={styles.catalogHeader}>
          {typeLabel && <span className={styles.catalogTypeLabel}>{typeLabel}</span>}
          <span className={styles.catalogTitle}>{field.name}</span>
          <button
            type="button"
            className={styles.catalogClose}
            onClick={onClose}
            aria-label={t('fieldHelpClose')}
            data-testid={testId ? `${testId}-close` : undefined}
          >
            ×
          </button>
        </div>

        {field.description && (
          <div className={styles.catalogDesc} data-testid={testId ? `${testId}-desc` : undefined}>
            {field.description}
          </div>
        )}

        {showOptions && (
          <div className={styles.catalogBody}>
            {options.map((opt) => {
              const isSelected = selected.has(opt.id)
              return (
                <div
                  key={opt.id}
                  className={`${styles.catalogOption} ${isSelected ? styles.catalogOptionSelected : ''}`}
                  data-testid={testId ? `${testId}-option-${opt.id}` : undefined}
                >
                  <div className={styles.catalogOptionHead}>
                    <span className={styles.catalogOptionName}>{opt.name}</span>
                    {isSelected && (
                      <span className={styles.catalogOptionBadge}>
                        {t('fieldHelpSelectedBadge')}
                      </span>
                    )}
                  </div>
                  {opt.description && (
                    <div className={styles.catalogOptionDesc}>{opt.description}</div>
                  )}
                </div>
              )
            })}
          </div>
        )}

        {showOptions && (
          <div className={styles.catalogFooter}>
            <span>{t('fieldHelpOptionCount', { count: options.length })}</span>
            <span>{t('fieldHelpFooterHint')}</span>
          </div>
        )}
      </div>
    </InlinePopover>
  )
}
