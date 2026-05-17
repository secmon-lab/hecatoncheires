// displayName returns the human-facing name we show in the UI for a Slack-
// backed user. Prefer the Slack display name (`name`); fall back to the real
// name only when the display name is empty so users with no display name set
// are still identifiable instead of rendering as a blank.

export interface NameableUser {
  name?: string | null
  realName?: string | null
}

export function displayName(u: NameableUser | null | undefined): string {
  if (!u) return ''
  const n = (u.name ?? '').trim()
  if (n) return n
  return (u.realName ?? '').trim()
}
