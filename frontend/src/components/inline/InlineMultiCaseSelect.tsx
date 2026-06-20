import { useMemo, useRef, useState } from 'react'
import InlineFieldFrame from './InlineFieldFrame'
import InlinePopover from './InlinePopover'
import styles from './Inline.module.css'
import { IconCheck } from '../Icons'
import { useTranslation } from '../../i18n'
import type { CaseRefItem } from './InlineCaseSelect'

interface Props {
  cases: CaseRefItem[]
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

  const selectedCases = useMemo(
    () => cases.filter((c) => values.includes(String(c.id))),
    [cases, values],
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
        {selectedCases.length === 0 ? (
          <span className={styles.placeholder}>{placeholder ?? t('placeholderSelectCaseRef')}</span>
        ) : (
          <span className={styles.triggerLabel}>
            {selectedCases.map((c) => caseLabel(c)).join(', ')}
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
