package firestore

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	slackChannelsCollection = "slack_channels"
	slackMessagesCollection = "messages"
)

type slackRepository struct {
	client           *firestore.Client
	collectionPrefix string
}

var _ interfaces.SlackRepository = &slackRepository{}

func newSlackRepository(client *firestore.Client) *slackRepository {
	return &slackRepository{
		client: client,
	}
}

// slackMessage is the Firestore persistence model
type slackMessage struct {
	ID        string
	ThreadTS  string
	UserID    string
	UserName  string
	Text      string
	EventTS   string
	CreatedAt time.Time
}

// slackChannel is the parent document for channel metadata
type slackChannel struct {
	ChannelID     string
	TeamID        string
	LastMessageAt time.Time
	MessageCount  int64
}

func (r *slackRepository) channelsCollection() *firestore.CollectionRef {
	name := slackChannelsCollection
	if r.collectionPrefix != "" {
		name = r.collectionPrefix + name
	}
	return r.client.Collection(name)
}

func (r *slackRepository) messagesCollection(channelID string) *firestore.CollectionRef {
	return r.channelsCollection().Doc(channelID).Collection(slackMessagesCollection)
}

func (r *slackRepository) PutMessage(ctx context.Context, msg *slack.Message) error {
	if msg == nil {
		return goerr.New("message is nil")
	}

	channelID := msg.ChannelID()
	messageID := msg.ID()

	// Update or create channel metadata
	channelRef := r.channelsCollection().Doc(channelID)
	if err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		_, err := tx.Get(channelRef)
		if err != nil {
			// Check if channel doesn't exist
			if status.Code(err) == codes.NotFound {
				// Channel doesn't exist, create it
				channelData := &slackChannel{
					ChannelID:     channelID,
					TeamID:        msg.TeamID(),
					LastMessageAt: msg.CreatedAt(),
					MessageCount:  1,
				}
				return tx.Set(channelRef, channelData)
			}
			// Other error, propagate it
			return err
		}

		// Channel exists, update metadata
		return tx.Update(channelRef, []firestore.Update{
			{Path: "LastMessageAt", Value: msg.CreatedAt()},
			{Path: "MessageCount", Value: firestore.Increment(1)},
		})
	}); err != nil {
		return goerr.Wrap(err, "failed to update channel metadata", goerr.V("channelID", channelID))
	}

	// Save message to subcollection
	msgData := &slackMessage{
		ID:        msg.ID(),
		ThreadTS:  msg.ThreadTS(),
		UserID:    msg.UserID(),
		UserName:  msg.UserName(),
		Text:      msg.Text(),
		EventTS:   msg.EventTS(),
		CreatedAt: msg.CreatedAt(),
	}

	msgRef := r.messagesCollection(channelID).Doc(messageID)
	if _, err := msgRef.Set(ctx, msgData); err != nil {
		return goerr.Wrap(err, "failed to save message", goerr.V("channelID", channelID), goerr.V("messageID", messageID))
	}

	return nil
}

func (r *slackRepository) ListMessages(ctx context.Context, channelID string, start, end time.Time, limit int, cursor string) ([]*slack.Message, string, error) {
	if channelID == "" {
		return nil, "", goerr.New("channelID is required")
	}

	if limit <= 0 {
		limit = 100
	}

	// Get channel metadata to retrieve teamID
	channelDoc, err := r.channelsCollection().Doc(channelID).Get(ctx)
	if err != nil {
		// Channel doesn't exist, return empty list
		if status.Code(err) == codes.NotFound {
			return []*slack.Message{}, "", nil
		}
		return nil, "", goerr.Wrap(err, "failed to get channel metadata", goerr.V("channelID", channelID))
	}

	var channelData slackChannel
	if err := channelDoc.DataTo(&channelData); err != nil {
		return nil, "", goerr.Wrap(err, "failed to unmarshal channel data", goerr.V("channelID", channelID))
	}

	query := r.messagesCollection(channelID).
		Where("CreatedAt", ">=", start).
		Where("CreatedAt", "<", end).
		OrderBy("CreatedAt", firestore.Desc).
		Limit(limit + 1)

	// Apply cursor if provided
	if cursor != "" {
		cursorDoc := r.messagesCollection(channelID).Doc(cursor)
		docSnap, err := cursorDoc.Get(ctx)
		if err != nil {
			return nil, "", goerr.Wrap(err, "failed to get cursor document", goerr.V("cursor", cursor))
		}
		query = query.StartAfter(docSnap)
	}

	iter := query.Documents(ctx)
	defer iter.Stop()

	var messages []*slack.Message
	var hasMore bool

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, "", goerr.Wrap(err, "failed to iterate messages")
		}

		// Check if we've already collected enough messages
		if len(messages) >= limit {
			// We have limit+1 results, meaning there's more data
			hasMore = true
			break
		}

		var msgData slackMessage
		if err := doc.DataTo(&msgData); err != nil {
			return nil, "", goerr.Wrap(err, "failed to unmarshal message", goerr.V("docID", doc.Ref.ID))
		}

		// Convert to domain model using the exported constructor
		msg := slack.NewMessageFromData(
			msgData.ID,
			channelID,
			msgData.ThreadTS,
			channelData.TeamID,
			msgData.UserID,
			msgData.UserName,
			msgData.Text,
			msgData.EventTS,
			msgData.CreatedAt,
		)
		messages = append(messages, msg)
	}

	// Set next cursor to the last message ID if there's more data
	var nextCursor string
	if hasMore && len(messages) > 0 {
		nextCursor = messages[len(messages)-1].ID()
	}

	return messages, nextCursor, nil
}

func (r *slackRepository) PruneMessages(ctx context.Context, channelID string, before time.Time) (int, error) {
	if channelID == "" {
		// Delete from all channels
		return r.pruneAllChannels(ctx, before)
	}

	return r.pruneChannel(ctx, channelID, before)
}

func (r *slackRepository) pruneChannel(ctx context.Context, channelID string, before time.Time) (int, error) {
	const batchSize = 500
	totalDeleted := 0

	for {
		// Query messages to delete
		query := r.messagesCollection(channelID).
			Where("CreatedAt", "<", before).
			Limit(batchSize)

		iter := query.Documents(ctx)
		bulkWriter := r.client.BulkWriter(ctx)
		count := 0

		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				iter.Stop()
				bulkWriter.End()
				return totalDeleted, goerr.Wrap(err, "failed to iterate messages for deletion", goerr.V("channelID", channelID))
			}

			if _, err := bulkWriter.Delete(doc.Ref); err != nil {
				iter.Stop()
				bulkWriter.End()
				return totalDeleted, goerr.Wrap(err, "failed to delete message", goerr.V("channelID", channelID))
			}
			count++
		}
		iter.Stop()
		bulkWriter.End()

		if count == 0 {
			break
		}

		totalDeleted += count

		// If we deleted less than batchSize, we're done
		if count < batchSize {
			break
		}
	}

	return totalDeleted, nil
}

func (r *slackRepository) pruneAllChannels(ctx context.Context, before time.Time) (int, error) {
	// List all channels
	iter := r.channelsCollection().Documents(ctx)
	defer iter.Stop()

	totalDeleted := 0

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return totalDeleted, goerr.Wrap(err, "failed to iterate channels")
		}

		channelID := doc.Ref.ID
		deleted, err := r.pruneChannel(ctx, channelID, before)
		if err != nil {
			return totalDeleted, goerr.Wrap(err, "failed to prune channel", goerr.V("channelID", channelID))
		}

		totalDeleted += deleted
	}

	return totalDeleted, nil
}
