package types

// ActionEventKind enumerates the kinds of structural changes that can be
// recorded against an Action. Used by ActionEvent to render an audit /
// change-history feed in the WebUI.
type ActionEventKind string

const (
	ActionEventCreated          ActionEventKind = "CREATED"
	ActionEventTitleChanged     ActionEventKind = "TITLE_CHANGED"
	ActionEventStatusChanged    ActionEventKind = "STATUS_CHANGED"
	ActionEventAssigneeChanged  ActionEventKind = "ASSIGNEE_CHANGED"
	ActionEventArchived         ActionEventKind = "ARCHIVED"
	ActionEventUnarchived       ActionEventKind = "UNARCHIVED"
	ActionEventStepAdded        ActionEventKind = "STEP_ADDED"
	ActionEventStepRemoved      ActionEventKind = "STEP_REMOVED"
	ActionEventStepDone         ActionEventKind = "STEP_DONE"
	ActionEventStepReopened     ActionEventKind = "STEP_REOPENED"
	ActionEventStepTitleChanged ActionEventKind = "STEP_TITLE_CHANGED"
)

func (k ActionEventKind) IsValid() bool {
	switch k {
	case ActionEventCreated,
		ActionEventTitleChanged,
		ActionEventStatusChanged,
		ActionEventAssigneeChanged,
		ActionEventArchived,
		ActionEventUnarchived,
		ActionEventStepAdded,
		ActionEventStepRemoved,
		ActionEventStepDone,
		ActionEventStepReopened,
		ActionEventStepTitleChanged:
		return true
	}
	return false
}

func (k ActionEventKind) String() string { return string(k) }
