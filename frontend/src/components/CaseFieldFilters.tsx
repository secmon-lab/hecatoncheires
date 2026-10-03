import { useEffect, useId, useMemo, useRef, useState } from 'react'
import Select from 'react-select'
import { useTranslation } from '../i18n'
import { collectCaseReferenceValues, type CaseFieldDefinition, type CaseFieldFilterValues, type CaseWithFields } from '../utils/caseFieldFilters'
import Button from './Button'
import CaseFieldFilterValue from './CaseFieldFilterValue'
import { buildSelectStyles, portalProps } from './selectStyles'
import styles from './CaseFieldFilters.module.css'

interface Props {
  fields: readonly CaseFieldDefinition[]
  cases?: readonly (CaseWithFields | null | undefined)[]
  filters: CaseFieldFilterValues
  onChange: (id: string, values: readonly string[]) => void
  onClear: () => void
}

export default function CaseFieldFilters({ fields, cases, filters, onChange, onClear }: Props) {
  const { t } = useTranslation()
  const referenceValues = useMemo(() => collectCaseReferenceValues(cases ?? [], fields), [cases, fields])
  const [open, setOpen] = useState(false)
  const [panelLeft, setPanelLeft] = useState(0)
  // An added condition may be empty while its values are being entered. Only
  // applied values belong in the URL; empty rows are local editor state.
  const [pending, setPending] = useState<string[]>([])
  const [menuTarget, setMenuTarget] = useState<HTMLDivElement | null>(null)
  const ref = useRef<HTMLDivElement>(null)
  const anchor = useRef<HTMLDivElement>(null)
  const panelId = useId()
  const addId = useId()

  useEffect(() => {
    if (!open) return
    const closeOutside = (event: MouseEvent) => {
      if (!ref.current?.contains(event.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', closeOutside)
    return () => document.removeEventListener('mousedown', closeOutside)
  }, [open])

  if (fields.length === 0 && filters.size === 0) return null
  const ids = [...new Set([...filters.keys(), ...pending.filter((id) => fields.some((f) => f.id === id))])]
  const available = fields.filter((f) => !ids.includes(f.id)).map((f) => ({ value: f.id, label: f.name }))
  const remove = (id: string) => {
    setPending((prev) => prev.filter((v) => v !== id))
    onChange(id, [])
  }
  const clear = () => { setPending([]); onClear() }

  return (
    <div ref={ref} className={styles.root}>
      <div ref={anchor} className={styles.anchor}>
        <Button
          variant={filters.size ? 'primary' : 'secondary'}
          aria-expanded={open}
          aria-controls={panelId}
          onClick={() => {
            const left = anchor.current?.getBoundingClientRect().left ?? 0
            const width = Math.min(420, window.innerWidth - 32)
            setPanelLeft(Math.max(16 - left, Math.min(0, window.innerWidth - 16 - left - width)))
            setOpen((v) => !v)
          }}
          data-testid="case-field-filters-button"
        >
          {t('filterCaseFields')}{filters.size > 0 ? ` · ${filters.size}` : ''}
        </Button>
        {open && (
          <div
            id={panelId}
            className={styles.panel}
            style={{ left: panelLeft }}
            data-testid="case-field-filters-panel"
            onKeyDown={(e) => { if (e.key === 'Escape') setOpen(false) }}
          >
            <p className={styles.hint}>{t('filterCaseFieldsHint')}</p>
            {ids.map((id) => {
              const field = fields.find((f) => f.id === id) ?? { id, name: id, type: 'TEXT' }
              return (
                <div className={styles.field} key={id} data-testid={`case-field-filter-${id}`}>
                  <div className={styles.fieldHeader}>
                    <strong>{field.name}</strong>
                    <Button size="sm" variant="ghost" aria-label={t('filterRemoveCondition', { field: field.name })} onClick={() => remove(id)}>
                      ×
                    </Button>
                  </div>
                  <CaseFieldFilterValue field={field} values={filters.get(id) ?? []} onChange={(values) => onChange(id, values)} menuTarget={menuTarget} referenceValues={referenceValues.get(id)} />
                </div>
              )
            })}
            {available.length > 0 && (
              <div className={styles.addCondition}>
                <Select
                  inputId={addId}
                  aria-label={t('filterAddCondition')}
                  placeholder={t('filterAddCondition')}
                  options={available}
                  value={null}
                  onChange={(option) => { if (option) setPending((prev) => [...prev, option.value]) }}
                  styles={buildSelectStyles({ compact: true })}
                  {...portalProps}
                  menuPortalTarget={menuTarget}
                  noOptionsMessage={() => t('filterNoOptions')}
                />
              </div>
            )}
            {(filters.size > 0 || pending.length > 0) && (
              <Button size="sm" variant="ghost" onClick={clear} data-testid="case-field-filters-clear">
                {t('filterClear')}
              </Button>
            )}
          </div>
        )}
      </div>
      {filters.size > 0 && (
        <div className={styles.summary} data-testid="case-field-filters-summary" aria-label={t('filterCaseFields')}>
          {[...filters].map(([id, values]) => {
            const field = fields.find((f) => f.id === id) ?? { id, name: id, type: 'TEXT' }
            return (
              <div className={styles.condition} key={id}>
                <strong>{field.name}:</strong>
                <CaseFieldFilterValue field={field} values={values} onChange={(next) => onChange(id, next)} summary />
                <Button size="sm" variant="ghost" aria-label={t('filterRemoveCondition', { field: field.name })} onClick={() => remove(id)}>×</Button>
              </div>
            )
          })}
          <Button size="sm" variant="ghost" onClick={clear} data-testid="case-field-filters-clear-summary">{t('filterClear')}</Button>
        </div>
      )}
      {/* Keep portaled menus outside the scrolling panel, but inside our click
          boundary. Choosing an option must not dismiss the condition editor. */}
      <div ref={setMenuTarget} />
    </div>
  )
}
