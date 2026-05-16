package interfaces

import (
	"context"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// NotificationSlotRepository persists per-channel notification slots used to
// aggregate Slack channel-side notifications within a rolling time window.
// See pkg/usecase/notification_slot.go for the consumer.
type NotificationSlotRepository interface {
	// GetActive returns the slot for channelID if ExpiresAt > now, otherwise
	// (nil, nil). An expired slot is treated as absent so the caller posts a
	// fresh channel message and replaces it via Save.
	GetActive(ctx context.Context, channelID string, now time.Time) (*model.NotificationSlot, error)

	// Save upserts the slot keyed by ChannelID.
	Save(ctx context.Context, slot *model.NotificationSlot) error

	// Delete removes the slot, e.g. when chat.update fails and the slot must
	// be reset so the next event starts a new channel message.
	Delete(ctx context.Context, channelID string) error
}
