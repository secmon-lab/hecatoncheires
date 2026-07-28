package threadcase

import (
	"fmt"
	"strings"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
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
	// ModeCreate runs when a monitored-channel post arrives but NO case exists
	// yet. The planner investigates / asks the user, and only commits a new
	// case (final create decision) once it can satisfy validation.
	ModeCreate
)

// buildSystemPrompt renders the planner system prompt for a thread-mode turn.
// It inlines the case snapshot, the workspace field schema, and the board
// status vocabulary so the planner can fill fields and pick a close status.
func buildSystemPrompt(c *model.Case, ws *model.WorkspaceEntry, mode Mode, createInstruction string) string {
	var b strings.Builder

	b.WriteString("You are an investigation agent operating inside a Slack thread that represents a single case.\n")
	switch mode {
	case ModeCreate:
		b.WriteString("A message was posted in a monitored channel, but NO case exists yet. Do NOT rush to create one. First do light investigation about the reporter and the topic (recent messages, related threads) using the read-only search tools. When the intent or required information is unclear, ask the user a `question` (a select / multi-select form) instead of guessing. Once the direction is clear, investigate deeper, and only then emit the final create decision with a concise title, a clear description, and every custom field required by the schema. The case is validated against the workspace field schema when it is created: satisfy every field marked (required), use only the listed option ids, and give date fields an RFC3339 timestamp. If a value is rejected the error is fed back to you and you get a few attempts to correct it, but aim to get it right the first time.\n")
	case ModeMaterialize:
		b.WriteString("A new case was just created from the first message in this thread. Investigate the message (using the read-only tools when helpful) and emit a `materialize` decision that fills a concise title, a clear description, and any custom fields you are confident about.\n")
	default:
		b.WriteString("A user mentioned you in this case thread. Investigate as needed. When the thread calls for a change to the case — a board status move (including closing it), an assignee change, or a content edit — dispatch a task that uses the matching `case__*` write tool; do NOT merely describe the change in your final answer, actually call the tool. Your terminal decision is then ONE of: `respond` to answer the user, or `materialize` to update the case title/description/fields.\n")
	}
	switch mode {
	case ModeMention:
		b.WriteString("You CANNOT create or manage Actions and you CANNOT create drafts — this is a thread-mode case. Sub-agents may read (Slack / Notion / GitHub / the web) and may write to this case: `case__update_case_status` (board status), `case__assign` / `case__unassign` (assignees), and `case__update_case` (title / description / custom fields). The assignee tools take Slack user IDs, never display names: resolve a name to its user ID from the thread messages first, and never guess an ID. Because a `materialize` decision REPLACES the title and description wholesale, pick one content path per turn — either edit with `case__update_case` inside the loop, or emit `materialize` at the end, never both.\n\n")
	default:
		b.WriteString("You CANNOT create or manage Actions and you CANNOT create drafts — this is a thread-mode case. Sub-agent tools are read-only.\n\n")
	}

	if c != nil {
		b.WriteString("# Current case\n")
		fmt.Fprintf(&b, "- Title: %s\n", orPlaceholder(c.Title))
		fmt.Fprintf(&b, "- Description: %s\n", orPlaceholder(c.Description))
		// Always rendered, including when empty: the agent has no tool to read the
		// case back, so an omitted line would leave it unable to tell "no
		// assignees" from "not shown" before calling case__assign / case__unassign.
		fmt.Fprintf(&b, "- Assignees (Slack user IDs): %s\n", orPlaceholder(strings.Join(c.AssigneeIDs, ", ")))
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
		b.WriteString("# Custom field schema (for materialize / create)\n")
		for _, f := range ws.FieldSchema.Fields {
			fmt.Fprintf(&b, "- %s (id=%s, type=%s)", f.Name, f.ID, f.Type)
			if f.Required {
				b.WriteString(" (required)")
			}
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
			// Date fields are persisted as RFC3339 strings; the validator rejects
			// a bare date like "2026-07-14". Spell the exact format out so the
			// planner emits a valid value on the first attempt.
			if f.Type == types.FieldTypeDate {
				b.WriteString(" format=RFC3339 (e.g. 2026-07-14T00:00:00Z)")
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

	// Workspace-specific instructions configured via TOML [case.prompts].create.
	// Applies to the ModeCreate flow (case initialization).
	if mode == ModeCreate && ws != nil && strings.TrimSpace(ws.CaseCreatePrompt) != "" {
		b.WriteString("# Workspace-specific instructions\n")
		b.WriteString(ws.CaseCreatePrompt)
		b.WriteString("\n")
	}

	// Host-supplied trigger context (e.g. reaction-initiated creation). Kept
	// separate from the workspace prompt: it is per-turn, not per-workspace, and
	// only meaningful while initializing a case.
	if mode == ModeCreate && strings.TrimSpace(createInstruction) != "" {
		b.WriteString("# Trigger context\n")
		b.WriteString(createInstruction)
		b.WriteString("\n")
	}

	return b.String()
}

// buildUserInput assembles the first user message handed to the planner. The
// system / delta conversation messages are prepended; the current mention is
// appended last (when it carries text). The mention is passed as a
// ConversationMessage so its author is rendered exactly like every other
// speaker — the agent needs the author's Slack user ID to satisfy a request
// like "assign me".
func buildUserInput(systemMessages, deltaMessages []ConversationMessage, mention ConversationMessage) string {
	var b strings.Builder
	if len(systemMessages) > 0 {
		b.WriteString("# Thread so far\n")
		writeMessages(&b, systemMessages, mention.Timestamp)
		b.WriteString("\n")
	}
	if len(deltaMessages) > 0 {
		b.WriteString("# New messages since last mention\n")
		writeMessages(&b, deltaMessages, mention.Timestamp)
		b.WriteString("\n")
	}
	if mention.Text != "" {
		b.WriteString("# Current mention\n")
		if speaker := speakerLabel(mention); speaker != "" {
			fmt.Fprintf(&b, "From: %s\n", speaker)
		}
		b.WriteString(mention.Text)
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
		fmt.Fprintf(b, "[%s] %s: %s\n", m.Timestamp, speakerLabel(m), m.Text)
	}
}

// speakerLabel renders a message author as "Display Name (U123)", degrading to
// whichever half is known. Both halves are emitted because the mention-mode
// system prompt tells the agent to resolve a named person to a Slack user ID
// before calling case__assign / case__unassign: a display name on its own
// cannot satisfy that, and there is no user-directory tool to look one up.
func speakerLabel(m ConversationMessage) string {
	switch {
	case m.UserName != "" && m.UserID != "":
		return fmt.Sprintf("%s (%s)", m.UserName, m.UserID)
	case m.UserName != "":
		return m.UserName
	default:
		return m.UserID
	}
}

func orPlaceholder(s string) string {
	if s == "" {
		return "(empty)"
	}
	return s
}
