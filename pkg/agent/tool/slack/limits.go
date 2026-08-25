package slacktool

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/m-mizutani/goerr/v2"
)

// Limits bounds how much of a Slack read tool's result reaches the model
// context.
//
// Both message-returning tools (slack__search_messages and
// slack__get_messages) apply the same pair. That is deliberate: an operator
// configures one budget for what a Slack read may inject, rather than one per
// tool, so a Slack read tool added later is bounded without a new flag.
//
// A non-positive value disables that bound. The defaults belong to the caller
// (pkg/cli/config), so this package embeds none.
type Limits struct {
	// MaxTextBytes caps one message's text. A longer text is cut on a rune
	// boundary and its entry carries "text_truncated": true.
	//
	// This is the bound the model cannot work around: it may lower the search
	// tool's count parameter, but per-message length is not something it can
	// ask for less of, and a small number of long messages is what drives the
	// large calls.
	MaxTextBytes int

	// MaxResultBytes caps the combined JSON size of the messages one call
	// returns. Messages that no longer fit are dropped and the result reports
	// how many, so a call returning many short messages is bounded too.
	MaxResultBytes int
}

// truncateText cuts s to at most max bytes and reports whether anything was
// removed. The cut lands on a rune boundary because the result is handed to the
// model, to the run timeline and to the trace archive alike, and a broken rune
// would reach all three. A non-positive max leaves s untouched — that bound is
// disabled.
func truncateText(s string, max int) (string, bool) {
	if max <= 0 || len(s) <= max {
		return s, false
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
}

// resultBudget admits message entries until their combined encoded size would
// exceed the cap.
//
// It measures the JSON encoding of the whole entry rather than the text alone
// because that is what the tool result becomes on the wire: the keys and the
// per-message metadata occupy the same context the cap exists to protect.
//
// The sum of the entries is a few bytes short of the encoded result — the
// enclosing object, the separators and the surrounding fields are not counted —
// so the cap bounds the messages, not the whole document. That is deliberate:
// counting the frame would make the bound depend on which tool is calling and on
// how many targets a request happened to name, and the overhead is a fixed
// handful of bytes against a cap measured in kilobytes.
type resultBudget struct {
	remaining int
	bounded   bool
	admitted  int
}

// newResultBudget returns a budget of max bytes. A non-positive max admits
// everything.
func newResultBudget(max int) *resultBudget {
	return &resultBudget{remaining: max, bounded: max > 0}
}

// admit reports whether entry still fits, charging its size when it does.
//
// The FIRST entry of a call is admitted whatever its size. Without that floor a
// single message larger than the whole budget empties the result, and the agent
// has no way back: lowering `count` returns the same oversized message first
// again, so the call can only be repeated to the same end. Returning it over
// budget costs one message's overshoot once; returning nothing costs a turn and
// yields nothing. With MaxTextBytes set this is unreachable — an entry is then
// bounded near that cap — so the floor only takes effect where the operator
// disabled the per-message bound and left the per-call one on.
func (b *resultBudget) admit(entry map[string]any) (bool, error) {
	if !b.bounded {
		b.admitted++
		return true, nil
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return false, goerr.Wrap(err, "failed to measure a slack tool result entry")
	}
	if len(encoded) > b.remaining && b.admitted > 0 {
		return false, nil
	}
	b.remaining -= len(encoded)
	b.admitted++
	return true, nil
}
