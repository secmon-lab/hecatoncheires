import { useState, useRef, useEffect } from 'react'
import { IconChevDown } from './Icons'

export interface FilterOption {
  value: string
  label: string
}

interface Props {
  label: string
  allLabel: string
  options: FilterOption[]
  value: string[]
  onChange: (next: string[]) => void
  testId?: string
}

// Ghost-style dropdown rendered as `Label value ▼`.
// Multi-select via checkbox list inside a popover. No bordered control.
export default function FilterDropdown({
  label, allLabel, options, value, onChange, testId,
}: Props) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [open])

  const valueLabel = value.length === 0
    ? allLabel
    : value.length === 1
      ? options.find((o) => o.value === value[0])?.label || allLabel
      : `${value.length}`

  const toggle = (v: string) => {
    if (value.includes(v)) onChange(value.filter((x) => x !== v))
    else onChange([...value, v])
  }

  return (
    <div className="h-filter-dd" ref={ref} data-testid={testId}>
      <button
        type="button"
        className="h-filter-dd-trigger"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="listbox"
        aria-expanded={open}
      >
        <span className="h-filter-dd-label">{label}</span>
        <span className="h-filter-dd-value">{valueLabel}</span>
        <IconChevDown size={12} />
      </button>
      {open && (
        <div className="h-filter-dd-menu" role="listbox">
          <button
            type="button"
            className="h-filter-dd-item"
            onClick={() => onChange([])}
          >
            <span className="h-filter-dd-check">{value.length === 0 ? '✓' : ''}</span>
            <span>{allLabel}</span>
          </button>
          {options.map((o) => (
            <button
              type="button"
              key={o.value}
              className="h-filter-dd-item"
              onClick={() => toggle(o.value)}
            >
              <span className="h-filter-dd-check">{value.includes(o.value) ? '✓' : ''}</span>
              <span>{o.label}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
