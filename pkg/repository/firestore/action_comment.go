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

// validateForWrite enforces the invariants shared by Create and Update: the
// model's own checks, plus agreement between the struct's ActionID and the
// parameter it is stored under, so the two can never diverge.
func validateActionCommentForWrite(actionID int64, comment *model.ActionComment) error {
	if err := comment.Validate(); err != nil {
		return goerr.Wrap(err, "action comment validation failed before write")
	}
	if comment.ActionID != actionID {
		return goerr.Wrap(model.ErrActionCommentValidation, "action comment ActionID does not match parameter",
			goerr.V("param", actionID), goerr.V("comment", comment.ActionID))
	}
	return nil
}

func (r *actionCommentRepository) Create(ctx context.Context, workspaceID string, actionID int64, comment *model.ActionComment) error {
	if err := validateActionCommentForWrite(actionID, comment); err != nil {
		return err
	}

	ref := r.commentsCollection(workspaceID, actionID).Doc(comment.ID)
	// Create (not Set) so a colliding id fails instead of overwriting.
	if _, err := ref.Create(ctx, comment); err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return goerr.Wrap(interfaces.ErrActionCommentExists, "action comment already exists",
				goerr.V("workspace_id", workspaceID),
				goerr.V("action_id", actionID),
				goerr.V("comment_id", comment.ID))
		}
		return goerr.Wrap(err, "failed to create action comment",
			goerr.V("workspace_id", workspaceID),
			goerr.V("action_id", actionID),
			goerr.V("comment_id", comment.ID))
	}
	return nil
}

// Update writes the comment only if its document still exists. The existence
// check and the write share one transaction so they are atomic against a
// concurrent Delete — a plain Set would resurrect a comment the author had
// already deleted from another tab. Mirrors jobRunLogRepository.setExistingLog.
func (r *actionCommentRepository) Update(ctx context.Context, workspaceID string, actionID int64, comment *model.ActionComment) error {
	if err := validateActionCommentForWrite(actionID, comment); err != nil {
		return err
	}

	ref := r.commentsCollection(workspaceID, actionID).Doc(comment.ID)
	return r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if _, err := tx.Get(ref); err != nil {
			if status.Code(err) == codes.NotFound {
				return goerr.Wrap(interfaces.ErrActionCommentNotFound, "action comment not found",
					goerr.V("workspace_id", workspaceID),
					goerr.V("action_id", actionID),
					goerr.V("comment_id", comment.ID))
			}
			return goerr.Wrap(err, "failed to read action comment before update",
				goerr.V("workspace_id", workspaceID),
				goerr.V("action_id", actionID),
				goerr.V("comment_id", comment.ID))
		}
		if err := tx.Set(ref, comment); err != nil {
			return goerr.Wrap(err, "failed to update action comment",
				goerr.V("workspace_id", workspaceID),
				goerr.V("action_id", actionID),
				goerr.V("comment_id", comment.ID))
		}
		return nil
	})
}

func (r *actionCommentRepository) Get(ctx context.Context, workspaceID string, actionID int64, commentID string) (*model.ActionComment, error) {
	docSnap, err := r.commentsCollection(workspaceID, actionID).Doc(commentID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(interfaces.ErrActionCommentNotFound, "action comment not found",
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
