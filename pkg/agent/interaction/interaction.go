// Package interaction defines a host-neutral port for soliciting input
// from a human user in the middle of an agent run.
//
// The port deliberately does NOT depend on planexec (or any specific
// runtime). The planexec runtime reaches it only through an adapter in the
// Job host (planexec.Question -> interaction.Request), and a future agent
// tool can depend on the very same interface without importing planexec.
// Keeping this type set runtime-agnostic is what lets the interaction
// capability grow (instruction injection, approval gates, ...) without
// rippling planexec types across the codebase.
package interaction

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
)

// Bounds applied by Request.Validate. They mirror the planner's question
// constraints so a Request built from a planexec.Question round-trips
// without surprise rejections, and so a tool-built Request is held to the
// same UX limits.
const (
	minItems       = 1
	maxItems       = 5
	minSelectOpts  = 2
	maxItemOptions = 20

	// Slack Block Kit length limits the host renderer must respect. An item
	// ID becomes part of a Slack block_id (max 255) with a fixed prefix and a
	// ":other" suffix, so it is capped well under 255. Item text renders as an
	// input label (max 2000). An option string is used as BOTH the option text
	// and value (each max 75 in Slack), so it is capped at 75. The reason
	// renders in a section (max 3000). Enforcing these here makes an
	// LLM-produced question that Slack would reject fail loudly at the call
	// site with a clear error, rather than as an opaque Slack 400 at post time.
	maxItemIDLen = 200
	maxItemText  = 2000
	maxOptionLen = 75
	maxReasonLen = 3000
)

// ItemType discriminates how the host should render the answer control.
// Closed-list types (select / multi_select) require non-empty Options;
// free_text ignores Options.
type ItemType string

const (
	ItemSelect      ItemType = "select"
	ItemMultiSelect ItemType = "multi_select"
	ItemFreeText    ItemType = "free_text"
)

// IsValid reports whether t is a recognised item type.
func (t ItemType) IsValid() bool {
	switch t {
	case ItemSelect, ItemMultiSelect, ItemFreeText:
		return true
	default:
		return false
	}
}

// Item is one question within a Request.
type Item struct {
	// ID is unique within the Request; the host echoes it back in the
	// matching Answer so the caller can correlate.
	ID string
	// Text is the human-facing prompt for this item.
	Text string
	// Type selects the answer control. select / multi_select require
	// Options; free_text ignores them.
	Type ItemType
	// Options is the closed list of allowed answers for select /
	// multi_select. Ignored for free_text.
	Options []string
}

// Request is a host-neutral description of something the agent wants from
// the user mid-run. Today it carries a question (a set of items); it is
// shaped to grow by adding fields/Kind later without breaking callers.
type Request struct {
	// Reason is the shared rationale ("why am I asking?") rendered once
	// above the items.
	Reason string
	// Items is the ordered list of questions (1..maxItems).
	Items []Item
}

// Validate enforces the Request invariants at the call site: item count
// bounds, unique non-empty IDs, valid types, and Options presence for the
// closed-list types.
func (r *Request) Validate() error {
	if r == nil {
		return goerr.New("interaction request is nil")
	}
	if len(r.Reason) > maxReasonLen {
		return goerr.New("interaction reason too long",
			goerr.V("len", len(r.Reason)), goerr.V("max", maxReasonLen))
	}
	if len(r.Items) < minItems {
		return goerr.New("interaction request must have at least one item",
			goerr.V("items", len(r.Items)))
	}
	if len(r.Items) > maxItems {
		return goerr.New("interaction request has too many items",
			goerr.V("items", len(r.Items)),
			goerr.V("max", maxItems))
	}
	seen := make(map[string]struct{}, len(r.Items))
	for i := range r.Items {
		it := r.Items[i]
		if it.ID == "" {
			return goerr.New("interaction item id is empty",
				goerr.V("index", i))
		}
		if _, dup := seen[it.ID]; dup {
			return goerr.New("duplicate interaction item id",
				goerr.V("id", it.ID))
		}
		seen[it.ID] = struct{}{}
		if len(it.ID) > maxItemIDLen {
			return goerr.New("interaction item id too long",
				goerr.V("id", it.ID), goerr.V("len", len(it.ID)), goerr.V("max", maxItemIDLen))
		}
		if it.Text == "" {
			return goerr.New("interaction item text is empty",
				goerr.V("id", it.ID))
		}
		if len(it.Text) > maxItemText {
			return goerr.New("interaction item text too long",
				goerr.V("id", it.ID), goerr.V("len", len(it.Text)), goerr.V("max", maxItemText))
		}
		if !it.Type.IsValid() {
			return goerr.New("invalid interaction item type",
				goerr.V("id", it.ID),
				goerr.V("type", string(it.Type)))
		}
		switch it.Type {
		case ItemSelect, ItemMultiSelect:
			if len(it.Options) < minSelectOpts {
				return goerr.New("select item needs at least two options",
					goerr.V("id", it.ID),
					goerr.V("options", len(it.Options)))
			}
			if len(it.Options) > maxItemOptions {
				return goerr.New("select item has too many options",
					goerr.V("id", it.ID),
					goerr.V("options", len(it.Options)),
					goerr.V("max", maxItemOptions))
			}
			for _, opt := range it.Options {
				if opt == "" {
					return goerr.New("select item has an empty option",
						goerr.V("id", it.ID))
				}
				// Slack uses the option string as both the visible text and
				// the submitted value, each capped at 75.
				if len(opt) > maxOptionLen {
					return goerr.New("select option too long",
						goerr.V("id", it.ID), goerr.V("len", len(opt)), goerr.V("max", maxOptionLen))
				}
			}
		case ItemFreeText:
			// Options are ignored for free_text; nothing to enforce.
		}
	}
	return nil
}

// Answer is the user's reply for one Item, correlated by ID. Exactly the
// field matching the item's Type is populated.
type Answer struct {
	ID       string
	Choice   string   // ItemSelect
	Choices  []string // ItemMultiSelect
	FreeText string   // ItemFreeText
}

// Outcome reports what happened to a Solicit call. For the pause/resume
// model used by Jobs, Paused is true and the caller must stop and return;
// resumption happens out-of-band (the user answers via Slack, which
// re-enters the run). Answers is reserved for a future synchronous path
// and is empty when Paused is true.
type Outcome struct {
	Paused  bool
	Answers []Answer
}

// Interactor is the host port used to solicit input from the user. The
// planexec OnQuestion callback adapts to this; a future agent tool depends
// on the same interface. Implementations persist whatever state is needed
// to resume and post the question to the user's surface (e.g. Slack).
type Interactor interface {
	Solicit(ctx context.Context, req Request) (Outcome, error)
}
