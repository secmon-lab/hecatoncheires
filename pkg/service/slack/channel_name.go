package slack

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// truncateToMaxBytes truncates a string to fit within maxBytes while respecting UTF-8 character boundaries.
// This ensures that multi-byte characters (like Japanese) are not corrupted during truncation.
func truncateToMaxBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}

	// Build string by adding runes until we exceed maxBytes
	var result strings.Builder
	for _, r := range s {
		runeLen := utf8.RuneLen(r)
		if result.Len()+runeLen > maxBytes {
			break
		}
		result.WriteRune(r)
	}

	return result.String()
}

// NormalizeChannelName normalizes a string to be a valid Slack channel name
// Slack allows: lowercase letters, numbers, hyphens, underscores, and Unicode characters (including Japanese)
// Slack prohibits: uppercase (Latin), spaces, slashes, periods, commas, and special symbols
// Maximum length: 80 characters
func NormalizeChannelName(name string) string {
	// 1. Replace spaces with hyphens
	name = strings.ReplaceAll(name, " ", "-")

	// 2. Remove prohibited characters
	var result strings.Builder
	result.Grow(len(name))

	for _, r := range name {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_':
			// Allow: lowercase Latin letters, numbers, hyphens, underscores.
			result.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			// Convert uppercase Latin letters to lowercase.
			result.WriteRune(unicode.ToLower(r))
		case r > 127 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			// Allow non-ASCII *letters and digits* (Japanese kana/kanji,
			// accented characters, etc.). Punctuation, dashes, brackets,
			// quotes, separators, and symbols are intentionally stripped —
			// Slack's chat.create rejects e.g. em dash (U+2014) / en dash
			// (U+2013) / Japanese 「」、・ as `invalid_name_specials`,
			// and unicode.IsLetter / IsDigit is the most reliable allowlist.
			result.WriteRune(r)
		}
		// Everything else (ASCII symbols, Unicode punctuation / symbols /
		// separators) is dropped silently.
	}

	return result.String()
}

// GenerateRiskChannelName generates a Slack channel name for a risk
// Format: {prefix}-{id}-{normalized-name}
// Prefix must not be empty and should be provided by the caller
func GenerateRiskChannelName(riskID int64, riskName string, prefix string) string {
	// Normalize the prefix as well (to ensure it's valid)
	prefix = NormalizeChannelName(prefix)

	// Normalize the risk name
	normalizedName := NormalizeChannelName(riskName)

	// Generate channel name with prefix
	channelName := fmt.Sprintf("%s-%d-%s", prefix, riskID, normalizedName)

	// Truncate to 80 bytes if needed, respecting UTF-8 character boundaries
	channelName = truncateToMaxBytes(channelName, 80)

	// Remove trailing hyphens (if truncation resulted in a trailing hyphen)
	channelName = strings.TrimRight(channelName, "-")

	return channelName
}
