import { describe, it, expect } from 'vitest'
import { toRFC3339WithOffset, formatHomeDate } from './time'

describe('toRFC3339WithOffset', () => {
  it('formats local date/time fields with a matching numeric offset', () => {
    const d = new Date(2026, 6, 17, 9, 5, 3) // 2026-07-17 09:05:03 local
    const result = toRFC3339WithOffset(d)

    expect(result).toMatch(/^2026-07-17T09:05:03[+-]\d{2}:\d{2}$/)

    const offsetMinutes = -d.getTimezoneOffset()
    const sign = offsetMinutes >= 0 ? '+' : '-'
    const hh = String(Math.floor(Math.abs(offsetMinutes) / 60)).padStart(2, '0')
    const mm = String(Math.abs(offsetMinutes) % 60).padStart(2, '0')
    expect(result).toBe(`2026-07-17T09:05:03${sign}${hh}:${mm}`)
  })

  it('never falls back to UTC (would silently drop the local offset)', () => {
    const d = new Date(2026, 0, 1, 0, 0, 0)
    const result = toRFC3339WithOffset(d)
    expect(result).not.toBe(d.toISOString())
  })

  it('zero-pads single-digit month, day, hour, minute, and second', () => {
    const d = new Date(2026, 0, 5, 3, 4, 5) // 2026-01-05 03:04:05
    const result = toRFC3339WithOffset(d)
    expect(result.startsWith('2026-01-05T03:04:05')).toBe(true)
  })
})

describe('formatHomeDate', () => {
  it('renders an English long-form date for lang=en', () => {
    const d = new Date(2026, 6, 17) // Friday, July 17, 2026
    const result = formatHomeDate(d, 'en')
    expect(result).toContain('2026')
    expect(result).toContain('July')
  })

  it('renders a Japanese long-form date for lang=ja', () => {
    const d = new Date(2026, 6, 17)
    const result = formatHomeDate(d, 'ja')
    expect(result).toContain('2026')
    expect(result).toMatch(/7月/)
  })
})
