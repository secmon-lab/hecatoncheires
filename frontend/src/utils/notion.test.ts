import { describe, it, expect } from 'vitest'
import { parseNotionDatabaseID } from './notion'

describe('parseNotionDatabaseID', () => {
  const wantUUID = 'a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6'

  it('parses raw 32-char hex ID', () => {
    expect(parseNotionDatabaseID('a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6')).toBe(wantUUID)
  })

  it('parses UUID format with dashes', () => {
    expect(parseNotionDatabaseID('a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6')).toBe(wantUUID)
  })

  it('normalizes uppercase hex ID', () => {
    expect(parseNotionDatabaseID('A1B2C3D4E5F6A7B8C9D0E1F2A3B4C5D6')).toBe(wantUUID)
  })

  it('extracts ID from Notion URL with workspace and title', () => {
    expect(
      parseNotionDatabaseID(
        'https://www.notion.so/myworkspace/My-Database-a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6?v=abc',
      ),
    ).toBe(wantUUID)
  })

  it('extracts ID from Notion URL without title prefix', () => {
    expect(
      parseNotionDatabaseID(
        'https://www.notion.so/myworkspace/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6',
      ),
    ).toBe(wantUUID)
  })

  it('extracts ID from Notion URL without workspace', () => {
    expect(
      parseNotionDatabaseID('https://www.notion.so/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6'),
    ).toBe(wantUUID)
  })

  it('extracts ID from notion.so without www', () => {
    expect(
      parseNotionDatabaseID(
        'https://notion.so/workspace/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6?v=xyz',
      ),
    ).toBe(wantUUID)
  })

  it('handles trailing slash in URL', () => {
    expect(
      parseNotionDatabaseID(
        'https://www.notion.so/workspace/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6/',
      ),
    ).toBe(wantUUID)
  })

  it('trims leading/trailing spaces', () => {
    expect(parseNotionDatabaseID('  a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6  ')).toBe(wantUUID)
  })

  it('handles UUID dashes in URL path', () => {
    expect(
      parseNotionDatabaseID(
        'https://www.notion.so/workspace/a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6',
      ),
    ).toBe(wantUUID)
  })

  it('parses real Notion URL with query params', () => {
    expect(
      parseNotionDatabaseID(
        'https://www.notion.so/mztn/2e6e628816658068b14bf84b39ad0762?v=2e6e6288166580199635000c717d60e7&source=copy_link',
      ),
    ).toBe('2e6e6288-1665-8068-b14b-f84b39ad0762')
  })

  // Error cases
  it('returns null for empty string', () => {
    expect(parseNotionDatabaseID('')).toBeNull()
  })

  it('returns null for whitespace only', () => {
    expect(parseNotionDatabaseID('   ')).toBeNull()
  })

  it('returns null for too short hex', () => {
    expect(parseNotionDatabaseID('a1b2c3d4')).toBeNull()
  })

  it('returns null for non-hex characters', () => {
    expect(parseNotionDatabaseID('g1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6')).toBeNull()
  })

  it('returns null for URL with wrong host', () => {
    expect(
      parseNotionDatabaseID('https://example.com/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6'),
    ).toBeNull()
  })

  it('returns null for Notion URL with no valid ID in path', () => {
    expect(
      parseNotionDatabaseID('https://www.notion.so/workspace/some-page'),
    ).toBeNull()
  })

  it('returns null for random string', () => {
    expect(parseNotionDatabaseID('not-a-valid-id-at-all')).toBeNull()
  })
})
