import { describe, expect, it } from 'vitest'
import { calendarDate, formatCalendarDate } from './calendarDate'

describe('calendar dates', () => {
  it.each(['2026-10-01', '2026-10-01T00:00:00Z', '2026-10-01T00:00:00+09:00', '2026-10-01T23:00:00-07:00'])('preserves the calendar day of %s', (value) => {
    expect(calendarDate(value)).toBe('2026-10-01')
    expect(formatCalendarDate(value)).toBe(new Date('2026-10-01T00:00:00Z').toLocaleDateString(undefined, { timeZone: 'UTC' }))
  })
  it.each(['2026-02-30', 'invalid', '', null])('rejects invalid dates and keeps malformed display values readable: %s', (value) => {
    expect(calendarDate(value)).toBe('')
    expect(formatCalendarDate(value)).toBe(value == null ? '' : value)
  })
})
