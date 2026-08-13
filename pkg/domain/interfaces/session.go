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

	// AdvanceLastMention moves the Session's LastMentionTS forward to mentionTS,
	// and does nothing when the stored value is already at or past it.
	//
	// It is narrow on purpose. LastMentionTS is the cursor the next turn's delta
	// scan starts after, so it must be stamped by the call that actually started a
	// turn — but that call races the turn it just started, whose completion handler
	// writes the same Session row. A full Put from the spawning side would clobber
	// the outcome that handler recorded (a pending question, for one); touching one
	// field cannot. Monotonic because two triggers may race and the later cursor
	// must win regardless of which write lands second.
	//
	// A missing Session is not an error: the thread it named is gone, and there is
	// no cursor to keep.
	AdvanceLastMention(ctx context.Context, channelID, threadTS, mentionTS string) error

	// AssociateProposal points the Session at the case draft the thread is now
	// working on.
	//
	// It is narrow for the same reason as AdvanceLastMention: the caller has just
	// started a turn, and that turn's completion handler writes the same Session
	// row. It is also only safe to call once the turn was ACCEPTED — a draft the
	// runtime refused must never become the thread's draft, or the accepted turn's
	// result would be written into it.
	//
	// A missing Session is not an error.
	AssociateProposal(ctx context.Context, channelID, threadTS string, proposalID model.CaseProposalID) error
}
