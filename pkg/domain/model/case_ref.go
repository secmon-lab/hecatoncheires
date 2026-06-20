package model

import "github.com/secmon-lab/hecatoncheires/pkg/domain/types"

// CaseRef is the summary projection of a Case used by case_ref fields:
// the picker, the agent search tool, and value-label resolution. It carries
// only the fields safe to surface for a referenceable (non-private) Case and
// deliberately omits Description / Reporter / Assignees / FieldValues so a
// summary can never leak more than a title.
type CaseRef struct {
	ID          int64
	Title       string
	Status      types.CaseStatus
	WorkspaceID string
}

// NewCaseRef builds a CaseRef summary for a Case in the given workspace.
func NewCaseRef(workspaceID string, c *Case) CaseRef {
	return CaseRef{
		ID:          c.ID,
		Title:       c.Title,
		Status:      c.Status,
		WorkspaceID: workspaceID,
	}
}
