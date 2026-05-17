// displayName returns the human-facing name we show in the UI for a Slack-
// backed user. The backend stuffs the Slack profile display name into
// `realName` (see pkg/domain/model/slack_user.go — the field name is legacy);
// fall back to `name` (the Slack handle, e.g. "alice") when no display name
// is set, and finally to `id` so users with no human-readable name at all
// are still identifiable in the UI rather than rendering as a blank.

export interface NameableUser {
  id?: string | null
  name?: string | null
  realName?: string | null
}

export function displayName(u: NameableUser | null | undefined): string {
  if (!u) return ''
  const r = (u.realName ?? '').trim()
  if (r) return r
  const n = (u.name ?? '').trim()
  if (n) return n
  return (u.id ?? '').trim()
}
