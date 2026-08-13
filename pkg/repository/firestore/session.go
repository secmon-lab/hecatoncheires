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

const sessionsCollection = "sessions"

type sessionRepository struct {
	client *firestore.Client
	now    func() time.Time
}

var _ interfaces.SessionRepository = &sessionRepository{}

func newSessionRepository(client *firestore.Client) *sessionRepository {
	return &sessionRepository{
		client: client,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

func (r *sessionRepository) docRef(channelID, threadTS string) *firestore.DocumentRef {
	return r.client.
		Collection(slackChannelsCollection).Doc(channelID).
		Collection(sessionsCollection).Doc(threadTS)
}

func (r *sessionRepository) GetByThread(ctx context.Context, channelID, threadTS string) (*model.Session, error) {
	if channelID == "" || threadTS == "" {
		return nil, goerr.New("channelID and threadTS are required",
			goerr.V("channel_id", channelID),
			goerr.V("thread_ts", threadTS),
		)
	}
	snap, err := r.docRef(channelID, threadTS).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, goerr.Wrap(err, "failed to get session",
			goerr.V("channel_id", channelID),
			goerr.V("thread_ts", threadTS),
		)
	}
	var s model.Session
	if err := snap.DataTo(&s); err != nil {
		return nil, goerr.Wrap(err, "failed to decode session",
			goerr.V("doc_id", snap.Ref.ID),
		)
	}
	return &s, nil
}

func (r *sessionRepository) Put(ctx context.Context, s *model.Session) error {
	if err := s.Validate(); err != nil {
		return goerr.Wrap(err, "session validation failed before put")
	}
	if _, err := r.docRef(s.ChannelID, s.ThreadTS).Set(ctx, s); err != nil {
		return goerr.Wrap(err, "failed to put session",
			goerr.V("channel_id", s.ChannelID),
			goerr.V("thread_ts", s.ThreadTS),
		)
	}
	return nil
}

func (r *sessionRepository) Claim(ctx context.Context, channelID, threadTS string, newSessionFn func() *model.Session) (*model.Session, error) {
	if channelID == "" || threadTS == "" {
		return nil, goerr.New("channelID and threadTS are required",
			goerr.V("channel_id", channelID),
			goerr.V("thread_ts", threadTS),
		)
	}
	if newSessionFn == nil {
		return nil, goerr.New("newSessionFn is required")
	}

	doc := r.docRef(channelID, threadTS)
	var claimed *model.Session
	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(doc)
		if err != nil && status.Code(err) != codes.NotFound {
			return goerr.Wrap(err, "tx get session")
		}
		if err == nil {
			// Already claimed: return it untouched so the first claimer's
			// decision about what owns this thread stands.
			var cur model.Session
			if err := snap.DataTo(&cur); err != nil {
				return goerr.Wrap(err, "decode session")
			}
			claimed = &cur
			return nil
		}

		fresh := newSessionFn()
		if fresh == nil {
			return goerr.New("newSessionFn returned nil")
		}
		fresh.ChannelID = channelID
		fresh.ThreadTS = threadTS
		now := r.now()
		if fresh.CreatedAt.IsZero() {
			fresh.CreatedAt = now
		}
		fresh.UpdatedAt = now
		if err := fresh.Validate(); err != nil {
			return goerr.Wrap(err, "session validation failed before claim")
		}
		if err := tx.Create(doc, fresh); err != nil {
			return goerr.Wrap(err, "tx create session")
		}
		claimed = fresh
		return nil
	})
	if err != nil {
		return nil, goerr.Wrap(err, "claim session",
			goerr.V("channel_id", channelID),
			goerr.V("thread_ts", threadTS),
		)
	}
	return claimed, nil
}

func (r *sessionRepository) AdvanceLastMention(ctx context.Context, channelID, threadTS, mentionTS string) error {
	if channelID == "" || threadTS == "" || mentionTS == "" {
		return goerr.New("channelID, threadTS and mentionTS are required",
			goerr.V("channel_id", channelID),
			goerr.V("thread_ts", threadTS),
			goerr.V("mention_ts", mentionTS),
		)
	}
	doc := r.docRef(channelID, threadTS)
	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(doc)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return nil
			}
			return goerr.Wrap(err, "tx get session for cursor advance")
		}
		var cur model.Session
		if err := snap.DataTo(&cur); err != nil {
			return goerr.Wrap(err, "decode session")
		}
		// Slack timestamps are fixed-width "<seconds>.<microseconds>", so string
		// ordering is chronological ordering.
		if cur.LastMentionTS >= mentionTS {
			return nil
		}
		// Update, not Set: the completion handler of the turn this cursor belongs to
		// writes the same row, and a full replace from here would drop what it
		// recorded.
		if err := tx.Update(doc, []firestore.Update{
			{Path: "LastMentionTS", Value: mentionTS},
			{Path: "UpdatedAt", Value: r.now()},
		}); err != nil {
			return goerr.Wrap(err, "tx update the mention cursor")
		}
		return nil
	})
	if err != nil {
		return goerr.Wrap(err, "advance the mention cursor",
			goerr.V("channel_id", channelID),
			goerr.V("thread_ts", threadTS),
			goerr.V("mention_ts", mentionTS),
		)
	}
	return nil
}
