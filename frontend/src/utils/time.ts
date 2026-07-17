// Home-page time helpers.
//
// The backend's `homeMessage` query needs the browser's local wall-clock
// time WITH its UTC offset (so an LLM prompt can reason about "good
// morning" vs "good evening" in the user's timezone). `Date.toISOString()`
// always normalizes to UTC and drops the offset, so it cannot be used here.

function pad(n: number): string {
  return String(Math.abs(n)).padStart(2, '0')
}

// toRFC3339WithOffset formats a Date using its LOCAL calendar fields plus
// its local UTC offset, e.g. "2026-07-17T09:30:00+09:00". This is the
// `clientTime` argument the backend expects — never swap in
// `d.toISOString()`, which silently reports everything in UTC.
export function toRFC3339WithOffset(d: Date): string {
  const year = d.getFullYear()
  const month = pad(d.getMonth() + 1)
  const day = pad(d.getDate())
  const hours = pad(d.getHours())
  const minutes = pad(d.getMinutes())
  const seconds = pad(d.getSeconds())

  // getTimezoneOffset() returns minutes the local time is BEHIND UTC
  // (e.g. +300 for UTC-5), which is the opposite sign convention RFC3339
  // offsets use — negate it.
  const offsetMinutes = -d.getTimezoneOffset()
  const sign = offsetMinutes >= 0 ? '+' : '-'
  const offsetHours = pad(Math.floor(Math.abs(offsetMinutes) / 60))
  const offsetMins = pad(Math.abs(offsetMinutes) % 60)

  return `${year}-${month}-${day}T${hours}:${minutes}:${seconds}${sign}${offsetHours}:${offsetMins}`
}

// formatHomeDate renders the Home page's header date line in the given UI
// language, independent of the browser's own locale (which may differ from
// the user's chosen in-app language).
export function formatHomeDate(d: Date, lang: 'en' | 'ja'): string {
  const locale = lang === 'ja' ? 'ja-JP' : 'en-US'
  return new Intl.DateTimeFormat(locale, {
    weekday: 'long',
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  }).format(d)
}
