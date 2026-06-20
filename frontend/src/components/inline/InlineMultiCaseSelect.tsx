import { useMemo, useRef, useState } from 'react'
import InlineFieldFrame from './InlineFieldFrame'
import InlinePopover from './InlinePopover'
import styles from './Inline.module.css'
import { IconCheck } from '../Icons'
import { useTranslation } from '../../i18n'
import type { CaseRefItem } from './InlineCaseSelect'

interface Props {
  cases: CaseRefItem[]
  /** Pre-resolved cases for the current stored values, used for trigger
   *  labels when values are not present in the picker (cases) list. */
  resolvedCases?: CaseRefItem[]
  /** Whether the CASE_REFS_BY_IDS resolution query is still in flight.
   *  While true, unresolved ids show a neutral "#id" instead of "Unavailable". */
  resolvedLoading?: boolean
  values: string[]
  onSave: (next: string[]) => Promise<void> | void
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

export default function InlineMultiCaseSelect({
  cases,
  resolvedCases = [],
  resolvedLoading = false,
  values,
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

  // Build a combined lookup: picker results + pre-resolved cases. Picker
  // results take precedence if a case appears in both (more up-to-date).
  const caseMap = useMemo(() => {
    const m = new Map<string, CaseRefItem>()
    for (const c of resolvedCases) m.set(String(c.id), c)
    for (const c of cases) m.set(String(c.id), c)
    return m
  }, [cases, resolvedCases])

  // Selected case objects for trigger label display. Falls back to undefined
  // when not in either list (unresolvable).
  const selectedCases = useMemo(
    () => values.map((id) => caseMap.get(id)),
    [caseMap, values],
  )

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

  const toggle = async (id: string) => {
    const next = values.includes(id)
      ? values.filter((v) => v !== id)
      : [...values, id]
    await onSave(next)
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
        {values.length === 0 ? (
          <span className={styles.placeholder}>{placeholder ?? t('placeholderSelectCaseRef')}</span>
        ) : (
          <span className={styles.triggerLabel}>
            {values.map((id, i) => {
              const c = selectedCases[i]
              return c != null ? caseLabel(c) : (resolvedLoading ? `#${id}` : t('caseRefUnavailable', { id }))
            }).join(', ')}
          </span>
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
            const active = values.includes(String(c.id))
            return (
              <button
                key={c.id}
                type="button"
                className={`${styles.option} ${active ? styles.optionActive : ''}`}
                onClick={() => void toggle(String(c.id))}
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
