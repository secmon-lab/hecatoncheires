package firestore

import (
	"context"
	"errors"
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

func (r *sessionRepository) AcquireTurnLock(
	ctx context.Context,
	channelID, threadTS, triggerTS, ownerID string,
	staleAfter time.Duration,
	newSessionFn func() *model.Session,
) (interfaces.AcquireResult, error) {
	if channelID == "" || threadTS == "" || ownerID == "" {
		return interfaces.AcquireResult{}, goerr.New("channelID, threadTS, ownerID are required",
			goerr.V("channel_id", channelID),
			goerr.V("thread_ts", threadTS),
			goerr.V("owner_id", ownerID),
		)
	}
	if newSessionFn == nil {
		return interfaces.AcquireResult{}, goerr.New("newSessionFn is required")
	}

	doc := r.docRef(channelID, threadTS)
	var result interfaces.AcquireResult
	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(doc)
		if err != nil && status.Code(err) != codes.NotFound {
			return goerr.Wrap(err, "tx get session")
		}

		now := r.now()

		if status.Code(err) == codes.NotFound {
			fresh := newSessionFn()
			if fresh == nil {
				return goerr.New("newSessionFn returned nil")
			}
			fresh.ChannelID = channelID
			fresh.ThreadTS = threadTS
			fresh.TurnState = model.SessionTurnRunning
			fresh.TurnOwnerID = ownerID
			fresh.TurnStartedAt = now
			fresh.TurnHeartbeatAt = now
			fresh.TurnTriggerTS = triggerTS
			if fresh.CreatedAt.IsZero() {
				fresh.CreatedAt = now
			}
			fresh.UpdatedAt = now
			if err := fresh.Validate(); err != nil {
				return goerr.Wrap(err, "session validation failed before acquire")
			}
			if err := tx.Set(doc, fresh); err != nil {
				return goerr.Wrap(err, "tx set new session")
			}
			result = interfaces.AcquireResult{Acquired: true, Session: fresh}
			return nil
		}

		var cur model.Session
		if err := snap.DataTo(&cur); err != nil {
			return goerr.Wrap(err, "decode session")
		}

		// Idempotent retry: same Slack-side trigger key as the live owner.
		// The key must be non-empty (synthetic events pass "").
		if cur.TurnState == model.SessionTurnRunning && triggerTS != "" && cur.TurnTriggerTS == triggerTS {
			result = interfaces.AcquireResult{IdempotentRetry: true, Session: &cur}
			return nil
		}

		reclaimed := false
		if cur.TurnState == model.SessionTurnRunning {
			if staleAfter <= 0 || now.Sub(cur.TurnHeartbeatAt) <= staleAfter {
				result = interfaces.AcquireResult{Session: &cur}
				return nil
			}
			reclaimed = true
		}

		cur.TurnState = model.SessionTurnRunning
		cur.TurnOwnerID = ownerID
		cur.TurnStartedAt = now
		cur.TurnHeartbeatAt = now
		cur.TurnTriggerTS = triggerTS
		cur.UpdatedAt = now
		if err := tx.Set(doc, &cur); err != nil {
			return goerr.Wrap(err, "tx set claimed session")
		}
		result = interfaces.AcquireResult{Acquired: true, Reclaimed: reclaimed, Session: &cur}
		return nil
	})
	if err != nil {
		return interfaces.AcquireResult{}, goerr.Wrap(err, "acquire turn lock",
			goerr.V("channel_id", channelID),
			goerr.V("thread_ts", threadTS),
		)
	}
	return result, nil
}

func (r *sessionRepository) Heartbeat(ctx context.Context, channelID, threadTS, ownerID string) (*model.Session, error) {
	if channelID == "" || threadTS == "" || ownerID == "" {
		return nil, goerr.New("channelID, threadTS, ownerID are required")
	}
	doc := r.docRef(channelID, threadTS)
	var out *model.Session
	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(doc)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return errors.Join(interfaces.ErrTurnOwnerMismatch,
					goerr.New("session not found",
						goerr.V("channel_id", channelID),
						goerr.V("thread_ts", threadTS)))
			}
			return goerr.Wrap(err, "tx get session for heartbeat")
		}
		var cur model.Session
		if err := snap.DataTo(&cur); err != nil {
			return goerr.Wrap(err, "decode session")
		}
		if cur.TurnOwnerID != ownerID {
			return errors.Join(interfaces.ErrTurnOwnerMismatch,
				goerr.New("owner mismatch",
					goerr.V("expected", ownerID),
					goerr.V("actual", cur.TurnOwnerID)))
		}
		now := r.now()
		cur.TurnHeartbeatAt = now
		cur.UpdatedAt = now
		if err := tx.Set(doc, &cur); err != nil {
			return goerr.Wrap(err, "tx set heartbeat")
		}
		out = &cur
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *sessionRepository) ReleaseTurnLock(ctx context.Context, channelID, threadTS, ownerID string) error {
	if channelID == "" || threadTS == "" || ownerID == "" {
		return goerr.New("channelID, threadTS, ownerID are required")
	}
	doc := r.docRef(channelID, threadTS)
	return r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(doc)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return nil
			}
			return goerr.Wrap(err, "tx get session for release")
		}
		var cur model.Session
		if err := snap.DataTo(&cur); err != nil {
			return goerr.Wrap(err, "decode session")
		}
		if cur.TurnOwnerID != ownerID {
			return nil
		}
		now := r.now()
		cur.TurnState = model.SessionTurnIdle
		cur.TurnOwnerID = ""
		cur.TurnTriggerTS = ""
		cur.UpdatedAt = now
		if err := tx.Set(doc, &cur); err != nil {
			return goerr.Wrap(err, "tx set released session")
		}
		return nil
	})
}
