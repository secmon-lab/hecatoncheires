package firestore

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const notificationSlotsSubcollection = "notification_slots"

type notificationSlotRepository struct {
	client *firestore.Client
}

var _ interfaces.NotificationSlotRepository = &notificationSlotRepository{}

func newNotificationSlotRepository(client *firestore.Client) *notificationSlotRepository {
	return &notificationSlotRepository{client: client}
}

func (r *notificationSlotRepository) docRef(channelID string) *firestore.DocumentRef {
	return r.client.
		Collection(slackChannelsCollection).Doc(channelID).
		Collection(notificationSlotsSubcollection).Doc("current")
}

func (r *notificationSlotRepository) GetActive(ctx context.Context, channelID string, now time.Time) (*model.NotificationSlot, error) {
	if channelID == "" {
		return nil, goerr.New("channelID is required")
	}
	snap, err := r.docRef(channelID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, goerr.Wrap(err, "failed to get notification slot",
			goerr.V("channel_id", channelID),
		)
	}
	var slot model.NotificationSlot
	if err := snap.DataTo(&slot); err != nil {
		return nil, goerr.Wrap(err, "failed to decode notification slot",
			goerr.V("doc_id", snap.Ref.ID),
		)
	}
	if !slot.ExpiresAt.After(now) {
		return nil, nil
	}
	return &slot, nil
}

func (r *notificationSlotRepository) Save(ctx context.Context, slot *model.NotificationSlot) error {
	if err := slot.Validate(); err != nil {
		return goerr.Wrap(err, "notification slot validation failed before save")
	}
	if _, err := r.docRef(slot.ChannelID).Set(ctx, slot); err != nil {
		return goerr.Wrap(err, "failed to save notification slot",
			goerr.V("channel_id", slot.ChannelID),
		)
	}
	return nil
}

func (r *notificationSlotRepository) Delete(ctx context.Context, channelID string) error {
	if channelID == "" {
		return goerr.New("channelID is required")
	}
	if _, err := r.docRef(channelID).Delete(ctx); err != nil {
		if status.Code(err) == codes.NotFound {
			return nil
		}
		return goerr.Wrap(err, "failed to delete notification slot",
			goerr.V("channel_id", channelID),
		)
	}
	return nil
}
