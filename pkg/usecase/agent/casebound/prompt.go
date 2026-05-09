package casebound

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

//go:embed prompts/system.md
var systemPromptTmpl string

var systemPromptTemplate = template.Must(template.New("casebound_system").Parse(systemPromptTmpl))

// promptField represents a case field for template rendering.
type promptField struct {
	Name  string
	Value any
}

// promptMessage represents a conversation message for template rendering.
type promptMessage struct {
	Timestamp   string
	DisplayName string
	Text        string
}

// promptAction represents a single action in the case-wide action list
// (rendered when the agent is NOT in an action-bound thread).
type promptAction struct {
	ID    int64
	Title string
}

// promptCurrentAction represents the action that the current Slack thread
// is bound to (when Session.ActionID != 0). The full set of fields is
// inlined so the LLM can answer questions about it without a tool call.
type promptCurrentAction struct {
	ID          int64
	Title       string
	Status      string
	StatusEmoji string
	Assignee    string
	Description string
	DueDate     string
}

// promptData holds all data for the casebound system prompt template.
type promptData struct {
	ChannelID     string
	Now           string
	Case          *model.Case
	Fields        []promptField
	CurrentAction *promptCurrentAction
	Actions       []promptAction
	Messages      []promptMessage
}

// buildSystemPrompt renders the casebound system prompt.
//
// When currentAction is non-nil, the agent is responding inside a Slack
// thread bound to that action. In that mode the case-wide actions list is
// suppressed — only the current action's detail is surfaced, to avoid
// drowning the LLM in unrelated work items. Otherwise the case-wide
// actions list is rendered as a title-only summary.
func buildSystemPrompt(c *model.Case, entry *model.WorkspaceEntry, channelID string, now time.Time, currentAction *model.Action, actions []*model.Action, messages []ConversationMessage) string {
	data := promptData{
		ChannelID: channelID,
		Now:       now.UTC().Format(time.RFC3339),
		Case:      c,
	}

	if entry != nil && entry.FieldSchema != nil && len(c.FieldValues) > 0 {
		fieldNames := make(map[string]string)
		for _, fd := range entry.FieldSchema.Fields {
			fieldNames[fd.ID] = fd.Name
		}
		for fieldID, fv := range c.FieldValues {
			name := fieldNames[fieldID]
			if name == "" {
				name = fieldID
			}
			data.Fields = append(data.Fields, promptField{Name: name, Value: fv.Value})
		}
	}

	if currentAction != nil {
		statusSet := model.DefaultActionStatusSet()
		if entry != nil && entry.ActionStatusSet != nil {
			statusSet = entry.ActionStatusSet
		}
		due := ""
		if currentAction.DueDate != nil {
			due = currentAction.DueDate.UTC().Format(time.RFC3339)
		}
		data.CurrentAction = &promptCurrentAction{
			ID:          currentAction.ID,
			Title:       currentAction.Title,
			Status:      currentAction.Status.String(),
			StatusEmoji: statusSet.Emoji(string(currentAction.Status)),
			Assignee:    currentAction.AssigneeID,
			Description: currentAction.Description,
			DueDate:     due,
		}
	} else {
		for _, a := range actions {
			data.Actions = append(data.Actions, promptAction{
				ID:    a.ID,
				Title: a.Title,
			})
		}
	}

	for _, msg := range messages {
		displayName := msg.UserName
		if displayName == "" {
			displayName = msg.UserID
		}
		data.Messages = append(data.Messages, promptMessage{
			Timestamp:   msg.Timestamp,
			DisplayName: displayName,
			Text:        msg.Text,
		})
	}

	var buf bytes.Buffer
	if err := systemPromptTemplate.Execute(&buf, data); err != nil {
		return fmt.Sprintf("You are an AI assistant. Case: %s", c.Title)
	}
	return buf.String()
}
