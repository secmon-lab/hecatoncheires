package proposal

// Trigger discriminates how the host started a turn for a given Session.
// The prompt may use it (e.g. WSSwitch should redraft from the conversation
// rather than investigating again).
type Trigger int

const (
	// TriggerAppMention — the user @-mentioned the bot.
	TriggerAppMention Trigger = iota
	// TriggerThreadReply — the user replied in the thread without a mention,
	// while the prior turn ended on a question.
	TriggerThreadReply
	// TriggerWSSwitch — the user switched the active workspace via the
	// preview UI, requiring a redraft on the existing proposal.
	TriggerWSSwitch
)

// QuestionPayload is the pure-domain shape passed to Host.Ask.
type QuestionPayload struct {
	// Reason explains the information gap (single rationale shared across
	// all items).
	Reason string
	// Items is the ordered list of questions to ask in this turn. Always
	// non-empty (validation guarantees ≥1).
	Items []QuestionItem
}

// QuestionItemType is the host-rendering hint for a question item.
type QuestionItemType string

const (
	// QuestionItemSelect is a single-choice picker.
	QuestionItemSelect QuestionItemType = "select"
	// QuestionItemMultiSelect is a multi-choice picker.
	QuestionItemMultiSelect QuestionItemType = "multi_select"
	// QuestionItemFreeText is a multiline plain-text input. Reserved
	// for the last-resort case where neither investigation nor a
	// closed-list classification can capture what we need from the
	// user.
	QuestionItemFreeText QuestionItemType = "free_text"
)

// QuestionItem is one question within QuestionPayload.Items.
type QuestionItem struct {
	// ID uniquely identifies the question within the payload; the host
	// uses it to correlate answers back when the user submits.
	ID string
	// Text is the prompt shown to the user.
	Text string
	// Type discriminates the answer control (select / multi_select /
	// free_text).
	Type QuestionItemType
	// Options is the allowed answer set for select / multi_select
	// (always ≥2 entries). Ignored for free_text.
	Options []string
}

// MaterializePayload is the pure-domain shape passed to Host.Propose.
type MaterializePayload struct {
	WorkspaceID       string
	Title             string
	Description       string
	CustomFieldValues map[string]any
	// IsTest marks the proposed case as a test case. Defaults to false.
	IsTest bool
}
