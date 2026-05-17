import { describe, it, expect } from 'vitest'
import { displayName } from './user'

describe('displayName', () => {
  it('returns realName (which carries the Slack profile display name) when present', () => {
    expect(displayName({ name: 'alice', realName: 'Alice Smith' })).toBe('Alice Smith')
  })

  it('falls back to name when realName is missing or blank', () => {
    expect(displayName({ name: 'alice', realName: '' })).toBe('alice')
    expect(displayName({ name: 'alice', realName: '   ' })).toBe('alice')
    expect(displayName({ name: 'alice', realName: null })).toBe('alice')
    expect(displayName({ name: 'alice', realName: undefined })).toBe('alice')
    expect(displayName({ name: 'alice' })).toBe('alice')
  })

  it('trims whitespace from the chosen value', () => {
    expect(displayName({ name: 'alice', realName: '  Alice Smith  ' })).toBe('Alice Smith')
    expect(displayName({ name: '  alice  ', realName: '' })).toBe('alice')
  })

  it('falls back to id when both realName and name are missing or blank', () => {
    expect(displayName({ id: 'U001', name: '', realName: '' })).toBe('U001')
    expect(displayName({ id: 'U001', name: '  ', realName: '  ' })).toBe('U001')
    expect(displayName({ id: 'U001', name: null, realName: null })).toBe('U001')
    expect(displayName({ id: '  U001  ' })).toBe('U001')
  })

  it('returns empty string when id, name, and realName are all missing or blank', () => {
    expect(displayName({})).toBe('')
    expect(displayName({ id: '', name: '', realName: '' })).toBe('')
    expect(displayName({ id: '  ', name: '  ', realName: '  ' })).toBe('')
    expect(displayName({ id: null, name: null, realName: null })).toBe('')
  })

  it('returns empty string for null / undefined input', () => {
    expect(displayName(null)).toBe('')
    expect(displayName(undefined)).toBe('')
  })
})
