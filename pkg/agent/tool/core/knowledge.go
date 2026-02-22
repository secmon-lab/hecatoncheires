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

// searchKnowledgeTool searches knowledge entries using vector similarity
type searchKnowledgeTool struct {
	repo        interfaces.Repository
	workspaceID string
	llmClient   gollem.LLMClient
}

func (t *searchKnowledgeTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__search_knowledge",
		Description: "Search knowledge entries using semantic (vector) similarity for the given query",
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

func (t *searchKnowledgeTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	tool.Update(ctx, fmt.Sprintf("Searching knowledge: %s", query))

	limit := 5
	if v, err := extractInt64(args, "limit"); err == nil && v > 0 {
		limit = int(v)
	}

	embeddings, err := t.llmClient.GenerateEmbedding(ctx, model.EmbeddingDimension, []string{query})
	if err != nil {
		return nil, goerr.Wrap(err, "failed to generate embedding for search query",
			goerr.V("query", query),
		)
	}
	if len(embeddings) == 0 || len(embeddings[0]) == 0 {
		return nil, fmt.Errorf("embedding generation returned empty result")
	}

	// Convert float64 embedding to float32
	embedding64 := embeddings[0]
	embedding32 := make([]float32, len(embedding64))
	for i, v := range embedding64 {
		embedding32[i] = float32(v)
	}

	results, err := t.repo.Knowledge().FindByEmbedding(ctx, t.workspaceID, embedding32, limit)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to search knowledge by embedding",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("limit", limit),
		)
	}

	items := make([]map[string]any, len(results))
	for i, k := range results {
		items[i] = map[string]any{
			"id":      string(k.ID),
			"title":   k.Title,
			"summary": k.Summary,
		}
	}
	return map[string]any{"knowledges": items}, nil
}

// getKnowledgeTool retrieves a knowledge entry by ID
type getKnowledgeTool struct {
	repo        interfaces.Repository
	workspaceID string
}

func (t *getKnowledgeTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__get_knowledge",
		Description: "Get full details of a knowledge entry by its ID",
		Parameters: map[string]*gollem.Parameter{
			"knowledge_id": {
				Type:        gollem.TypeString,
				Description: "The ID of the knowledge entry to retrieve",
				Required:    true,
			},
		},
	}
}

func (t *getKnowledgeTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	knowledgeID, _ := args["knowledge_id"].(string)
	if knowledgeID == "" {
		return nil, fmt.Errorf("knowledge_id is required")
	}

	tool.Update(ctx, fmt.Sprintf("Getting knowledge %s...", knowledgeID))

	k, err := t.repo.Knowledge().Get(ctx, t.workspaceID, model.KnowledgeID(knowledgeID))
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get knowledge",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("knowledgeID", knowledgeID),
		)
	}
	if k == nil {
		return nil, fmt.Errorf("knowledge not found: id=%s", knowledgeID)
	}

	return map[string]any{
		"id":          string(k.ID),
		"case_id":     k.CaseID,
		"source_id":   string(k.SourceID),
		"source_urls": k.SourceURLs,
		"title":       k.Title,
		"summary":     k.Summary,
		"sourced_at":  k.SourcedAt.String(),
		"created_at":  k.CreatedAt.String(),
		"updated_at":  k.UpdatedAt.String(),
	}, nil
}
