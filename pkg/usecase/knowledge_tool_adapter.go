package usecase

import (
	"context"

	knowledgetool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/knowledge"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// knowledgeToolAdapter wraps the knowledge + tag use cases so the knowledge
// agent tools see them through the narrow knowledgetool.KnowledgeAccessor /
// KnowledgeMutator surfaces, without the tool package importing pkg/usecase
// (which would create an import cycle).
type knowledgeToolAdapter struct {
	uc    *KnowledgeUseCase
	tagUC *TagUseCase
}

// NewKnowledgeToolAccessor returns the read surface backed by the use cases.
// Returns nil when either use case is nil so the tool wiring can detect an
// unconfigured knowledge feature.
func NewKnowledgeToolAccessor(uc *KnowledgeUseCase, tagUC *TagUseCase) knowledgetool.KnowledgeAccessor {
	if uc == nil || tagUC == nil {
		return nil
	}
	return &knowledgeToolAdapter{uc: uc, tagUC: tagUC}
}

// NewKnowledgeToolMutator returns the write surface backed by the use cases.
// Returns nil when either use case is nil.
func NewKnowledgeToolMutator(uc *KnowledgeUseCase, tagUC *TagUseCase) knowledgetool.KnowledgeMutator {
	if uc == nil || tagUC == nil {
		return nil
	}
	return &knowledgeToolAdapter{uc: uc, tagUC: tagUC}
}

func (a *knowledgeToolAdapter) SearchKnowledge(ctx context.Context, workspaceID, query string, tagIDs []model.TagID, limit int) ([]*model.Knowledge, error) {
	return a.uc.SearchKnowledge(ctx, workspaceID, SearchKnowledgeInput{Query: query, TagIDs: tagIDs, Limit: limit})
}

func (a *knowledgeToolAdapter) GetKnowledge(ctx context.Context, workspaceID string, id model.KnowledgeID) (*model.Knowledge, error) {
	return a.uc.GetKnowledge(ctx, workspaceID, id)
}

func (a *knowledgeToolAdapter) ListTags(ctx context.Context, workspaceID string) ([]*model.Tag, error) {
	return a.tagUC.ListTags(ctx, workspaceID)
}

func (a *knowledgeToolAdapter) CreateTag(ctx context.Context, workspaceID, name string) (*model.Tag, error) {
	return a.tagUC.CreateTag(ctx, workspaceID, name)
}

func (a *knowledgeToolAdapter) UpdateTag(ctx context.Context, workspaceID string, id model.TagID, name string) (*model.Tag, error) {
	return a.tagUC.UpdateTag(ctx, workspaceID, id, name)
}

func (a *knowledgeToolAdapter) DeleteTag(ctx context.Context, workspaceID string, id model.TagID) error {
	return a.tagUC.DeleteTag(ctx, workspaceID, id)
}

func (a *knowledgeToolAdapter) CreateKnowledge(ctx context.Context, workspaceID, title, claim string, tagIDs []model.TagID) (*model.Knowledge, error) {
	return a.uc.CreateKnowledge(ctx, workspaceID, CreateKnowledgeInput{Title: title, Claim: claim, TagIDs: tagIDs})
}

func (a *knowledgeToolAdapter) UpdateKnowledge(ctx context.Context, workspaceID string, id model.KnowledgeID, title, claim *string, tagIDs *[]model.TagID) (*model.Knowledge, error) {
	return a.uc.UpdateKnowledge(ctx, workspaceID, UpdateKnowledgeInput{ID: id, Title: title, Claim: claim, TagIDs: tagIDs})
}

// ensure the adapter satisfies both surfaces.
var (
	_ knowledgetool.KnowledgeAccessor = (*knowledgeToolAdapter)(nil)
	_ knowledgetool.KnowledgeMutator  = (*knowledgeToolAdapter)(nil)
)
