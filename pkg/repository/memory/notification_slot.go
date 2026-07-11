package memory

import (
	"context"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type notificationSlotRepository struct {
	mu    sync.Mutex
	slots map[string]model.NotificationSlot
}

var _ interfaces.NotificationSlotRepository = &notificationSlotRepository{}

func newNotificationSlotRepository() *notificationSlotRepository {
	return &notificationSlotRepository{
		slots: make(map[string]model.NotificationSlot),
	}
}

func (r *notificationSlotRepository) GetActive(_ context.Context, channelID string, now time.Time) (*model.NotificationSlot, error) {
	if channelID == "" {
		return nil, goerr.New("channelID is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.slots[channelID]
	if !ok {
		return nil, nil
	}
	if !s.ExpiresAt.After(now) {
		return nil, nil
	}
	copied := s
	// Copy slice so callers can't mutate stored state.
	if len(s.Entries) > 0 {
		copied.Entries = append([]model.NotificationSlotEntry(nil), s.Entries...)
	}
	return &copied, nil
}

func (r *notificationSlotRepository) Save(_ context.Context, slot *model.NotificationSlot) error {
	if err := slot.Validate(); err != nil {
		return goerr.Wrap(err, "notification slot validation failed before save")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := *slot
	if len(slot.Entries) > 0 {
		stored.Entries = append([]model.NotificationSlotEntry(nil), slot.Entries...)
	}
	r.slots[slot.ChannelID] = stored
	return nil
}

func (r *notificationSlotRepository) Delete(_ context.Context, channelID string) error {
	if channelID == "" {
		return goerr.New("channelID is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.slots, channelID)
	return nil
}
