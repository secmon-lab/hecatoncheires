package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type sessionRepository struct {
	mu       sync.Mutex
	sessions map[string]model.Session
	now      func() time.Time
}

var _ interfaces.SessionRepository = &sessionRepository{}

func newSessionRepository() *sessionRepository {
	return &sessionRepository{
		sessions: make(map[string]model.Session),
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func sessionKey(channelID, threadTS string) string {
	return fmt.Sprintf("%s/%s", channelID, threadTS)
}

func (r *sessionRepository) GetByThread(_ context.Context, channelID, threadTS string) (*model.Session, error) {
	if channelID == "" || threadTS == "" {
		return nil, goerr.New("channelID and threadTS are required",
			goerr.V("channel_id", channelID),
			goerr.V("thread_ts", threadTS),
		)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[sessionKey(channelID, threadTS)]
	if !ok {
		return nil, nil
	}
	copied := s
	return &copied, nil
}

func (r *sessionRepository) Put(_ context.Context, s *model.Session) error {
	if s == nil {
		return goerr.New("session is nil")
	}
	if s.ID == "" || s.ChannelID == "" || s.ThreadTS == "" {
		return goerr.New("session missing required fields",
			goerr.V("id", s.ID),
			goerr.V("channel_id", s.ChannelID),
			goerr.V("thread_ts", s.ThreadTS),
		)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[sessionKey(s.ChannelID, s.ThreadTS)] = *s
	return nil
}

func (r *sessionRepository) AcquireTurnLock(
	_ context.Context,
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

	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	key := sessionKey(channelID, threadTS)
	cur, exists := r.sessions[key]

	// New session — create and claim in one go.
	if !exists {
		fresh := newSessionFn()
		if fresh == nil {
			return interfaces.AcquireResult{}, goerr.New("newSessionFn returned nil")
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
		r.sessions[key] = *fresh
		copied := *fresh
		return interfaces.AcquireResult{Acquired: true, Session: &copied}, nil
	}

	// Idempotent retry: same trigger as the live owner — drop politely.
	if cur.TurnState == model.SessionTurnRunning && cur.TurnTriggerTS == triggerTS {
		copied := cur
		return interfaces.AcquireResult{IdempotentRetry: true, Session: &copied}, nil
	}

	// Reclaim a stale running lock.
	reclaimed := false
	if cur.TurnState == model.SessionTurnRunning {
		if staleAfter <= 0 || now.Sub(cur.TurnHeartbeatAt) <= staleAfter {
			// busy
			copied := cur
			return interfaces.AcquireResult{Session: &copied}, nil
		}
		reclaimed = true
	}

	cur.TurnState = model.SessionTurnRunning
	cur.TurnOwnerID = ownerID
	cur.TurnStartedAt = now
	cur.TurnHeartbeatAt = now
	cur.TurnTriggerTS = triggerTS
	cur.UpdatedAt = now
	r.sessions[key] = cur
	copied := cur
	return interfaces.AcquireResult{Acquired: true, Reclaimed: reclaimed, Session: &copied}, nil
}

func (r *sessionRepository) Heartbeat(_ context.Context, channelID, threadTS, ownerID string) (*model.Session, error) {
	if channelID == "" || threadTS == "" || ownerID == "" {
		return nil, goerr.New("channelID, threadTS, ownerID are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	key := sessionKey(channelID, threadTS)
	cur, ok := r.sessions[key]
	if !ok {
		return nil, errors.Join(interfaces.ErrTurnOwnerMismatch,
			goerr.New("session does not exist",
				goerr.V("channel_id", channelID),
				goerr.V("thread_ts", threadTS)))
	}
	if cur.TurnOwnerID != ownerID {
		return nil, errors.Join(interfaces.ErrTurnOwnerMismatch,
			goerr.New("owner mismatch",
				goerr.V("expected", ownerID),
				goerr.V("actual", cur.TurnOwnerID)))
	}
	now := r.now()
	cur.TurnHeartbeatAt = now
	cur.UpdatedAt = now
	r.sessions[key] = cur
	copied := cur
	return &copied, nil
}

func (r *sessionRepository) ReleaseTurnLock(_ context.Context, channelID, threadTS, ownerID string) error {
	if channelID == "" || threadTS == "" || ownerID == "" {
		return goerr.New("channelID, threadTS, ownerID are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := sessionKey(channelID, threadTS)
	cur, ok := r.sessions[key]
	if !ok {
		return nil
	}
	if cur.TurnOwnerID != ownerID {
		// Silently ignore — caller's lock was already taken / released.
		return nil
	}
	now := r.now()
	cur.TurnState = model.SessionTurnIdle
	cur.TurnOwnerID = ""
	cur.TurnTriggerTS = ""
	cur.UpdatedAt = now
	r.sessions[key] = cur
	return nil
}
