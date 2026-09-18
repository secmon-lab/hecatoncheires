// Package slackfmt holds the one statement of how an agent must write text that
// is posted to Slack.
//
// It exists because the rules were previously written out by hand in two host
// prompts and absent from every other one, so the hosts that had no copy replied
// in Markdown — headings, horizontal rules and pipe tables reached a thread as
// literal characters, since Slack's mrkdwn has no syntax for any of them.
//
// The text is static: it carries no per-turn value, so a host that appends it
// keeps its system prompt byte-identical from one turn to the next and stays a
// prompt-cache hit.
package slackfmt

import (
	_ "embed"
	"strings"
)

//go:embed prompts/message_format.md
var messageFormat string

// toolHint is the short form for the argument description of a tool that posts to
// Slack. A tool spec rides on every LLM call of a run, so it states the rules that
// were actually broken rather than repeating the whole section the same host's
// system prompt already carries.
const toolHint = "Slack mrkdwn only: *bold*, _italic_, `code`, <https://example.com|label>. " +
	"Markdown headings (#), tables (| a | b |), horizontal rules (---) and **bold** do not render in Slack and are printed literally."

// Section returns the Slack message formatting rules to append to the system
// prompt of an agent host whose run can post text to Slack.
func Section() string {
	return strings.TrimSpace(messageFormat)
}

// ToolHint returns the short form for the text argument of a Slack posting tool.
func ToolHint() string {
	return toolHint
}
