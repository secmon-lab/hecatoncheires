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

  const selectedCase = useMemo(
    () => (value != null ? cases.find((c) => String(c.id) === value) ?? null : null),
    [cases, value],
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
