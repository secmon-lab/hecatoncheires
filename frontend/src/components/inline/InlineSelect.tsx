import { useMemo, useRef, useState, type ReactNode } from 'react'
import InlineFieldFrame from './InlineFieldFrame'
import InlinePopover from './InlinePopover'
import styles from './Inline.module.css'
import { IconCheck } from '../Icons'
import { useTranslation } from '../../i18n'

export interface InlineSelectOption<V extends string = string> {
  value: V
  label: string
  /** Optional pip color (rendered as a small dot before the label). */
  color?: string
  /** Optional fully custom icon node (overrides `color`). */
  icon?: ReactNode
}

interface Props<V extends string = string> {
  value: V | null | undefined
  options: InlineSelectOption<V>[]
  onSave: (next: V) => Promise<void> | void
  ariaLabel: string
  placeholder?: string
  disabled?: boolean
  searchable?: boolean
  testId?: string
  /** Allow clearing the value (renders a "—" / clear option). */
  clearable?: boolean
}

export default function InlineSelect<V extends string = string>({
  value,
  options,
  onSave,
  ariaLabel,
  placeholder,
  disabled,
  searchable = false,
  testId,
  clearable = false,
}: Props<V>) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const anchorRef = useRef<HTMLDivElement>(null)

  const current = useMemo(
    () => options.find((o) => o.value === value) || null,
    [options, value],
  )

  const filtered = useMemo(() => {
    if (!query) return options
    const q = query.toLowerCase()
    return options.filter((o) => o.label.toLowerCase().includes(q))
  }, [options, query])

  const handlePick = async (next: V | null) => {
    setOpen(false)
    setQuery('')
    if (next === value) return
    if (next == null) return // clear handling reserved for future
    await onSave(next)
  }

  return (
    <>
      <div ref={anchorRef} style={{ display: 'inline-flex', maxWidth: '100%' }}>
        <InlineFieldFrame
          ariaLabel={ariaLabel}
          disabled={disabled}
          onActivate={() => setOpen((v) => !v)}
          testId={testId}
        >
          {current ? (
            <>
              {current.icon ?? (
                current.color && (
                  <span className={styles.pip} style={{ background: current.color }} />
                )
              )}
              <span className={styles.optionLabel}>{current.label}</span>
            </>
          ) : (
            <span className={styles.placeholder}>{placeholder || '—'}</span>
          )}
        </InlineFieldFrame>
      </div>
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
            const active = o.value === value
            return (
              <button
                key={o.value}
                type="button"
                className={`${styles.option} ${active ? styles.optionActive : ''}`}
                onClick={() => void handlePick(o.value)}
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
        {clearable && value != null && (
          <button
            type="button"
            className={styles.option}
            onClick={() => { setOpen(false); void onSave(null as unknown as V) }}
            data-testid={testId ? `${testId}-clear` : undefined}
          >
            <span className={styles.optionLabel} style={{ color: 'var(--fg-muted)' }}>
              {t('btnClear')}
            </span>
          </button>
        )}
      </InlinePopover>
    </>
  )
}
