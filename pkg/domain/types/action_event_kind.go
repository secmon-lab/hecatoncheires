package types

// ActionEventKind enumerates the kinds of structural changes that can be
// recorded against an Action. Used by ActionEvent to render an audit /
// change-history feed in the WebUI.
type ActionEventKind string

const (
	ActionEventCreated         ActionEventKind = "CREATED"
	ActionEventTitleChanged    ActionEventKind = "TITLE_CHANGED"
	ActionEventStatusChanged   ActionEventKind = "STATUS_CHANGED"
	ActionEventAssigneeChanged ActionEventKind = "ASSIGNEE_CHANGED"
)

func (k ActionEventKind) IsValid() bool {
	switch k {
	case ActionEventCreated,
		ActionEventTitleChanged,
		ActionEventStatusChanged,
		ActionEventAssigneeChanged:
		return true
	}
	return false
}

func (k ActionEventKind) String() string { return string(k) }
