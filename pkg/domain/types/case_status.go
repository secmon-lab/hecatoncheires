package types

import "fmt"

// CaseStatus represents the status of a case
type CaseStatus string

const (
	CaseStatusOpen   CaseStatus = "OPEN"
	CaseStatusClosed CaseStatus = "CLOSED"
)

// AllCaseStatuses returns all valid case statuses
func AllCaseStatuses() []CaseStatus {
	return []CaseStatus{
		CaseStatusOpen,
		CaseStatusClosed,
	}
}

// IsValid checks if the case status is valid
func (s CaseStatus) IsValid() bool {
	switch s {
	case CaseStatusOpen,
		CaseStatusClosed:
		return true
	default:
		return false
	}
}

// Normalize returns the status, treating empty as CaseStatusOpen for backward compatibility.
func (s CaseStatus) Normalize() CaseStatus {
	if s == "" {
		return CaseStatusOpen
	}
	return s
}

// String returns the string representation of the case status
func (s CaseStatus) String() string {
	return string(s)
}

// ParseCaseStatus parses a string into a CaseStatus
func ParseCaseStatus(s string) (CaseStatus, error) {
	status := CaseStatus(s)
	if !status.IsValid() {
		return "", fmt.Errorf("invalid case status: %s", s)
	}
	return status, nil
}
