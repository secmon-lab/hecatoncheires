package firestore

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const actionCommentsCollection = "comments"

type actionCommentRepository struct {
	client *firestore.Client
}

var _ interfaces.ActionCommentRepository = &actionCommentRepository{}

func newActionCommentRepository(client *firestore.Client) *actionCommentRepository {
	return &actionCommentRepository{client: client}
}

func (r *actionCommentRepository) commentsCollection(workspaceID string, actionID int64) *firestore.CollectionRef {
	return r.client.
		Collection("workspaces").Doc(workspaceID).
		Collection("actions").Doc(fmt.Sprintf("%d", actionID)).
		Collection(actionCommentsCollection)
}

func (r *actionCommentRepository) Put(ctx context.Context, workspaceID string, actionID int64, comment *model.ActionComment) error {
	if err := comment.Validate(); err != nil {
		return goerr.Wrap(err, "action comment validation failed before put")
	}
	// The comment is stored under the actionID parameter's key; reject a struct
	// whose own ActionID points elsewhere so the two can never diverge.
	if comment.ActionID != actionID {
		return goerr.Wrap(model.ErrActionCommentValidation, "action comment ActionID does not match parameter",
			goerr.V("param", actionID), goerr.V("comment", comment.ActionID))
	}

	ref := r.commentsCollection(workspaceID, actionID).Doc(comment.ID)
	if _, err := ref.Set(ctx, comment); err != nil {
		return goerr.Wrap(err, "failed to save action comment",
			goerr.V("workspace_id", workspaceID),
			goerr.V("action_id", actionID),
			goerr.V("comment_id", comment.ID))
	}
	return nil
}

func (r *actionCommentRepository) Get(ctx context.Context, workspaceID string, actionID int64, commentID string) (*model.ActionComment, error) {
	docSnap, err := r.commentsCollection(workspaceID, actionID).Doc(commentID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "action comment not found",
				goerr.V("workspace_id", workspaceID),
				goerr.V("action_id", actionID),
				goerr.V("comment_id", commentID))
		}
		return nil, goerr.Wrap(err, "failed to get action comment",
			goerr.V("workspace_id", workspaceID),
			goerr.V("action_id", actionID),
			goerr.V("comment_id", commentID))
	}

	var c model.ActionComment
	if err := docSnap.DataTo(&c); err != nil {
		return nil, goerr.Wrap(err, "failed to decode action comment",
			goerr.V("doc_id", docSnap.Ref.ID))
	}
	return &c, nil
}

func (r *actionCommentRepository) List(ctx context.Context, workspaceID string, actionID int64, limit int, cursor string) ([]*model.ActionComment, string, error) {
	if limit <= 0 {
		limit = 100
	}

	query := r.commentsCollection(workspaceID, actionID).
		OrderBy("CreatedAt", firestore.Desc).
		Limit(limit + 1)

	if cursor != "" {
		cursorDoc := r.commentsCollection(workspaceID, actionID).Doc(cursor)
		docSnap, err := cursorDoc.Get(ctx)
		if err != nil {
			return nil, "", goerr.Wrap(err, "failed to get cursor document",
				goerr.V("cursor", cursor))
		}
		query = query.StartAfter(docSnap)
	}

	iter := query.Documents(ctx)
	defer iter.Stop()

	comments := []*model.ActionComment{}
	hasMore := false
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, "", goerr.Wrap(err, "failed to iterate action comments",
				goerr.V("workspace_id", workspaceID),
				goerr.V("action_id", actionID))
		}
		if len(comments) >= limit {
			hasMore = true
			break
		}
		var c model.ActionComment
		if err := doc.DataTo(&c); err != nil {
			return nil, "", goerr.Wrap(err, "failed to decode action comment",
				goerr.V("doc_id", doc.Ref.ID))
		}
		comments = append(comments, &c)
	}

	var nextCursor string
	if hasMore && len(comments) > 0 {
		nextCursor = comments[len(comments)-1].ID
	}
	return comments, nextCursor, nil
}

func (r *actionCommentRepository) Delete(ctx context.Context, workspaceID string, actionID int64, commentID string) error {
	ref := r.commentsCollection(workspaceID, actionID).Doc(commentID)
	if _, err := ref.Delete(ctx); err != nil {
		if status.Code(err) == codes.NotFound {
			return nil
		}
		return goerr.Wrap(err, "failed to delete action comment",
			goerr.V("workspace_id", workspaceID),
			goerr.V("action_id", actionID),
			goerr.V("comment_id", commentID))
	}
	return nil
}
