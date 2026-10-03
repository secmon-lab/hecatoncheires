// Custom DATE fields represent a calendar day, not an instant in the viewer's
// timezone. Preserve the stored day even when RFC3339 includes an offset.
export function calendarDate(value: unknown): string {
  if (value == null) return ''
  const match = String(value).match(/^(\d{4}-\d{2}-\d{2})(?:T|$)/)
  if (!match) return ''
  const day = match[1]
  const date = new Date(`${day}T00:00:00Z`)
  return !Number.isNaN(date.getTime()) && date.toISOString().slice(0, 10) === day ? day : ''
}

export function formatCalendarDate(value: unknown): string {
  const day = calendarDate(value)
  if (!day) return value == null ? '' : String(value)
  return new Date(`${day}T00:00:00Z`).toLocaleDateString(undefined, { timeZone: 'UTC' })
}
