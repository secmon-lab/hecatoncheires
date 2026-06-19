package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// knowledgeRepository stores knowledge entries indexed by workspaceID -> id.
type knowledgeRepository struct {
	mu   sync.RWMutex
	data map[string]map[model.KnowledgeID]*model.Knowledge
}

func newKnowledgeRepository() *knowledgeRepository {
	return &knowledgeRepository{
		data: make(map[string]map[model.KnowledgeID]*model.Knowledge),
	}
}

func (r *knowledgeRepository) ensureWorkspace(workspaceID string) {
	if _, ok := r.data[workspaceID]; !ok {
		r.data[workspaceID] = make(map[model.KnowledgeID]*model.Knowledge)
	}
}

// copyKnowledge creates a full deep copy so mutations by the caller after
// Create/Update cannot silently alter the stored value.
func copyKnowledge(k *model.Knowledge) *model.Knowledge {
	copied := &model.Knowledge{
		ID:          k.ID,
		WorkspaceID: k.WorkspaceID,
		Title:       k.Title,
		Claim:       k.Claim,
		CreatorID:   k.CreatorID,
		CreatedAt:   k.CreatedAt,
		UpdatedAt:   k.UpdatedAt,
	}
	if k.Tags != nil {
		copied.Tags = make([]string, len(k.Tags))
		copy(copied.Tags, k.Tags)
	}
	if k.Embedding != nil {
		copied.Embedding = make([]float64, len(k.Embedding))
		copy(copied.Embedding, k.Embedding)
	}
	return copied
}

// knowledgeHasAllTags reports whether k carries every tag in want (AND).
func knowledgeHasAllTags(k *model.Knowledge, want []string) bool {
	if len(want) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(k.Tags))
	for _, t := range k.Tags {
		set[t] = struct{}{}
	}
	for _, w := range want {
		if _, ok := set[w]; !ok {
			return false
		}
	}
	return true
}

func (r *knowledgeRepository) Create(ctx context.Context, workspaceID string, knowledge *model.Knowledge) (*model.Knowledge, error) {
	if err := knowledge.Validate(); err != nil {
		return nil, goerr.Wrap(err, "knowledge validation failed before create")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.ensureWorkspace(workspaceID)
	stored := copyKnowledge(knowledge)
	r.data[workspaceID][knowledge.ID] = stored
	return copyKnowledge(stored), nil
}

func (r *knowledgeRepository) Get(ctx context.Context, workspaceID string, id model.KnowledgeID) (*model.Knowledge, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, ok := r.data[workspaceID]
	if !ok {
		return nil, goerr.Wrap(ErrNotFound, "knowledge not found",
			goerr.V("knowledge_id", id), goerr.V("workspace_id", workspaceID))
	}
	k, ok := ws[id]
	if !ok {
		return nil, goerr.Wrap(ErrNotFound, "knowledge not found",
			goerr.V("knowledge_id", id), goerr.V("workspace_id", workspaceID))
	}
	return copyKnowledge(k), nil
}

func (r *knowledgeRepository) List(ctx context.Context, workspaceID string, opts interfaces.KnowledgeListOptions) ([]*model.Knowledge, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, ok := r.data[workspaceID]
	if !ok {
		return []*model.Knowledge{}, nil
	}

	items := make([]*model.Knowledge, 0, len(ws))
	for _, k := range ws {
		if !knowledgeHasAllTags(k, opts.Tags) {
			continue
		}
		items = append(items, copyKnowledge(k))
	}

	// Sort by CreatedAt ascending to mirror the Firestore implementation,
	// tie-breaking on ID (UUID v7 is lexicographically time-ordered).
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})

	return items, nil
}

func (r *knowledgeRepository) Update(ctx context.Context, workspaceID string, knowledge *model.Knowledge) (*model.Knowledge, error) {
	if err := knowledge.Validate(); err != nil {
		return nil, goerr.Wrap(err, "knowledge validation failed before update")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	ws, ok := r.data[workspaceID]
	if !ok {
		return nil, goerr.Wrap(ErrNotFound, "knowledge not found",
			goerr.V("knowledge_id", knowledge.ID), goerr.V("workspace_id", workspaceID))
	}
	if _, ok := ws[knowledge.ID]; !ok {
		return nil, goerr.Wrap(ErrNotFound, "knowledge not found",
			goerr.V("knowledge_id", knowledge.ID), goerr.V("workspace_id", workspaceID))
	}

	stored := copyKnowledge(knowledge)
	r.data[workspaceID][knowledge.ID] = stored
	return copyKnowledge(stored), nil
}

func (r *knowledgeRepository) Delete(ctx context.Context, workspaceID string, id model.KnowledgeID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ws, ok := r.data[workspaceID]
	if !ok {
		return goerr.Wrap(ErrNotFound, "knowledge not found",
			goerr.V("knowledge_id", id), goerr.V("workspace_id", workspaceID))
	}
	if _, ok := ws[id]; !ok {
		return goerr.Wrap(ErrNotFound, "knowledge not found",
			goerr.V("knowledge_id", id), goerr.V("workspace_id", workspaceID))
	}
	delete(ws, id)
	return nil
}
