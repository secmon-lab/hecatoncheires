import { useMemo, useRef, useState } from 'react'
import InlineFieldFrame from './InlineFieldFrame'
import InlinePopover from './InlinePopover'
import styles from './Inline.module.css'
import { IconCheck } from '../Icons'
import { useTranslation } from '../../i18n'

export interface CaseRefItem {
  id: number
  title: string
  status: string
  workspaceId: string
}

interface Props {
  cases: CaseRefItem[]
  /** Pre-resolved cases for the current stored value, used for the trigger
   *  label when the value is not present in the picker (cases) list. */
  resolvedCases?: CaseRefItem[]
  /** Whether the CASE_REFS_BY_IDS resolution query is still in flight.
   *  While true, unresolved ids show a neutral "#id" instead of "Unavailable". */
  resolvedLoading?: boolean
  value: string | null
  onSave: (next: string | null) => Promise<void> | void
  ariaLabel: string
  placeholder?: string
  disabled?: boolean
  testId?: string
  loading?: boolean
  onSearchChange?: (query: string) => void
}

function caseLabel(c: CaseRefItem): string {
  return `${c.title} (#${c.id})`
}

export default function InlineCaseSelect({
  cases,
  resolvedCases = [],
  resolvedLoading = false,
  value,
  onSave,
  ariaLabel,
  placeholder,
  disabled,
  testId,
  loading = false,
  onSearchChange,
}: Props) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const anchorRef = useRef<HTMLDivElement>(null)

  // Resolve the selected case for the trigger label. Look first in the picker
  // list, then in the pre-resolved list (covers cases outside the top-50).
  const selectedCase = useMemo(() => {
    if (value == null) return null
    return (
      cases.find((c) => String(c.id) === value) ??
      resolvedCases.find((c) => String(c.id) === value) ??
      null
    )
  }, [cases, resolvedCases, value])

  const filtered = useMemo(() => {
    if (!query) return cases
    const q = query.toLowerCase()
    return cases.filter(
      (c) => c.title.toLowerCase().includes(q) || String(c.id).includes(q),
    )
  }, [cases, query])

  const handleSearch = (q: string) => {
    setQuery(q)
    onSearchChange?.(q)
  }

  const handlePick = async (id: string | null) => {
    setOpen(false)
    setQuery('')
    onSearchChange?.('')
    if (id === value) return
    await onSave(id)
  }

  return (
    <>
      <InlineFieldFrame
        ref={anchorRef}
        ariaLabel={ariaLabel}
        disabled={disabled}
        onActivate={() => setOpen((v) => !v)}
        testId={testId}
        block
      >
        {selectedCase ? (
          <span className={styles.triggerLabel}>{caseLabel(selectedCase)}</span>
        ) : value != null ? (
          // Value is stored but could not be resolved — show neutral id while loading,
          // or the unavailable fallback once resolution has completed with no result.
          <span className={styles.triggerLabel}>{resolvedLoading ? `#${value}` : t('caseRefUnavailable', { id: value })}</span>
        ) : (
          <span className={styles.placeholder}>{placeholder ?? t('placeholderSelectCaseRef')}</span>
        )}
      </InlineFieldFrame>
      <InlinePopover
        anchor={anchorRef.current}
        open={open}
        onClose={() => { setOpen(false); setQuery(''); onSearchChange?.('') }}
        testId={testId ? `${testId}-popover` : undefined}
      >
        <div className={styles.popoverSearch}>
          <input
            autoFocus
            className={styles.popoverSearchInput}
            placeholder={t('placeholderSearch')}
            value={query}
            onChange={(e) => handleSearch(e.target.value)}
            data-testid={testId ? `${testId}-search` : undefined}
          />
        </div>
        {loading ? (
          <div className={styles.optionEmpty}>{t('loading')}</div>
        ) : filtered.length === 0 ? (
          <div className={styles.optionEmpty}>{t('noDataAvailable')}</div>
        ) : (
          filtered.map((c) => {
            const active = String(c.id) === value
            return (
              <button
                key={c.id}
                type="button"
                className={`${styles.option} ${active ? styles.optionActive : ''}`}
                onClick={() => void handlePick(String(c.id))}
                data-testid={testId ? `${testId}-option-${c.id}` : undefined}
              >
                <span className={styles.optionLabel}>{caseLabel(c)}</span>
                {active && <IconCheck size={12} className={styles.optionCheck} />}
              </button>
            )
          })
        )}
      </InlinePopover>
    </>
  )
}
