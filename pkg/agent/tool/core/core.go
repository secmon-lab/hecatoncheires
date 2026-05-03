package core

import (
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slackService "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

// New builds core tools for the agent mention use case.
// It creates tools for managing actions and searching/retrieving knowledge
// associated with the given case in the given workspace.
//
// statusSet is the workspace-scoped Action status configuration; pass
// model.DefaultActionStatusSet() if the workspace has no custom statuses.
func New(repo interfaces.Repository, workspaceID string, caseID int64, statusSet *model.ActionStatusSet, llmClient gollem.LLMClient) []gollem.Tool {
	if statusSet == nil {
		statusSet = model.DefaultActionStatusSet()
	}
	return []gollem.Tool{
		&listActionsTool{repo: repo, workspaceID: workspaceID, caseID: caseID},
		&getActionTool{repo: repo, workspaceID: workspaceID},
		&createActionTool{repo: repo, workspaceID: workspaceID, caseID: caseID, statusSet: statusSet},
		&updateActionTool{repo: repo, workspaceID: workspaceID},
		&updateActionStatusTool{repo: repo, workspaceID: workspaceID, statusSet: statusSet},
		&setActionAssigneeTool{repo: repo, workspaceID: workspaceID},
		&searchKnowledgeTool{repo: repo, workspaceID: workspaceID, llmClient: llmClient},
		&getKnowledgeTool{repo: repo, workspaceID: workspaceID},
	}
}

// NewForAssist builds tools for the assist use case.
// In addition to the base tools from New(), it includes:
// - Knowledge write tools (create/update)
// - Slack message posting tool
// - Memory CRUD + search tools
func NewForAssist(repo interfaces.Repository, workspaceID string, caseID int64, statusSet *model.ActionStatusSet, llmClient gollem.LLMClient, slack slackService.Service, channelID string) []gollem.Tool {
	tools := New(repo, workspaceID, caseID, statusSet, llmClient)

	// Knowledge write tools
	tools = append(tools,
		&createKnowledgeTool{repo: repo, workspaceID: workspaceID, caseID: caseID, llmClient: llmClient},
		&updateKnowledgeTool{repo: repo, workspaceID: workspaceID, llmClient: llmClient},
	)

	// Slack message posting tool
	tools = append(tools,
		&postMessageTool{slack: slack, channelID: channelID},
	)

	// Memory tools
	tools = append(tools,
		&createMemoryTool{repo: repo, workspaceID: workspaceID, caseID: caseID, llmClient: llmClient},
		&deleteMemoryTool{repo: repo, workspaceID: workspaceID, caseID: caseID},
		&searchMemoryTool{repo: repo, workspaceID: workspaceID, caseID: caseID, llmClient: llmClient},
		&listMemoriesTool{repo: repo, workspaceID: workspaceID, caseID: caseID},
	)

	return tools
}
