package firestore

import (
	"context"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type assigneeRankingRepository struct {
	client *firestore.Client
}

var _ interfaces.AssigneeRankingRepository = &assigneeRankingRepository{}

func newAssigneeRankingRepository(client *firestore.Client) *assigneeRankingRepository {
	return &assigneeRankingRepository{client: client}
}

// docRef returns the single per-workspace ranking document.
// Path: workspaces/{workspaceID}/rankings/assignee
//
// A subcollection under the workspace rather than an underscore-joined
// top-level name, per CLAUDE.md § Firestore Naming Policy. "rankings" is
// plural so a future ranking of something else gets its own document ID here
// rather than a new collection.
func (r *assigneeRankingRepository) docRef(workspaceID string) *firestore.DocumentRef {
	return r.client.Collection("workspaces").Doc(workspaceID).Collection("rankings").Doc("assignee")
}

func (r *assigneeRankingRepository) Get(ctx context.Context, workspaceID string) (*model.AssigneeRanking, error) {
	docSnap, err := r.docRef(workspaceID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "assignee ranking not found", goerr.V("workspace_id", workspaceID))
		}
		return nil, goerr.Wrap(err, "failed to get assignee ranking", goerr.V("workspace_id", workspaceID))
	}

	var ranking model.AssigneeRanking
	if err := docSnap.DataTo(&ranking); err != nil {
		return nil, goerr.Wrap(err, "failed to decode assignee ranking", goerr.V("workspace_id", workspaceID))
	}
	return &ranking, nil
}

func (r *assigneeRankingRepository) Set(ctx context.Context, ranking *model.AssigneeRanking) error {
	if err := ranking.Validate(); err != nil {
		return goerr.Wrap(err, "assignee ranking validation failed before set")
	}

	if _, err := r.docRef(ranking.WorkspaceID).Set(ctx, ranking); err != nil {
		return goerr.Wrap(err, "failed to set assignee ranking", goerr.V("workspace_id", ranking.WorkspaceID))
	}
	return nil
}
