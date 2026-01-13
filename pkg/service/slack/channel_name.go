package slack

import (
	"fmt"
	"strings"
	"unicode"
)

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
		// Allow: lowercase Latin letters, numbers, hyphens, underscores
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			result.WriteRune(r)
		} else if r >= 'A' && r <= 'Z' {
			// Convert uppercase Latin letters to lowercase
			result.WriteRune(unicode.ToLower(r))
		} else if r > 127 {
			// Allow all non-ASCII characters (Japanese, accented characters, etc.)
			// except for specific prohibited ones
			if !isProhibitedSymbol(r) {
				result.WriteRune(r)
			}
		}
		// Prohibited: slashes, periods, commas, and other ASCII symbols (except hyphen and underscore)
	}

	return result.String()
}

// isProhibitedSymbol checks if a Unicode character is prohibited in Slack channel names
func isProhibitedSymbol(r rune) bool {
	// Japanese punctuation marks that should be removed
	prohibitedRunes := []rune{
		'。', '、', '!', '?', '/', '\\', '.', ',', '!', '?',
		'@', '#', '$', '%', '^', '&', '*', '(', ')', '[', ']',
		'{', '}', '<', '>', '|', '~', '`', '\'', '"', ';', ':',
		'+', '=',
	}

	for _, prohibited := range prohibitedRunes {
		if r == prohibited {
			return true
		}
	}
	return false
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

	// Truncate to 80 characters if needed
	if len(channelName) > 80 {
		channelName = channelName[:80]
	}

	// Remove trailing hyphens (if truncation resulted in a trailing hyphen)
	channelName = strings.TrimRight(channelName, "-")

	return channelName
}
