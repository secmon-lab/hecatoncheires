package model

// MyOpenCase is one row of the cross-workspace "my open cases" home aggregation.
// It carries the workspace identity alongside the Case because the GraphQL Case
// type has no workspaceId field (a Case is always workspace-scoped elsewhere).
type MyOpenCase struct {
	WorkspaceID   string
	WorkspaceName string
	Case          *Case
	// Stalled is true when the Case is open but has not been updated within the
	// configured stale window. Always false when the window is disabled.
	Stalled bool
}

// MyDueAction is one row of the cross-workspace "my incomplete actions"
// home aggregation. CaseID / CaseTitle are pre-expanded from the parent Case so
// the frontend need not resolve the parent (avoids an N+1 per row).
type MyDueAction struct {
	WorkspaceID   string
	WorkspaceName string
	Action        *Action
	CaseID        int64
	CaseTitle     string
}
