import { describe, expect, it } from 'vitest'
import { collectCaseReferenceValues, clearCaseFieldFilters, matchesCaseFields, readCaseFieldFilters, writeCaseFieldFilter } from './caseFieldFilters'

const defs = [
  { id: 'category', name: 'Category', type: 'SELECT', options: [{ id: 'it', name: 'IT' }, { id: 'sales', name: 'Sales' }] },
  { id: 'teams', name: 'Teams', type: 'MULTI_SELECT', options: [{ id: 'it', name: 'IT' }, { id: 'sales', name: 'Sales' }] },
  ...['TEXT', 'MARKDOWN', 'URL', 'NUMBER', 'DATE', 'USER', 'MULTI_USER', 'CASE_REF', 'MULTI_CASE_REF'].map((type) => ({ id: type, name: type, type })),
]
const filters = (query: string) => readCaseFieldFilters(new URLSearchParams(query))
const row = (fieldId: string, value: unknown) => ({ fields: [{ fieldId, value }] })

describe('case field filters', () => {
  it('reads repeated parameters without splitting commas or losing encoded values', () => {
    expect([...filters('field.category=it&field.category=sales&field.category=it&field.TEXT=a%2Cb%26c&field.TEXT=&field.=x&status=closed')])
      .toEqual([['category', ['it', 'sales']], ['TEXT', ['a,b&c']]])
  })

  it('updates only the requested field, preserves unrelated parameters and resets pagination', () => {
    const original = new URLSearchParams('status=closed&page=3&field.category=it&field.teams=sales&custom=keep')
    const next = writeCaseFieldFilter(original, 'category', ['sales', 'it', 'sales', ''])
    expect(next.getAll('field.category')).toEqual(['sales', 'it'])
    expect(next.get('field.teams')).toBe('sales')
    expect(next.get('status')).toBe('closed')
    expect(next.get('custom')).toBe('keep')
    expect(next.has('page')).toBe(false)
    expect(original.get('page')).toBe('3')
    expect(writeCaseFieldFilter(next, 'category', []).has('field.category')).toBe(false)
    expect(clearCaseFieldFilters(original).toString()).toBe('status=closed&custom=keep')
  })

  it('ORs values within one field and ANDs different fields', () => {
    const selected = filters('field.category=it&field.category=sales&field.teams=it')
    expect(matchesCaseFields({ fields: [{ fieldId: 'category', value: 'sales' }, { fieldId: 'teams', value: ['sales', 'it'] }] }, selected, defs)).toBe(true)
    expect(matchesCaseFields({ fields: [{ fieldId: 'category', value: 'sales' }, { fieldId: 'teams', value: ['sales'] }] }, selected, defs)).toBe(false)
    expect(matchesCaseFields(row('category', 'IT'), filters('field.category=it'), defs)).toBe(true)
  })

  it.each([
    ['TEXT', ' exact ', ' exact ', true], ['TEXT', 'Exact', 'exact', false],
    ['MARKDOWN', '**bold**', '**bold**', true], ['URL', 'https://example.com/a?b=c', 'https://example.com/a?b=c', true],
    ['NUMBER', 0, '0.0', true], ['NUMBER', 12, '012', true], ['NUMBER', 12, 'invalid', false], ['NUMBER', null, '0', false],
    ['DATE', '2026-09-30T00:00:00Z', '2026-09-30', true], ['DATE', '2026-09-29', '2026-09-30', false],
    ['USER', 'U123', 'U123', true], ['MULTI_USER', ['U123', 'U456'], 'U456', true],
    ['CASE_REF', '42', '42', true], ['MULTI_CASE_REF', ['42'], '42', true],
  ])('matches %s values (%j, %s)', (fieldId, value, selected, expected) => {
    const f = new Map([[fieldId, [selected]]])
    expect(matchesCaseFields(row(fieldId, value), f, defs)).toBe(expected)
  })

  it('does not widen a link with unknown fields or options, or match absent and restricted values', () => {
    expect(matchesCaseFields(row('category', 'it'), filters('field.unknown=it'), defs)).toBe(false)
    expect(matchesCaseFields(row('category', 'retired'), filters('field.category=retired'), defs)).toBe(false)
    expect(matchesCaseFields({ fields: [] }, filters('field.category=it'), defs)).toBe(false)
    expect(matchesCaseFields({ ...row('category', 'it'), accessDenied: true }, filters('field.category=it'), defs)).toBe(false)
    expect(matchesCaseFields({ accessDenied: true }, filters(''), defs)).toBe(true)
    expect(matchesCaseFields(undefined, filters('field.category=it'), defs)).toBe(false)
  })
})


describe('reference filter candidates', () => {
  it('collects distinct scalar and array references from accessible, unfiltered rows only', () => {
    const definitions = [
      { id: 'ref', name: 'Ref', type: 'CASE_REF' },
      { id: 'refs', name: 'Refs', type: 'MULTI_CASE_REF' },
      { id: 'text', name: 'Text', type: 'TEXT' },
    ]
    expect([...collectCaseReferenceValues([
      { fields: [{ fieldId: 'ref', value: '42' }, { fieldId: 'refs', value: ['99', '42'] }, { fieldId: 'text', value: '123' }] },
      { fields: [{ fieldId: 'ref', value: 42 }, { fieldId: 'refs', value: ['99', null, ''] }] },
      { accessDenied: true, fields: [{ fieldId: 'ref', value: '100' }] },
      null, { fields: null },
    ], definitions)]).toEqual([['ref', ['42']], ['refs', ['99', '42']]])
  })
})

describe('option ID precedence', () => {
  it.each(['SELECT', 'MULTI_SELECT'])('does not mistake a stored %s ID for another option label', (type) => {
    const definitions = [{ id: 'team', name: 'Team', type, options: [
      { id: 'a', name: 'b' }, { id: 'b', name: 'B team' }, { id: 'c', name: 'Legacy team' },
    ] }]
    const selected = new Map([['team', ['a']]])
    const value = type === 'MULTI_SELECT' ? ['b'] : 'b'
    expect(matchesCaseFields(row('team', value), selected, definitions)).toBe(false)
    expect(matchesCaseFields(row('team', value), new Map([['team', ['b']]]), definitions)).toBe(true)
    expect(matchesCaseFields(row('team', type === 'MULTI_SELECT' ? ['Legacy team'] : 'Legacy team'), new Map([['team', ['c']]]), definitions)).toBe(true)
  })
})
