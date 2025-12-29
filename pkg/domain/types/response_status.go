package types

import "fmt"

// ResponseStatus represents the status of a response to a risk
type ResponseStatus string

const (
	ResponseStatusBacklog    ResponseStatus = "backlog"
	ResponseStatusTodo       ResponseStatus = "todo"
	ResponseStatusInProgress ResponseStatus = "in-progress"
	ResponseStatusBlocked    ResponseStatus = "blocked"
	ResponseStatusCompleted  ResponseStatus = "completed"
	ResponseStatusAbandoned  ResponseStatus = "abandoned"
)

// AllResponseStatuses returns all valid response statuses
func AllResponseStatuses() []ResponseStatus {
	return []ResponseStatus{
		ResponseStatusBacklog,
		ResponseStatusTodo,
		ResponseStatusInProgress,
		ResponseStatusBlocked,
		ResponseStatusCompleted,
		ResponseStatusAbandoned,
	}
}

// IsValid checks if the response status is valid
func (s ResponseStatus) IsValid() bool {
	switch s {
	case ResponseStatusBacklog,
		ResponseStatusTodo,
		ResponseStatusInProgress,
		ResponseStatusBlocked,
		ResponseStatusCompleted,
		ResponseStatusAbandoned:
		return true
	default:
		return false
	}
}

// String returns the string representation of the response status
func (s ResponseStatus) String() string {
	return string(s)
}

// ParseResponseStatus parses a string into a ResponseStatus
func ParseResponseStatus(s string) (ResponseStatus, error) {
	status := ResponseStatus(s)
	if !status.IsValid() {
		return "", fmt.Errorf("invalid response status: %s", s)
	}
	return status, nil
}
