import { describe, it, expect } from 'vitest'
import { sanitizeFieldValues } from './sanitizeFieldValues'

const defs = [
  { id: 'position', type: 'SELECT', options: [{ id: 'fe', name: 'FE' }, { id: 'be', name: 'BE' }] },
  { id: 'category', type: 'MULTI_SELECT', options: [{ id: 'a', name: 'A' }, { id: 'b', name: 'B' }] },
  { id: 'title', type: 'TEXT' },
]

describe('sanitizeFieldValues', () => {
  it('keeps valid SELECT option', () => {
    const out = sanitizeFieldValues([{ fieldId: 'position', value: 'fe' }], defs)
    expect(out).toEqual([{ fieldId: 'position', value: 'fe' }])
  })

  it('drops SELECT value whose option id is unknown', () => {
    const out = sanitizeFieldValues([{ fieldId: 'position', value: 'frontend-engineer' }], defs)
    expect(out).toEqual([])
  })

  it('filters unknown ids from MULTI_SELECT, keeps valid ones', () => {
    const out = sanitizeFieldValues([{ fieldId: 'category', value: ['a', 'unknown', 'b'] }], defs)
    expect(out).toEqual([{ fieldId: 'category', value: ['a', 'b'] }])
  })

  it('drops MULTI_SELECT entirely if all ids are unknown', () => {
    const out = sanitizeFieldValues([{ fieldId: 'category', value: ['x', 'y'] }], defs)
    expect(out).toEqual([])
  })

  it('passes through unknown field ids untouched', () => {
    const out = sanitizeFieldValues([{ fieldId: 'mystery', value: 'whatever' }], defs)
    expect(out).toEqual([{ fieldId: 'mystery', value: 'whatever' }])
  })

  it('passes through TEXT values untouched', () => {
    const out = sanitizeFieldValues([{ fieldId: 'title', value: 'hi' }], defs)
    expect(out).toEqual([{ fieldId: 'title', value: 'hi' }])
  })
})
