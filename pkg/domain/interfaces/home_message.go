package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// HomeMessageRepository persists LLM-generated home messages append-only, per
// user. There is no Update or Delete: freshness is judged by the caller from
// CreatedAt, and history is retained deliberately (anti-repetition input).
type HomeMessageRepository interface {
	// Add appends one generated message (Validate then persist).
	Add(ctx context.Context, msg *model.HomeMessage) error
	// ListRecent returns the user's most recent messages, newest first, up to
	// limit. Returns an empty slice when the user has none.
	ListRecent(ctx context.Context, userID string, limit int) ([]*model.HomeMessage, error)
}
