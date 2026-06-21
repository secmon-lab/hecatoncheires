package usecase

import (
	"context"

	knowledgetool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/knowledge"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// knowledgeToolAdapter wraps a *KnowledgeUseCase so the knowledge agent tools
// see it through the narrow knowledgetool.KnowledgeAccessor / KnowledgeMutator
// surfaces, without the tool package importing pkg/usecase (which would create
// an import cycle).
type knowledgeToolAdapter struct {
	uc *KnowledgeUseCase
}

// NewKnowledgeToolAccessor returns the read surface backed by the use case.
// Returns nil when uc is nil so the tool wiring can detect an unconfigured
// knowledge feature.
func NewKnowledgeToolAccessor(uc *KnowledgeUseCase) knowledgetool.KnowledgeAccessor {
	if uc == nil {
		return nil
	}
	return &knowledgeToolAdapter{uc: uc}
}

// NewKnowledgeToolMutator returns the write surface backed by the use case.
// Returns nil when uc is nil.
func NewKnowledgeToolMutator(uc *KnowledgeUseCase) knowledgetool.KnowledgeMutator {
	if uc == nil {
		return nil
	}
	return &knowledgeToolAdapter{uc: uc}
}

func (a *knowledgeToolAdapter) SearchKnowledge(ctx context.Context, workspaceID, query string, tags []string, limit int) ([]*model.Knowledge, error) {
	return a.uc.SearchKnowledge(ctx, workspaceID, SearchKnowledgeInput{Query: query, Tags: tags, Limit: limit})
}

func (a *knowledgeToolAdapter) GetKnowledge(ctx context.Context, workspaceID string, id model.KnowledgeID) (*model.Knowledge, error) {
	return a.uc.GetKnowledge(ctx, workspaceID, id)
}

func (a *knowledgeToolAdapter) ListTags(ctx context.Context, workspaceID string) ([]string, error) {
	return a.uc.ListTags(ctx, workspaceID)
}

func (a *knowledgeToolAdapter) CreateKnowledge(ctx context.Context, workspaceID, title, claim string, tags []string) (*model.Knowledge, error) {
	return a.uc.CreateKnowledge(ctx, workspaceID, CreateKnowledgeInput{Title: title, Claim: claim, Tags: tags})
}

func (a *knowledgeToolAdapter) UpdateKnowledge(ctx context.Context, workspaceID string, id model.KnowledgeID, title, claim *string, tags *[]string) (*model.Knowledge, error) {
	return a.uc.UpdateKnowledge(ctx, workspaceID, UpdateKnowledgeInput{ID: id, Title: title, Claim: claim, Tags: tags})
}

// ensure the adapter satisfies both surfaces.
var (
	_ knowledgetool.KnowledgeAccessor = (*knowledgeToolAdapter)(nil)
	_ knowledgetool.KnowledgeMutator  = (*knowledgeToolAdapter)(nil)
)
