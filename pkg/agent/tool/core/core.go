// Package core contains gollem tools that operate on the case's domain state
// — currently actions. Slack and Notion integrations live in their own tool
// packages (pkg/agent/tool/slack, pkg/agent/tool/notion); this package
// intentionally has no dependency on either external service.
package core

import (
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// Deps groups the dependencies the core tool factories need.
type Deps struct {
	Repo        interfaces.Repository
	WorkspaceID string
	CaseID      int64
	StatusSet   *model.ActionStatusSet
}

// New builds core tools for the agent mention use case: action management.
// deps.StatusSet may be nil; it falls back to model.DefaultActionStatusSet().
func New(deps Deps) []gollem.Tool {
	statusSet := deps.StatusSet
	if statusSet == nil {
		statusSet = model.DefaultActionStatusSet()
	}

	return []gollem.Tool{
		&listActionsTool{repo: deps.Repo, workspaceID: deps.WorkspaceID, caseID: deps.CaseID},
		&getActionTool{repo: deps.Repo, workspaceID: deps.WorkspaceID},
		&createActionTool{repo: deps.Repo, workspaceID: deps.WorkspaceID, caseID: deps.CaseID, statusSet: statusSet},
		&updateActionTool{repo: deps.Repo, workspaceID: deps.WorkspaceID},
		&updateActionStatusTool{repo: deps.Repo, workspaceID: deps.WorkspaceID, statusSet: statusSet},
		&setActionAssigneeTool{repo: deps.Repo, workspaceID: deps.WorkspaceID},
	}
}

// NewForAssist builds tools for the assist use case. Currently identical to
// New(); kept as a separate factory so future assist-only tools can be added
// without touching the mention flow.
func NewForAssist(deps Deps) []gollem.Tool {
	return New(deps)
}
