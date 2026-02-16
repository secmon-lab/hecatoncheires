package types

import "fmt"

// ActionStatus represents the status of an action in a case
type ActionStatus string

const (
	ActionStatusBacklog    ActionStatus = "BACKLOG"
	ActionStatusTodo       ActionStatus = "TODO"
	ActionStatusInProgress ActionStatus = "IN_PROGRESS"
	ActionStatusBlocked    ActionStatus = "BLOCKED"
	ActionStatusCompleted  ActionStatus = "COMPLETED"
)

// AllActionStatuses returns all valid action statuses
func AllActionStatuses() []ActionStatus {
	return []ActionStatus{
		ActionStatusBacklog,
		ActionStatusTodo,
		ActionStatusInProgress,
		ActionStatusBlocked,
		ActionStatusCompleted,
	}
}

// IsValid checks if the action status is valid
func (s ActionStatus) IsValid() bool {
	switch s {
	case ActionStatusBacklog,
		ActionStatusTodo,
		ActionStatusInProgress,
		ActionStatusBlocked,
		ActionStatusCompleted:
		return true
	default:
		return false
	}
}

// String returns the string representation of the action status
func (s ActionStatus) String() string {
	return string(s)
}

// ParseActionStatus parses a string into an ActionStatus
func ParseActionStatus(s string) (ActionStatus, error) {
	status := ActionStatus(s)
	if !status.IsValid() {
		return "", fmt.Errorf("invalid action status: %s", s)
	}
	return status, nil
}
