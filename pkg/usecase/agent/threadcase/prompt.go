package threadcase

import (
	"fmt"
	"strings"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// Mode discriminates the purpose of a thread-mode turn.
type Mode int

const (
	// ModeMention is a user @-mention in a case thread. The planner may
	// respond, update fields (materialize), or close the case.
	ModeMention Mode = iota
	// ModeMaterialize runs right after a case is auto-created from a
	// monitored-channel post. The planner investigates the message and emits
	// a materialize decision to fill title / description / fields.
	ModeMaterialize
)

// buildSystemPrompt renders the planner system prompt for a thread-mode turn.
// It inlines the case snapshot, the workspace field schema, and the board
// status vocabulary so the planner can fill fields and pick a close status.
func buildSystemPrompt(c *model.Case, ws *model.WorkspaceEntry, mode Mode) string {
	var b strings.Builder

	b.WriteString("You are an investigation agent operating inside a Slack thread that represents a single case.\n")
	switch mode {
	case ModeMaterialize:
		b.WriteString("A new case was just created from the first message in this thread. Investigate the message (using the read-only tools when helpful) and emit a `materialize` decision that fills a concise title, a clear description, and any custom fields you are confident about.\n")
	default:
		b.WriteString("A user mentioned you in this case thread. Investigate as needed and then choose ONE terminal decision: `respond` to answer the user, `materialize` to update the case title/description/fields, or `close` to mark the case done when the thread indicates it is resolved.\n")
	}
	b.WriteString("You CANNOT create or manage Actions and you CANNOT create drafts — this is a thread-mode case. Sub-agent tools are read-only.\n\n")

	if c != nil {
		b.WriteString("# Current case\n")
		fmt.Fprintf(&b, "- Title: %s\n", orPlaceholder(c.Title))
		fmt.Fprintf(&b, "- Description: %s\n", orPlaceholder(c.Description))
		if c.BoardStatus != "" {
			fmt.Fprintf(&b, "- Current status: %s\n", c.BoardStatus)
		}
		if len(c.FieldValues) > 0 {
			b.WriteString("- Existing field values:\n")
			for id, fv := range c.FieldValues {
				fmt.Fprintf(&b, "  - %s: %v\n", id, fv.Value)
			}
		}
		b.WriteString("\n")
	}

	if ws != nil && ws.FieldSchema != nil && len(ws.FieldSchema.Fields) > 0 {
		b.WriteString("# Custom field schema (for materialize)\n")
		for _, f := range ws.FieldSchema.Fields {
			fmt.Fprintf(&b, "- %s (id=%s, type=%s)", f.Name, f.ID, f.Type)
			if f.Description != "" {
				fmt.Fprintf(&b, " description=%q", f.Description)
			}
			if len(f.Options) > 0 {
				opts := make([]string, 0, len(f.Options))
				for _, o := range f.Options {
					opts = append(opts, o.ID)
				}
				fmt.Fprintf(&b, " options=[%s]", strings.Join(opts, ", "))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if ws != nil && ws.CaseStatusSet != nil {
		closed := ws.CaseStatusSet.ClosedIDs()
		if len(closed) > 0 {
			fmt.Fprintf(&b, "# Closed status ids (for close): %s\n\n", strings.Join(closed, ", "))
		}
	}

	return b.String()
}

// buildUserInput assembles the first user message handed to the planner. The
// system / delta conversation messages are prepended; the current mention
// text is appended last (when present).
func buildUserInput(systemMessages, deltaMessages []ConversationMessage, mentionText, mentionTS string) string {
	var b strings.Builder
	if len(systemMessages) > 0 {
		b.WriteString("# Thread so far\n")
		writeMessages(&b, systemMessages, mentionTS)
		b.WriteString("\n")
	}
	if len(deltaMessages) > 0 {
		b.WriteString("# New messages since last mention\n")
		writeMessages(&b, deltaMessages, mentionTS)
		b.WriteString("\n")
	}
	if mentionText != "" {
		b.WriteString("# Current mention\n")
		b.WriteString(mentionText)
	}
	if b.Len() == 0 {
		// Defensive: never hand the planner an empty user input (planexec
		// rejects it at Validate). Materialize turns may have no mention text.
		return "Investigate this case and decide the next action."
	}
	return b.String()
}

func writeMessages(b *strings.Builder, msgs []ConversationMessage, skipTS string) {
	for _, m := range msgs {
		if skipTS != "" && m.Timestamp == skipTS {
			continue
		}
		name := m.UserName
		if name == "" {
			name = m.UserID
		}
		fmt.Fprintf(b, "[%s] %s: %s\n", m.Timestamp, name, m.Text)
	}
}

func orPlaceholder(s string) string {
	if s == "" {
		return "(empty)"
	}
	return s
}
