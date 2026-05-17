import { describe, it, expect } from 'vitest'
import { displayName } from './user'

describe('displayName', () => {
  it('returns the Slack display name when present', () => {
    expect(displayName({ name: 'alice', realName: 'Alice Smith' })).toBe('alice')
  })

  it('falls back to realName when name is missing or blank', () => {
    expect(displayName({ name: '', realName: 'Alice Smith' })).toBe('Alice Smith')
    expect(displayName({ name: '   ', realName: 'Alice Smith' })).toBe('Alice Smith')
    expect(displayName({ name: null, realName: 'Alice Smith' })).toBe('Alice Smith')
    expect(displayName({ name: undefined, realName: 'Alice Smith' })).toBe('Alice Smith')
    expect(displayName({ realName: 'Alice Smith' })).toBe('Alice Smith')
  })

  it('trims whitespace from the chosen value', () => {
    expect(displayName({ name: '  alice  ', realName: 'Alice Smith' })).toBe('alice')
    expect(displayName({ name: '', realName: '  Alice Smith  ' })).toBe('Alice Smith')
  })

  it('returns empty string when both name and realName are missing or blank', () => {
    expect(displayName({})).toBe('')
    expect(displayName({ name: '', realName: '' })).toBe('')
    expect(displayName({ name: '  ', realName: '  ' })).toBe('')
    expect(displayName({ name: null, realName: null })).toBe('')
  })

  it('returns empty string for null / undefined input', () => {
    expect(displayName(null)).toBe('')
    expect(displayName(undefined)).toBe('')
  })
})
