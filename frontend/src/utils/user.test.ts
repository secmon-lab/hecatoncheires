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
