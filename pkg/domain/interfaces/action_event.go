package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// ActionEventRepository persists structural change events for an Action.
type ActionEventRepository interface {
	// Put inserts a new event. The ID must be unique within the action.
	Put(ctx context.Context, workspaceID string, actionID int64, event *model.ActionEvent) error

	// List returns events for the action, newest first. limit must be > 0.
	// cursor is the last-seen event ID for pagination; "" means start from the
	// newest. The returned cursor is "" when there are no more events.
	List(ctx context.Context, workspaceID string, actionID int64, limit int, cursor string) ([]*model.ActionEvent, string, error)
}
