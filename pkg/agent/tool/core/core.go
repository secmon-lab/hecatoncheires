package core

import (
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
)

// New builds all core tools for the agent.
// It creates tools for managing actions and searching/retrieving knowledge
// associated with the given case in the given workspace.
func New(repo interfaces.Repository, workspaceID string, caseID int64, llmClient gollem.LLMClient) []gollem.Tool {
	return []gollem.Tool{
		&listActionsTool{repo: repo, workspaceID: workspaceID, caseID: caseID},
		&getActionTool{repo: repo, workspaceID: workspaceID},
		&createActionTool{repo: repo, workspaceID: workspaceID, caseID: caseID},
		&updateActionTool{repo: repo, workspaceID: workspaceID},
		&updateActionStatusTool{repo: repo, workspaceID: workspaceID},
		&addActionAssigneeTool{repo: repo, workspaceID: workspaceID},
		&removeActionAssigneeTool{repo: repo, workspaceID: workspaceID},
		&searchKnowledgeTool{repo: repo, workspaceID: workspaceID, llmClient: llmClient},
		&getKnowledgeTool{repo: repo, workspaceID: workspaceID},
	}
}
