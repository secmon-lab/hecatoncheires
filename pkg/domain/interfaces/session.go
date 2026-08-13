package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// SessionRepository persists the per-thread Session every agent host keys its
// conversation state on.
//
// The lookup key is (ChannelID, ThreadTS). Serialising concurrent turns on one
// thread is NOT this repository's job: the agent runtime does it, by spawning
// every turn under the thread's subject, which admits one live run at a time.
type SessionRepository interface {
	// GetByThread returns the Session for (channelID, threadTS), or
	// (nil, nil) when no Session exists yet.
	GetByThread(ctx context.Context, channelID, threadTS string) (*model.Session, error)

	// Put writes the Session.
	Put(ctx context.Context, s *model.Session) error

	// Claim atomically creates the Session from newSessionFn() when none
	// exists for (channelID, threadTS), and returns the stored Session
	// either way. An existing Session is returned untouched — Claim never
	// overwrites, so the first caller to reach a thread decides what owns
	// it (see model.SessionKind) and every later caller observes that
	// decision.
	//
	// It exists because "read, then create later" is not the same thing: a
	// host that reads first and writes after its own setup work leaves a
	// window in which a concurrent event sees no Session and routes the
	// thread somewhere else. Claim is the durable marker a host takes BEFORE
	// that work.
	Claim(ctx context.Context, channelID, threadTS string, newSessionFn func() *model.Session) (*model.Session, error)
}
