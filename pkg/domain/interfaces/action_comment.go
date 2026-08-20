package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// ActionCommentRepository persists Web-UI-authored comments for an Action.
// These are distinct from ActionMessageRepository, which stores Slack thread
// replies ingested from the Action's Slack thread.
type ActionCommentRepository interface {
	// Put inserts or replaces a comment. The ID must be unique within the
	// action, and comment.ActionID must equal the actionID parameter.
	Put(ctx context.Context, workspaceID string, actionID int64, comment *model.ActionComment) error

	// Get retrieves a single comment by id. A missing comment is reported as
	// the implementation's own not-found error.
	Get(ctx context.Context, workspaceID string, actionID int64, commentID string) (*model.ActionComment, error)

	// List returns comments for the action, newest first. A non-positive limit
	// falls back to 100. cursor is the last-seen comment ID for pagination; ""
	// means start from the newest. The returned cursor is "" when there are no
	// more comments.
	List(ctx context.Context, workspaceID string, actionID int64, limit int, cursor string) ([]*model.ActionComment, string, error)

	// Delete removes a single comment. Deleting a non-existent comment is a no-op.
	Delete(ctx context.Context, workspaceID string, actionID int64, commentID string) error
}
