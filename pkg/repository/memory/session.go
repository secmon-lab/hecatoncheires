package memory

import (
	"context"
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
	if err := s.Validate(); err != nil {
		return goerr.Wrap(err, "session validation failed before put")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[sessionKey(s.ChannelID, s.ThreadTS)] = *s
	return nil
}

func (r *sessionRepository) Claim(_ context.Context, channelID, threadTS string, newSessionFn func() *model.Session) (*model.Session, error) {
	if channelID == "" || threadTS == "" {
		return nil, goerr.New("channelID and threadTS are required",
			goerr.V("channel_id", channelID),
			goerr.V("thread_ts", threadTS),
		)
	}
	if newSessionFn == nil {
		return nil, goerr.New("newSessionFn is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := sessionKey(channelID, threadTS)
	if existing, ok := r.sessions[key]; ok {
		copied := existing
		return &copied, nil
	}

	fresh := newSessionFn()
	if fresh == nil {
		return nil, goerr.New("newSessionFn returned nil")
	}
	fresh.ChannelID = channelID
	fresh.ThreadTS = threadTS
	now := r.now()
	if fresh.CreatedAt.IsZero() {
		fresh.CreatedAt = now
	}
	fresh.UpdatedAt = now
	if err := fresh.Validate(); err != nil {
		return nil, goerr.Wrap(err, "session validation failed before claim")
	}
	r.sessions[key] = *fresh
	return fresh, nil
}

func (r *sessionRepository) AdvanceLastMention(_ context.Context, channelID, threadTS, mentionTS string) error {
	if channelID == "" || threadTS == "" || mentionTS == "" {
		return goerr.New("channelID, threadTS and mentionTS are required",
			goerr.V("channel_id", channelID),
			goerr.V("thread_ts", threadTS),
			goerr.V("mention_ts", mentionTS),
		)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	key := sessionKey(channelID, threadTS)
	cur, ok := r.sessions[key]
	if !ok {
		return nil
	}
	// Slack timestamps are fixed-width "<seconds>.<microseconds>", so string
	// ordering is chronological ordering.
	if cur.LastMentionTS >= mentionTS {
		return nil
	}
	cur.LastMentionTS = mentionTS
	cur.UpdatedAt = r.now()
	r.sessions[key] = cur
	return nil
}
