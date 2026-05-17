// displayName returns the human-facing name we show in the UI for a Slack-
// backed user. The backend stuffs the Slack profile display name into
// `realName` (see pkg/domain/model/slack_user.go — the field name is legacy);
// fall back to `name` (the Slack handle, e.g. "alice") only when no display
// name is available so users without a display name set are still
// identifiable.

export interface NameableUser {
  name?: string | null
  realName?: string | null
}

export function displayName(u: NameableUser | null | undefined): string {
  if (!u) return ''
  const r = (u.realName ?? '').trim()
  if (r) return r
  return (u.name ?? '').trim()
}
