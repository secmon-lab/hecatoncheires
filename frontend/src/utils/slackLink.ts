// Slack archive URL / permalink composition.
//
// Slack accepts archive URLs of the form {channelURL}/p{ts-without-dot}, and
// also tolerates the canonical https://slack.com/archives/... fallback when
// the workspace subdomain isn't known. The workspace-qualified form is
// preferred wherever the team URL is available because it names the target
// workspace unambiguously.

// buildSlackPermalink composes a permalink to a specific message in a channel.
export function buildSlackPermalink(
  channelURL: string | null | undefined,
  channelID: string | null | undefined,
  ts: string | null | undefined,
): string | null {
  if (!ts) return null
  const tsCompact = ts.replace('.', '')
  if (channelURL) {
    return `${trimTrailingSlash(channelURL)}/p${tsCompact}`
  }
  if (channelID) {
    return `https://slack.com/archives/${channelID}/p${tsCompact}`
  }
  return null
}

// buildSlackCaseLink composes the link that represents a Case in Slack.
//
// A thread-mode Case lives in a thread of a monitored channel, so its channel
// alone is the wrong destination — the link must carry the thread timestamp.
// A channel-mode Case owns its channel, so the channel archive URL is the
// destination.
//
// Invariant: a non-empty channelID always yields a non-null result. Callers
// deciding whether a cell renders its own <a> rely on this.
export function buildSlackCaseLink(
  channelURL: string | null | undefined,
  channelID: string | null | undefined,
  threadTS: string | null | undefined,
): string | null {
  if (!channelID) return null
  if (threadTS) return buildSlackPermalink(channelURL, channelID, threadTS)
  if (channelURL) return trimTrailingSlash(channelURL)
  return `https://slack.com/archives/${channelID}`
}

function trimTrailingSlash(url: string): string {
  return url.replace(/\/+$/, '')
}
