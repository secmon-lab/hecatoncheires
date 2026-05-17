package types

import "fmt"

// CaseStatus represents the lifecycle state of a case.
//
// Lifecycle: DRAFT → OPEN → CLOSED (CLOSED can return to OPEN via reopenCase).
// DRAFT can only transition to OPEN (via SubmitDraft) or be deleted entirely
// (via DiscardDraft); it never goes directly to CLOSED.
type CaseStatus string

const (
	// CaseStatusDraft marks a case that has been saved from an in-progress
	// "Save as Draft" action on the Slack creation modal. Drafts are visible
	// only to their author, do not receive Slack channel binding or
	// notification side effects, and are excluded from default case listings.
	CaseStatusDraft  CaseStatus = "DRAFT"
	CaseStatusOpen   CaseStatus = "OPEN"
	CaseStatusClosed CaseStatus = "CLOSED"
)

// AllCaseStatuses returns all valid case statuses.
func AllCaseStatuses() []CaseStatus {
	return []CaseStatus{
		CaseStatusDraft,
		CaseStatusOpen,
		CaseStatusClosed,
	}
}

// IsValid checks if the case status is valid.
func (s CaseStatus) IsValid() bool {
	switch s {
	case CaseStatusDraft,
		CaseStatusOpen,
		CaseStatusClosed:
		return true
	default:
		return false
	}
}

// IsDraft reports whether the case is in the unsubmitted draft state.
func (s CaseStatus) IsDraft() bool {
	return s == CaseStatusDraft
}

// Normalize returns the status, treating empty as CaseStatusOpen for backward compatibility.
// Existing Firestore documents predate the DRAFT status and only ever stored
// OPEN/CLOSED (or the empty default that means OPEN), so this fallback never
// silently coerces a real DRAFT value.
func (s CaseStatus) Normalize() CaseStatus {
	if s == "" {
		return CaseStatusOpen
	}
	return s
}

// String returns the string representation of the case status.
func (s CaseStatus) String() string {
	return string(s)
}

// ParseCaseStatus parses a string into a CaseStatus.
func ParseCaseStatus(s string) (CaseStatus, error) {
	status := CaseStatus(s)
	if !status.IsValid() {
		return "", fmt.Errorf("invalid case status: %s", s)
	}
	return status, nil
}
