// Package casebound hosts the case-channel agent: a Slack mention in the
// channel of a channel-mode Case. The turn runs as a durable agentkit process on
// the ReAct strategy, so this package spawns it and finishes it; Slack SDK
// imports are forbidden here and everything user-facing crosses the Host port as
// plain text.
package casebound

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
)

// ConversationMessage is a single pre-fetched Slack message handed to the
// runtime. The host resolves display names; the runtime only formats.
type ConversationMessage struct {
	UserID    string
	UserName  string
	Text      string
	Timestamp string
}

// Host is the Slack-facing surface a finished turn needs.
//
// It exists because a turn no longer ends where it started: the run is a durable
// process, so its answer is posted from the completion handler — on whichever
// instance committed the terminal transition — rather than by the caller of
// StartTurn.
type Host interface {
	// Reply posts the agent's answer to the thread.
	Reply(ctx context.Context, channelID, threadTS, text string) error
	// ReportFailure tells the user the turn could not finish. reason is the
	// technical cause; the host decides how much of it to show.
	ReportFailure(ctx context.Context, channelID, threadTS, reason string) error
}

// HostFuncs is a struct-of-funcs adapter for tests and minimal hosts. A missing
// entry is an error rather than a no-op: silently dropping the answer to a
// mention is indistinguishable from the agent having had nothing to say.
type HostFuncs struct {
	ReplyFn         func(ctx context.Context, channelID, threadTS, text string) error
	ReportFailureFn func(ctx context.Context, channelID, threadTS, reason string) error
}

// Reply satisfies Host.
func (h HostFuncs) Reply(ctx context.Context, channelID, threadTS, text string) error {
	if h.ReplyFn == nil {
		return goerr.New("casebound: Reply host is not configured")
	}
	return h.ReplyFn(ctx, channelID, threadTS, text)
}

// ReportFailure satisfies Host.
func (h HostFuncs) ReportFailure(ctx context.Context, channelID, threadTS, reason string) error {
	if h.ReportFailureFn == nil {
		return goerr.New("casebound: ReportFailure host is not configured")
	}
	return h.ReportFailureFn(ctx, channelID, threadTS, reason)
}

var _ Host = HostFuncs{}
