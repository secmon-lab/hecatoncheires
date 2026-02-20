import { describe, it, expect } from 'vitest'
import { parseNotionID } from './notion'

describe('parseNotionID', () => {
  const wantUUID = 'a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6'

  it('parses raw 32-char hex ID', () => {
    expect(parseNotionID('a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6')).toBe(wantUUID)
  })

  it('parses UUID format with dashes', () => {
    expect(parseNotionID('a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6')).toBe(wantUUID)
  })

  it('normalizes uppercase hex ID', () => {
    expect(parseNotionID('A1B2C3D4E5F6A7B8C9D0E1F2A3B4C5D6')).toBe(wantUUID)
  })

  it('extracts ID from Notion URL with workspace and title', () => {
    expect(
      parseNotionID(
        'https://www.notion.so/myworkspace/My-Database-a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6?v=abc',
      ),
    ).toBe(wantUUID)
  })

  it('extracts ID from Notion URL without title prefix', () => {
    expect(
      parseNotionID(
        'https://www.notion.so/myworkspace/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6',
      ),
    ).toBe(wantUUID)
  })

  it('extracts ID from Notion URL without workspace', () => {
    expect(
      parseNotionID('https://www.notion.so/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6'),
    ).toBe(wantUUID)
  })

  it('extracts ID from notion.so without www', () => {
    expect(
      parseNotionID(
        'https://notion.so/workspace/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6?v=xyz',
      ),
    ).toBe(wantUUID)
  })

  it('handles trailing slash in URL', () => {
    expect(
      parseNotionID(
        'https://www.notion.so/workspace/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6/',
      ),
    ).toBe(wantUUID)
  })

  it('trims leading/trailing spaces', () => {
    expect(parseNotionID('  a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6  ')).toBe(wantUUID)
  })

  it('handles UUID dashes in URL path', () => {
    expect(
      parseNotionID(
        'https://www.notion.so/workspace/a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6',
      ),
    ).toBe(wantUUID)
  })

  it('parses real Notion URL with query params', () => {
    expect(
      parseNotionID(
        'https://www.notion.so/mztn/2e6e628816658068b14bf84b39ad0762?v=2e6e6288166580199635000c717d60e7&source=copy_link',
      ),
    ).toBe('2e6e6288-1665-8068-b14b-f84b39ad0762')
  })

  // Error cases
  it('returns null for empty string', () => {
    expect(parseNotionID('')).toBeNull()
  })

  it('returns null for whitespace only', () => {
    expect(parseNotionID('   ')).toBeNull()
  })

  it('returns null for too short hex', () => {
    expect(parseNotionID('a1b2c3d4')).toBeNull()
  })

  it('returns null for non-hex characters', () => {
    expect(parseNotionID('g1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6')).toBeNull()
  })

  it('returns null for URL with wrong host', () => {
    expect(
      parseNotionID('https://example.com/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6'),
    ).toBeNull()
  })

  it('returns null for Notion URL with no valid ID in path', () => {
    expect(
      parseNotionID('https://www.notion.so/workspace/some-page'),
    ).toBeNull()
  })

  it('returns null for random string', () => {
    expect(parseNotionID('not-a-valid-id-at-all')).toBeNull()
  })
})
