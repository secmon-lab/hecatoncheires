package interfaces

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// ErrActionCommentNotFound is returned when an ActionCommentRepository
// operation expects an existing comment for the key but none exists. Callers
// MUST discriminate with errors.Is(err, ErrActionCommentNotFound) so a storage
// failure is never mistaken for absence.
var ErrActionCommentNotFound = goerr.New("action comment not found")

// ErrActionCommentExists is returned when Create is called with an ID that
// already exists. Comment IDs are server-generated UUIDs, so a collision is a
// generator bug rather than a transient clash — it fails loudly instead of
// overwriting somebody else's comment.
var ErrActionCommentExists = goerr.New("action comment already exists")

// ActionCommentRepository persists Web-UI-authored comments for an Action.
// These are distinct from ActionMessageRepository, which stores Slack thread
// replies ingested from the Action's Slack thread.
//
// Create and Update are separate rather than one upsert: a plain Set would
// silently resurrect a comment its author had already deleted (an edit in a
// second tab reads the comment, the first tab deletes it, and the edit writes
// it back). The same reasoning already governs JobRunLogRepository — see
// firestore.setExistingLog.
type ActionCommentRepository interface {
	// Create inserts a new comment. comment.ActionID must equal the actionID
	// parameter. An ID that already exists fails with ErrActionCommentExists.
	Create(ctx context.Context, workspaceID string, actionID int64, comment *model.ActionComment) error

	// Update replaces an existing comment. comment.ActionID must equal the
	// actionID parameter. A comment that no longer exists fails with
	// ErrActionCommentNotFound; the check and the write are atomic against a
	// concurrent Delete.
	Update(ctx context.Context, workspaceID string, actionID int64, comment *model.ActionComment) error

	// Get retrieves a single comment by id. A missing comment is reported as
	// ErrActionCommentNotFound; every other failure is returned as itself.
	Get(ctx context.Context, workspaceID string, actionID int64, commentID string) (*model.ActionComment, error)

	// List returns comments for the action, newest first. A non-positive limit
	// falls back to 100. cursor is the last-seen comment ID for pagination; ""
	// means start from the newest. The returned cursor is "" when there are no
	// more comments.
	List(ctx context.Context, workspaceID string, actionID int64, limit int, cursor string) ([]*model.ActionComment, string, error)

	// Delete removes a single comment. Deleting a non-existent comment is a no-op.
	Delete(ctx context.Context, workspaceID string, actionID int64, commentID string) error
}
