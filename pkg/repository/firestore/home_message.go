package firestore

import (
	"context"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/api/iterator"
)

type homeMessageRepository struct {
	client *firestore.Client
}

func newHomeMessageRepository(client *firestore.Client) *homeMessageRepository {
	return &homeMessageRepository{client: client}
}

// messagesCollection returns the append-only per-user message subcollection.
// Path: userHomeMessages/{userID}/messages
func (r *homeMessageRepository) messagesCollection(userID string) *firestore.CollectionRef {
	return r.client.Collection("userHomeMessages").Doc(userID).Collection("messages")
}

func (r *homeMessageRepository) Add(ctx context.Context, msg *model.HomeMessage) error {
	if err := msg.Validate(); err != nil {
		return goerr.Wrap(err, "home message validation failed before add")
	}

	docRef := r.messagesCollection(msg.UserID).Doc(string(msg.ID))
	if _, err := docRef.Set(ctx, msg); err != nil {
		return goerr.Wrap(err, "failed to add home message",
			goerr.V("user_id", msg.UserID), goerr.V("id", msg.ID))
	}
	return nil
}

func (r *homeMessageRepository) ListRecent(ctx context.Context, userID string, limit int) ([]*model.HomeMessage, error) {
	// Single-field CreatedAt descending order — served by the automatic
	// single-field index, so no composite index is required.
	q := r.messagesCollection(userID).OrderBy("CreatedAt", firestore.Desc)
	if limit >= 0 {
		q = q.Limit(limit)
	}

	iter := q.Documents(ctx)
	defer iter.Stop()

	items := make([]*model.HomeMessage, 0)
	for {
		docSnap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate home messages", goerr.V("user_id", userID))
		}

		var m model.HomeMessage
		if err := docSnap.DataTo(&m); err != nil {
			return nil, goerr.Wrap(err, "failed to decode home message", goerr.V("doc_id", docSnap.Ref.ID))
		}
		items = append(items, &m)
	}
	return items, nil
}
