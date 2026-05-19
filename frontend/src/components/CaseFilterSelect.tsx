import { useEffect, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent } from 'react'
import { IconChevDown } from './Icons'
import { commitOnEnter } from '../utils/keyboard'
import styles from './CaseFilterSelect.module.css'

export interface CaseFilterOption {
  id: number
  title: string
}

interface Props {
  cases: CaseFilterOption[]
  selectedCaseId: number | null
  onSelect: (caseId: number | null) => void
  caseLabel: string
  allLabel: string
  searchPlaceholder: string
  emptyLabel: string
  triggerAriaLabel: string
  searchAriaLabel: string
  extraOption?: CaseFilterOption | null
  testId?: string
}

interface MenuItem {
  /** null represents the "all" / clear option. */
  caseId: number | null
  label: string
  idLabel?: string
}

export default function CaseFilterSelect({
  cases,
  selectedCaseId,
  onSelect,
  caseLabel,
  allLabel,
  searchPlaceholder,
  emptyLabel,
  triggerAriaLabel,
  searchAriaLabel,
  extraOption,
  testId,
}: Props) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [activeIndex, setActiveIndex] = useState(0)
  const rootRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  const merged = useMemo<CaseFilterOption[]>(() => {
    if (!extraOption) return cases
    if (cases.some((c) => c.id === extraOption.id)) return cases
    return [extraOption, ...cases]
  }, [cases, extraOption])

  const selectedOption = useMemo(
    () => merged.find((c) => c.id === selectedCaseId) ?? null,
    [merged, selectedCaseId],
  )

  const items = useMemo<MenuItem[]>(() => {
    const q = query.trim().toLowerCase()
    const filtered = merged.filter((c) => {
      if (!q) return true
      const idStr = `#${c.id}`
      return (
        c.title.toLowerCase().includes(q) ||
        String(c.id).includes(q) ||
        idStr.includes(q)
      )
    })
    const mapped: MenuItem[] = filtered.map((c) => ({
      caseId: c.id,
      label: c.title,
      idLabel: `#${c.id}`,
    }))
    if (!q) {
      return [{ caseId: null, label: allLabel }, ...mapped]
    }
    return mapped
  }, [merged, query, allLabel])

  // Close on outside click.
  useEffect(() => {
    if (!open) return
    const handler = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  // Reset query and active index every time the popup re-opens, and focus
  // the search input so the user can start typing immediately.
  useEffect(() => {
    if (!open) {
      setQuery('')
      setActiveIndex(0)
      return
    }
    setActiveIndex(0)
    const id = window.setTimeout(() => inputRef.current?.focus(), 0)
    return () => window.clearTimeout(id)
  }, [open])

  // Whenever the visible item list shrinks (e.g. via filtering) keep
  // activeIndex in range so the highlighted row never drops off the list.
  useEffect(() => {
    if (activeIndex >= items.length && items.length > 0) {
      setActiveIndex(items.length - 1)
    }
  }, [items.length, activeIndex])

  const commitActive = () => {
    if (items.length === 0) {
      setOpen(false)
      return
    }
    const idx = Math.min(Math.max(activeIndex, 0), items.length - 1)
    const item = items[idx]
    onSelect(item.caseId)
    setOpen(false)
  }

  const inputCommit = commitOnEnter<HTMLInputElement>({
    onCommit: commitActive,
    onCancel: () => setOpen(false),
  })

  const handleInputKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      if (items.length === 0) return
      setActiveIndex((i) => (i + 1) % items.length)
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      if (items.length === 0) return
      setActiveIndex((i) => (i - 1 + items.length) % items.length)
      return
    }
    inputCommit(e)
  }

  const triggerLabel = caseLabel
  const triggerValue = selectedOption
    ? [`#${selectedOption.id}`, selectedOption.title].filter(Boolean).join(' ')
    : allLabel

  return (
    <div className="h-filter-dd" ref={rootRef} data-testid={testId}>
      <button
        type="button"
        className="h-filter-dd-trigger"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={triggerAriaLabel}
        data-testid={testId ? `${testId}-trigger` : undefined}
      >
        <span className="h-filter-dd-label">{triggerLabel}</span>
        <span className="h-filter-dd-value">{triggerValue}</span>
        <IconChevDown size={12} />
      </button>
      {open && (
        <div className="h-filter-dd-menu" role="listbox" aria-label={triggerAriaLabel}>
          <div className={styles.searchRow}>
            <input
              ref={inputRef}
              className={styles.searchInput}
              type="text"
              value={query}
              onChange={(e) => {
                setQuery(e.target.value)
                setActiveIndex(0)
              }}
              onKeyDown={handleInputKeyDown}
              placeholder={searchPlaceholder}
              aria-label={searchAriaLabel}
              data-testid={testId ? `${testId}-search` : undefined}
            />
          </div>
          {items.length === 0 ? (
            <div
              className={styles.empty}
              data-testid={testId ? `${testId}-empty` : undefined}
            >
              {emptyLabel}
            </div>
          ) : (
            <div className={styles.itemsScroll}>
              {items.map((item, idx) => {
                const isSelected = item.caseId === selectedCaseId
                const isActive = idx === activeIndex
                const value = item.caseId == null ? '__all__' : String(item.caseId)
                return (
                  <button
                    type="button"
                    key={value}
                    role="option"
                    aria-selected={isSelected}
                    className={`h-filter-dd-item ${isActive ? styles.itemActive : ''}`}
                    onMouseEnter={() => setActiveIndex(idx)}
                    onClick={() => {
                      onSelect(item.caseId)
                      setOpen(false)
                    }}
                    data-testid={
                      testId
                        ? `${testId}-item-${item.caseId == null ? 'all' : item.caseId}`
                        : undefined
                    }
                  >
                    <span className="h-filter-dd-check">{isSelected ? '✓' : ''}</span>
                    <span>
                      {item.idLabel && (
                        <span className={styles.idTag}>{item.idLabel}</span>
                      )}
                      {item.label}
                    </span>
                  </button>
                )
              })}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
