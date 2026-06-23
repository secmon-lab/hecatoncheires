package firestore

import (
	"context"
	"sort"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type knowledgeRepository struct {
	client *firestore.Client
}

func newKnowledgeRepository(client *firestore.Client) *knowledgeRepository {
	return &knowledgeRepository{
		client: client,
	}
}

// knowledgesCollection returns the subcollection ref for knowledge entries under
// a workspace. Path: workspaces/{workspaceID}/knowledges
func (r *knowledgeRepository) knowledgesCollection(workspaceID string) *firestore.CollectionRef {
	return r.client.Collection("workspaces").Doc(workspaceID).Collection("knowledges")
}

// knowledgeHasAllTags reports whether k references every tag id in want (AND).
func knowledgeHasAllTags(k *model.Knowledge, want []model.TagID) bool {
	if len(want) == 0 {
		return true
	}
	set := make(map[model.TagID]struct{}, len(k.TagIDs))
	for _, id := range k.TagIDs {
		set[id] = struct{}{}
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

	docRef := r.knowledgesCollection(workspaceID).Doc(string(knowledge.ID))
	if _, err := docRef.Set(ctx, knowledge); err != nil {
		return nil, goerr.Wrap(err, "failed to create knowledge",
			goerr.V("knowledge_id", knowledge.ID), goerr.V("workspace_id", workspaceID))
	}

	return knowledge, nil
}

func (r *knowledgeRepository) Get(ctx context.Context, workspaceID string, id model.KnowledgeID) (*model.Knowledge, error) {
	docSnap, err := r.knowledgesCollection(workspaceID).Doc(string(id)).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "knowledge not found",
				goerr.V("knowledge_id", id), goerr.V("workspace_id", workspaceID))
		}
		return nil, goerr.Wrap(err, "failed to get knowledge",
			goerr.V("knowledge_id", id), goerr.V("workspace_id", workspaceID))
	}

	var k model.Knowledge
	if err := docSnap.DataTo(&k); err != nil {
		return nil, goerr.Wrap(err, "failed to decode knowledge", goerr.V("doc_id", docSnap.Ref.ID))
	}

	return &k, nil
}

func (r *knowledgeRepository) List(ctx context.Context, workspaceID string, opts interfaces.KnowledgeListOptions) ([]*model.Knowledge, error) {
	iter := r.knowledgesCollection(workspaceID).Documents(ctx)
	defer iter.Stop()

	items := make([]*model.Knowledge, 0)
	for {
		docSnap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate knowledge", goerr.V("workspace_id", workspaceID))
		}

		var k model.Knowledge
		if err := docSnap.DataTo(&k); err != nil {
			return nil, goerr.Wrap(err, "failed to decode knowledge", goerr.V("doc_id", docSnap.Ref.ID))
		}

		// Tag AND filter is applied in memory: storing tag ids as an array and
		// filtering here avoids a Firestore composite index entirely.
		if !knowledgeHasAllTags(&k, opts.TagIDs) {
			continue
		}

		items = append(items, &k)
	}

	// Sort by CreatedAt ascending in memory — no composite index needed.
	// Tie-break on ID (UUID v7 is lexicographically time-ordered).
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

	docRef := r.knowledgesCollection(workspaceID).Doc(string(knowledge.ID))

	if _, err := docRef.Get(ctx); err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "knowledge not found",
				goerr.V("knowledge_id", knowledge.ID), goerr.V("workspace_id", workspaceID))
		}
		return nil, goerr.Wrap(err, "failed to check knowledge existence",
			goerr.V("knowledge_id", knowledge.ID), goerr.V("workspace_id", workspaceID))
	}

	if _, err := docRef.Set(ctx, knowledge); err != nil {
		return nil, goerr.Wrap(err, "failed to update knowledge",
			goerr.V("knowledge_id", knowledge.ID), goerr.V("workspace_id", workspaceID))
	}

	return knowledge, nil
}

func (r *knowledgeRepository) Delete(ctx context.Context, workspaceID string, id model.KnowledgeID) error {
	docRef := r.knowledgesCollection(workspaceID).Doc(string(id))

	if _, err := docRef.Get(ctx); err != nil {
		if status.Code(err) == codes.NotFound {
			return goerr.Wrap(ErrNotFound, "knowledge not found",
				goerr.V("knowledge_id", id), goerr.V("workspace_id", workspaceID))
		}
		return goerr.Wrap(err, "failed to check knowledge existence",
			goerr.V("knowledge_id", id), goerr.V("workspace_id", workspaceID))
	}

	if _, err := docRef.Delete(ctx); err != nil {
		return goerr.Wrap(err, "failed to delete knowledge",
			goerr.V("knowledge_id", id), goerr.V("workspace_id", workspaceID))
	}

	return nil
}
