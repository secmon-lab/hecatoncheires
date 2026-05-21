package usecase

import "regexp"

// slackPlainTextMaxRunes caps the number of runes we hand Slack as the
// initial_value of a plain_text_input element. Slack's plain_text_input
// rejects views.open with invalid_arguments once initial_value exceeds the
// element's max_length (default and ceiling both 3000). We sit well under
// that so multibyte / emoji / newline width quirks never push us over.
//
// The planner prompt instructs the LLM to produce <=2000 characters for
// long-form fields; the extra 500-rune headroom here absorbs occasional
// overrun without failing the modal open.
const slackPlainTextMaxRunes = 2500

const (
	clampSuffixSingleLine = "…"
	clampSuffixMultiLine  = "\n\n…(truncated)"
)

// clampPlainText returns value truncated to fit inside Slack's
// plain_text_input element. Truncation respects rune boundaries so we never
// split a multibyte character. When truncation happens, a suffix is
// appended so the human sees the value was clipped; multiline inputs get a
// blank line + sentinel, single-line inputs get a bare ellipsis.
//
// The returned string is always <= slackPlainTextMaxRunes + suffix length
// in runes, comfortably below Slack's 3000-rune ceiling.
func clampPlainText(value string, multiline bool) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	if len(runes) <= slackPlainTextMaxRunes {
		return value
	}
	suffix := clampSuffixSingleLine
	if multiline {
		suffix = clampSuffixMultiLine
	}
	return string(runes[:slackPlainTextMaxRunes]) + suffix
}

// slackOptionDescriptionMaxRunes is Slack's hard cap on
// option.description.text for static_select / multi_static_select option
// objects. Exceeding it (e.g. an operator-authored field option with a
// long description in the workspace config) makes Slack reject the entire
// views.open with invalid_arguments, taking every custom-field modal
// (Create / Edit / Draft Edit) down with it. Description text is only
// rendered as a small subtitle in the dropdown anyway and gets visually
// truncated by Slack on display, so trimming the tail to fit the cap
// keeps the visible portion intact.
const slackOptionDescriptionMaxRunes = 75

// clampSlackOptionDescription truncates value so the resulting rune count
// never exceeds slackOptionDescriptionMaxRunes. Truncation respects rune
// boundaries so multibyte characters (e.g. Japanese) are not split. When
// truncation happens, the last rune is replaced with an ellipsis so the
// final rune count is exactly slackOptionDescriptionMaxRunes. Empty input
// is returned unchanged so the caller can keep its "omit description when
// blank" branch (Slack rejects empty description.text just as it rejects
// over-long ones).
func clampSlackOptionDescription(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	if len(runes) <= slackOptionDescriptionMaxRunes {
		return value
	}
	// Reserve room for the suffix in rune units so changing
	// clampSuffixSingleLine to a longer sentinel does not silently push us
	// back over the 75-rune cap.
	suffixRunes := len([]rune(clampSuffixSingleLine))
	return string(runes[:slackOptionDescriptionMaxRunes-suffixRunes]) + clampSuffixSingleLine
}

// slackUserIDPattern matches the rendered form of a Slack user ID. Real
// user IDs start with U (regular users) or W (Enterprise Grid org-level
// users), followed by uppercase alphanumerics. Bots ("B…"), apps ("A…"),
// channels ("C…"), and free-form strings (emails, names) MUST NOT be
// passed as initial_user(s) on a users_select — Slack rejects the view.
var slackUserIDPattern = regexp.MustCompile(`^[UW][A-Z0-9]{2,}$`)

// isLikelySlackUserID reports whether s looks like a Slack user ID. It is
// a syntactic check only — it does not confirm the user exists, only that
// the value is shaped like an ID Slack would accept as initial_user.
func isLikelySlackUserID(s string) bool {
	return slackUserIDPattern.MatchString(s)
}

// filterSlackUserIDs returns only the entries of ids that look like Slack
// user IDs. The relative order is preserved. The returned slice is a fresh
// allocation; the caller may mutate it.
func filterSlackUserIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if isLikelySlackUserID(id) {
			out = append(out, id)
		}
	}
	return out
}
