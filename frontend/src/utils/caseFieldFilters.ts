import { calendarDate } from './calendarDate'

export interface CaseFieldDefinition {
  id: string
  name: string
  type: string
  options?: { id: string; name: string }[] | null
  referenceWorkspaceId?: string | null
}

export interface CaseWithFields {
  accessDenied?: boolean
  fields?: { fieldId: string; value: unknown }[] | null
}

export type CaseFieldFilterValues = ReadonlyMap<string, readonly string[]>
const FIELD_PREFIX = 'field.'

// Repeated parameters represent OR, never comma-separated strings: text and
// option IDs may themselves contain commas. IDs stay stable when labels change.
export function readCaseFieldFilters(params: URLSearchParams): CaseFieldFilterValues {
  const filters = new Map<string, string[]>()
  for (const [key, value] of params) {
    if (!key.startsWith(FIELD_PREFIX) || !value) continue
    const id = key.slice(FIELD_PREFIX.length)
    if (!id) continue
    const values = filters.get(id) ?? []
    if (!values.includes(value)) values.push(value)
    filters.set(id, values)
  }
  return filters
}

export function writeCaseFieldFilter(params: URLSearchParams, id: string, values: readonly string[]): URLSearchParams {
  const next = new URLSearchParams(params)
  next.delete(`${FIELD_PREFIX}${id}`)
  for (const value of new Set(values)) {
    if (value) next.append(`${FIELD_PREFIX}${id}`, value)
  }
  next.delete('page')
  return next
}

export function clearCaseFieldFilters(params: URLSearchParams): URLSearchParams {
  const next = new URLSearchParams(params)
  for (const key of new Set(next.keys())) {
    if (key.startsWith(FIELD_PREFIX)) next.delete(key)
  }
  next.delete('page')
  return next
}

// Filtering happens on the same complete, access-controlled results the list
// and board already fetch, before pagination / grouping. No hidden data is
// requested, and restricted rows cannot satisfy a field condition.
export function matchesCaseFields(
  c: CaseWithFields | null | undefined,
  filters: CaseFieldFilterValues,
  definitions: readonly CaseFieldDefinition[],
): boolean {
  if (filters.size === 0) return true
  if (!c || c.accessDenied) return false
  for (const [id, selected] of filters) {
    const def = definitions.find((f) => f.id === id)
    if (!def) return false
    const raw = c.fields?.find((f) => f.fieldId === id)?.value
    if (raw == null || raw === '') return false
    const values = Array.isArray(raw) ? raw : [raw]
    const hit = selected.some((filterValue) => values.some((value) => {
      if (value == null || value === '') return false
      if (def.type === 'SELECT' || def.type === 'MULTI_SELECT') {
        const option = def.options?.find((o) => o.id === filterValue)
        // A configured ID wins over a colliding legacy display name.
        const stored = def.options?.find((o) => o.id === value)
        return Boolean(option && (stored ? stored.id === option.id : value === option.name))
      }
      if (def.type === 'NUMBER') {
        return filterValue.trim() !== '' && Number.isFinite(Number(filterValue))
          && Number(value) === Number(filterValue)
      }
      if (def.type === 'DATE') return calendarDate(value) === filterValue
      return String(value) === filterValue
    }))
    if (!hit) return false
  }
  return true
}


// Offer references present in the unfiltered result set, including archived
// targets. Resolve their labels with the access-controlled ID resolver; the
// creation picker's candidate search intentionally excludes archived targets.
export function collectCaseReferenceValues(
  cases: readonly (CaseWithFields | null | undefined)[],
  definitions: readonly CaseFieldDefinition[],
): ReadonlyMap<string, readonly string[]> {
  const referenceFields = new Set(definitions.filter((f) => f.type === 'CASE_REF' || f.type === 'MULTI_CASE_REF').map((f) => f.id))
  const result = new Map<string, string[]>()
  for (const c of cases) {
    if (!c || c.accessDenied) continue
    for (const field of c.fields ?? []) {
      if (!referenceFields.has(field.fieldId)) continue
      const values = result.get(field.fieldId) ?? []
      for (const value of Array.isArray(field.value) ? field.value : [field.value]) {
        if (value == null || value === '') continue
        const id = String(value)
        if (!values.includes(id)) values.push(id)
      }
      result.set(field.fieldId, values)
    }
  }
  return result
}
