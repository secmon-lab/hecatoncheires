import { useMemo, useRef, useState } from 'react'
import InlineFieldFrame from './InlineFieldFrame'
import InlinePopover from './InlinePopover'
import styles from './Inline.module.css'
import { IconCheck } from '../Icons'
import { useTranslation } from '../../i18n'
import type { InlineSelectOption } from './InlineSelect'

interface Props<V extends string = string> {
  values: V[]
  options: InlineSelectOption<V>[]
  onSave: (next: V[]) => Promise<void> | void
  ariaLabel: string
  placeholder?: string
  disabled?: boolean
  searchable?: boolean
  testId?: string
}

// Multi-select with toggleable options. Each click toggles selection and
// persists immediately. Popover stays open until outside-click / Escape.
export default function InlineMultiSelect<V extends string = string>({
  values,
  options,
  onSave,
  ariaLabel,
  placeholder,
  disabled,
  searchable = false,
  testId,
}: Props<V>) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const anchorRef = useRef<HTMLDivElement>(null)

  const selectedOptions = useMemo(
    () => options.filter((o) => values.includes(o.value)),
    [options, values],
  )

  const filtered = useMemo(() => {
    if (!query) return options
    const q = query.toLowerCase()
    return options.filter((o) => o.label.toLowerCase().includes(q))
  }, [options, query])

  const toggle = async (val: V) => {
    const next = values.includes(val)
      ? values.filter((v) => v !== val)
      : [...values, val]
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
        {selectedOptions.length === 0 ? (
          <span className={styles.placeholder}>{placeholder || '—'}</span>
        ) : (
          <span className={styles.triggerLabel}>
            {selectedOptions.map((o) => o.label).join(', ')}
          </span>
        )}
      </InlineFieldFrame>
      <InlinePopover
        anchor={anchorRef.current}
        open={open}
        onClose={() => { setOpen(false); setQuery('') }}
        testId={testId ? `${testId}-popover` : undefined}
      >
        {searchable && (
          <div className={styles.popoverSearch}>
            <input
              autoFocus
              className={styles.popoverSearchInput}
              placeholder={t('placeholderSearch')}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              data-testid={testId ? `${testId}-search` : undefined}
            />
          </div>
        )}
        {filtered.length === 0 ? (
          <div className={styles.optionEmpty}>{t('noDataAvailable')}</div>
        ) : (
          filtered.map((o) => {
            const active = values.includes(o.value)
            return (
              <button
                key={o.value}
                type="button"
                className={`${styles.option} ${active ? styles.optionActive : ''}`}
                onClick={() => void toggle(o.value)}
                data-testid={testId ? `${testId}-option-${o.value}` : undefined}
              >
                {o.icon ?? (
                  o.color && <span className={styles.pip} style={{ background: o.color }} />
                )}
                <span className={styles.optionLabel}>{o.label}</span>
                {active && <IconCheck size={12} className={styles.optionCheck} />}
              </button>
            )
          })
        )}
      </InlinePopover>
    </>
  )
}
