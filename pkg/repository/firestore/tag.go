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

type tagRepository struct {
	client *firestore.Client
}

func newTagRepository(client *firestore.Client) *tagRepository {
	return &tagRepository{client: client}
}

var _ interfaces.TagRepository = (*tagRepository)(nil)

// tagsCollection returns the subcollection ref for tags under a workspace.
// Path: workspaces/{workspaceID}/tags
func (r *tagRepository) tagsCollection(workspaceID string) *firestore.CollectionRef {
	return r.client.Collection("workspaces").Doc(workspaceID).Collection("tags")
}

func (r *tagRepository) Create(ctx context.Context, workspaceID string, tag *model.Tag) (*model.Tag, error) {
	if err := tag.Validate(); err != nil {
		return nil, goerr.Wrap(err, "tag validation failed before create")
	}

	docRef := r.tagsCollection(workspaceID).Doc(string(tag.ID))
	if _, err := docRef.Set(ctx, tag); err != nil {
		return nil, goerr.Wrap(err, "failed to create tag",
			goerr.V("tag_id", tag.ID), goerr.V("workspace_id", workspaceID))
	}

	return tag, nil
}

func (r *tagRepository) Get(ctx context.Context, workspaceID string, id model.TagID) (*model.Tag, error) {
	docSnap, err := r.tagsCollection(workspaceID).Doc(string(id)).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "tag not found",
				goerr.V("tag_id", id), goerr.V("workspace_id", workspaceID))
		}
		return nil, goerr.Wrap(err, "failed to get tag",
			goerr.V("tag_id", id), goerr.V("workspace_id", workspaceID))
	}

	var t model.Tag
	if err := docSnap.DataTo(&t); err != nil {
		return nil, goerr.Wrap(err, "failed to decode tag", goerr.V("doc_id", docSnap.Ref.ID))
	}

	return &t, nil
}

func (r *tagRepository) List(ctx context.Context, workspaceID string) ([]*model.Tag, error) {
	iter := r.tagsCollection(workspaceID).Documents(ctx)
	defer iter.Stop()

	items := make([]*model.Tag, 0)
	for {
		docSnap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate tags", goerr.V("workspace_id", workspaceID))
		}

		var t model.Tag
		if err := docSnap.DataTo(&t); err != nil {
			return nil, goerr.Wrap(err, "failed to decode tag", goerr.V("doc_id", docSnap.Ref.ID))
		}
		items = append(items, &t)
	}

	// Sort by CreatedAt ascending in memory — no composite index needed.
	// Tie-break on ID for determinism.
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})

	return items, nil
}

func (r *tagRepository) Update(ctx context.Context, workspaceID string, tag *model.Tag) (*model.Tag, error) {
	if err := tag.Validate(); err != nil {
		return nil, goerr.Wrap(err, "tag validation failed before update")
	}

	docRef := r.tagsCollection(workspaceID).Doc(string(tag.ID))

	if _, err := docRef.Get(ctx); err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "tag not found",
				goerr.V("tag_id", tag.ID), goerr.V("workspace_id", workspaceID))
		}
		return nil, goerr.Wrap(err, "failed to check tag existence",
			goerr.V("tag_id", tag.ID), goerr.V("workspace_id", workspaceID))
	}

	if _, err := docRef.Set(ctx, tag); err != nil {
		return nil, goerr.Wrap(err, "failed to update tag",
			goerr.V("tag_id", tag.ID), goerr.V("workspace_id", workspaceID))
	}

	return tag, nil
}

func (r *tagRepository) Delete(ctx context.Context, workspaceID string, id model.TagID) error {
	docRef := r.tagsCollection(workspaceID).Doc(string(id))

	if _, err := docRef.Get(ctx); err != nil {
		if status.Code(err) == codes.NotFound {
			return goerr.Wrap(ErrNotFound, "tag not found",
				goerr.V("tag_id", id), goerr.V("workspace_id", workspaceID))
		}
		return goerr.Wrap(err, "failed to check tag existence",
			goerr.V("tag_id", id), goerr.V("workspace_id", workspaceID))
	}

	if _, err := docRef.Delete(ctx); err != nil {
		return goerr.Wrap(err, "failed to delete tag",
			goerr.V("tag_id", id), goerr.V("workspace_id", workspaceID))
	}

	return nil
}
