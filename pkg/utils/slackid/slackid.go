// Package slackid normalises Slack user identifiers across the codebase.
//
// Slack's OIDC sub claim is the composite "Uxxx-Txxx" (user-team) form
// — see https://api.slack.com/authentication/sign-in-with-slack —
// while every downstream API (channel invites, Slack interactivity
// callback.User.ID, the channel-membership cache) keys on the bare
// "Uxxx" / "Wxxx" user ID. We normalise at every persistence /
// resolution boundary so the rest of the codebase consistently sees
// a single user-ID form; otherwise reporter / actor IDs persisted
// from the Web side fail silently when downstream lookups (Slack
// API, SlackUser repository) reject the composite value.
package slackid

import "strings"

// Normalize extracts the Slack user ID portion (Uxxx or Wxxx) from
// an OIDC sub claim. Slack returns the sub as a hyphen-separated
// composite of the user ID and the team ID — in practice either
// form ("Uxxx-Txxx" or "Txxx-Uxxx") may appear depending on the
// workspace configuration / enterprise-grid setup, so we pick the
// first hyphen-separated chunk that looks like a user identifier.
// If no chunk matches we fall back to the raw value so a strange
// future sub format does not silently strip identity information.
func Normalize(sub string) string {
	if sub == "" {
		return sub
	}
	for part := range strings.SplitSeq(sub, "-") {
		if IsUserID(part) {
			return part
		}
	}
	return sub
}

// IsUserID reports whether s looks like a Slack user ID (a single
// "U" or "W" prefix followed by alphanumeric characters).
func IsUserID(s string) bool {
	if len(s) < 2 {
		return false
	}
	if s[0] != 'U' && s[0] != 'W' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		isDigit := c >= '0' && c <= '9'
		isUpper := c >= 'A' && c <= 'Z'
		isLower := c >= 'a' && c <= 'z'
		if !isDigit && !isUpper && !isLower {
			return false
		}
	}
	return true
}
