package core

import (
	"context"
	"fmt"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// createMemoryTool creates a new memory entry with auto-generated embedding
type createMemoryTool struct {
	repo        interfaces.Repository
	workspaceID string
	caseID      int64
	llmClient   gollem.LLMClient
}

func (t *createMemoryTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__create_memory",
		Description: "Create a new memory entry. Use memories to track facts, observations, pending decisions, and follow-up items across assist sessions. An embedding is automatically generated from the claim for similarity search.",
		Parameters: map[string]*gollem.Parameter{
			"claim": {
				Type:        gollem.TypeString,
				Description: "The fact, observation, or note to remember",
				Required:    true,
			},
		},
	}
}

func (t *createMemoryTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	claim, _ := args["claim"].(string)
	if claim == "" {
		return nil, fmt.Errorf("claim is required")
	}

	tool.Update(ctx, "Creating memory...")

	embedding, err := generateMemoryEmbedding(ctx, t.llmClient, claim)
	if err != nil {
		return nil, err
	}

	mem := &model.Memory{
		CaseID:    t.caseID,
		Claim:     claim,
		Embedding: embedding,
	}

	created, err := t.repo.Memory().Create(ctx, t.workspaceID, t.caseID, mem)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create memory",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("caseID", t.caseID),
		)
	}

	return map[string]any{
		"id":    string(created.ID),
		"claim": created.Claim,
	}, nil
}

// deleteMemoryTool deletes a memory entry by ID
type deleteMemoryTool struct {
	repo        interfaces.Repository
	workspaceID string
	caseID      int64
}

func (t *deleteMemoryTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__delete_memory",
		Description: "Delete a memory entry by its ID. Use this to remove outdated or irrelevant memories.",
		Parameters: map[string]*gollem.Parameter{
			"memory_id": {
				Type:        gollem.TypeString,
				Description: "The ID of the memory entry to delete",
				Required:    true,
			},
		},
	}
}

func (t *deleteMemoryTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	memoryID, _ := args["memory_id"].(string)
	if memoryID == "" {
		return nil, fmt.Errorf("memory_id is required")
	}

	tool.Update(ctx, fmt.Sprintf("Deleting memory %s...", memoryID))

	if err := t.repo.Memory().Delete(ctx, t.workspaceID, t.caseID, model.MemoryID(memoryID)); err != nil {
		return nil, goerr.Wrap(err, "failed to delete memory",
			goerr.V("memoryID", memoryID),
		)
	}

	return map[string]any{"deleted": true}, nil
}

// searchMemoryTool searches memories using vector similarity
type searchMemoryTool struct {
	repo        interfaces.Repository
	workspaceID string
	caseID      int64
	llmClient   gollem.LLMClient
}

func (t *searchMemoryTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__search_memory",
		Description: "Search memory entries using semantic (vector) similarity for the given query",
		Parameters: map[string]*gollem.Parameter{
			"query": {
				Type:        gollem.TypeString,
				Description: "Search query text",
				Required:    true,
			},
			"limit": {
				Type:        gollem.TypeInteger,
				Description: "Maximum number of results to return (default: 5)",
				Required:    false,
			},
		},
	}
}

func (t *searchMemoryTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	tool.Update(ctx, fmt.Sprintf("Searching memories: %s", query))

	limit := 5
	if v, err := extractInt64(args, "limit"); err == nil && v > 0 {
		limit = int(v)
	}

	embedding, err := generateMemoryEmbedding(ctx, t.llmClient, query)
	if err != nil {
		return nil, err
	}

	results, err := t.repo.Memory().FindByEmbedding(ctx, t.workspaceID, t.caseID, embedding, limit)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to search memories by embedding",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("caseID", t.caseID),
		)
	}

	items := make([]map[string]any, len(results))
	for i, m := range results {
		items[i] = map[string]any{
			"id":         string(m.ID),
			"claim":      m.Claim,
			"created_at": m.CreatedAt.String(),
		}
	}
	return map[string]any{"memories": items}, nil
}

// listMemoriesTool lists all memories for the current case
type listMemoriesTool struct {
	repo        interfaces.Repository
	workspaceID string
	caseID      int64
}

func (t *listMemoriesTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__list_memories",
		Description: "List all memory entries for the current case, sorted by creation date (newest first)",
		Parameters:  map[string]*gollem.Parameter{},
	}
}

func (t *listMemoriesTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Listing memories...")

	memories, err := t.repo.Memory().List(ctx, t.workspaceID, t.caseID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list memories",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("caseID", t.caseID),
		)
	}

	items := make([]map[string]any, len(memories))
	for i, m := range memories {
		items[i] = map[string]any{
			"id":         string(m.ID),
			"claim":      m.Claim,
			"created_at": m.CreatedAt.String(),
		}
	}
	return map[string]any{"memories": items, "count": len(items)}, nil
}

// generateMemoryEmbedding generates a float32 embedding from claim text
func generateMemoryEmbedding(ctx context.Context, llmClient gollem.LLMClient, text string) ([]float32, error) {
	embeddings, err := llmClient.GenerateEmbedding(ctx, model.EmbeddingDimension, []string{text})
	if err != nil {
		return nil, goerr.Wrap(err, "failed to generate embedding for memory")
	}
	if len(embeddings) == 0 || len(embeddings[0]) == 0 {
		return nil, fmt.Errorf("embedding generation returned empty result")
	}

	embedding64 := embeddings[0]
	embedding32 := make([]float32, len(embedding64))
	for i, v := range embedding64 {
		embedding32[i] = float32(v)
	}
	return embedding32, nil
}
