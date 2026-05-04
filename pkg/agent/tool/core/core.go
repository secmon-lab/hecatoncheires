// Package core contains gollem tools that operate on the case's domain state
// — actions, knowledge, and (in the assist flow) memory. Slack and Notion
// integrations live in their own tool packages (pkg/agent/tool/slack,
// pkg/agent/tool/notion); this package intentionally has no dependency on
// either external service.
package core

import (
	"context"
	"time"

	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// ActionCreator is the narrow surface of the ActionUseCase that the
// core__create_action tool depends on. Defined here so the tool can route
// creation through the unified usecase entry point (which handles Slack
// posting, ActionEvent recording, and access control) without taking a
// dependency on the entire usecase package — which would create an import
// cycle (pkg/usecase already imports pkg/agent/tool/core).
type ActionCreator interface {
	CreateAction(ctx context.Context, workspaceID string, caseID int64, title, description string, assigneeID string, slackMessageTS string, status types.ActionStatus, dueDate *time.Time) (*model.Action, error)
}

// Deps groups the dependencies the core tool factories need.
type Deps struct {
	Repo        interfaces.Repository
	WorkspaceID string
	CaseID      int64
	StatusSet   *model.ActionStatusSet
	EmbedClient interfaces.EmbedClient
	// ActionUC routes core__create_action through the unified usecase entry
	// point. Required when the agent flow expects newly created actions to
	// trigger Slack notifications, ActionEvent records, and any future
	// CreateAction-side effects.
	ActionUC ActionCreator
}

// New builds core tools for the agent mention use case: action management plus
// knowledge search/get. deps.StatusSet may be nil; it falls back to
// model.DefaultActionStatusSet().
func New(deps Deps) []gollem.Tool {
	statusSet := deps.StatusSet
	if statusSet == nil {
		statusSet = model.DefaultActionStatusSet()
	}

	return []gollem.Tool{
		&listActionsTool{repo: deps.Repo, workspaceID: deps.WorkspaceID, caseID: deps.CaseID},
		&getActionTool{repo: deps.Repo, workspaceID: deps.WorkspaceID},
		&createActionTool{actionUC: deps.ActionUC, workspaceID: deps.WorkspaceID, caseID: deps.CaseID, statusSet: statusSet},
		&updateActionTool{repo: deps.Repo, workspaceID: deps.WorkspaceID},
		&updateActionStatusTool{repo: deps.Repo, workspaceID: deps.WorkspaceID, statusSet: statusSet},
		&setActionAssigneeTool{repo: deps.Repo, workspaceID: deps.WorkspaceID},
		&searchKnowledgeTool{repo: deps.Repo, workspaceID: deps.WorkspaceID, embedClient: deps.EmbedClient},
		&getKnowledgeTool{repo: deps.Repo, workspaceID: deps.WorkspaceID},
	}
}

// NewForAssist builds tools for the assist use case. In addition to the base
// tools from New(), it includes the knowledge write tools (create/update) and
// the case memory CRUD + search tools.
func NewForAssist(deps Deps) []gollem.Tool {
	tools := New(deps)

	// Knowledge write tools
	tools = append(tools,
		&createKnowledgeTool{repo: deps.Repo, workspaceID: deps.WorkspaceID, caseID: deps.CaseID, embedClient: deps.EmbedClient},
		&updateKnowledgeTool{repo: deps.Repo, workspaceID: deps.WorkspaceID, embedClient: deps.EmbedClient},
	)

	// Memory tools
	tools = append(tools,
		&createMemoryTool{repo: deps.Repo, workspaceID: deps.WorkspaceID, caseID: deps.CaseID, embedClient: deps.EmbedClient},
		&deleteMemoryTool{repo: deps.Repo, workspaceID: deps.WorkspaceID, caseID: deps.CaseID},
		&searchMemoryTool{repo: deps.Repo, workspaceID: deps.WorkspaceID, caseID: deps.CaseID, embedClient: deps.EmbedClient},
		&listMemoriesTool{repo: deps.Repo, workspaceID: deps.WorkspaceID, caseID: deps.CaseID},
	)

	return tools
}
