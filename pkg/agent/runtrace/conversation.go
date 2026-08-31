package runtrace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"

	"github.com/google/uuid"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// conversation tracks how much of one LLM conversation has already been
// recorded, so an LLM_REQUEST event carries only the messages that are new.
//
// A conversation grows by appending: every call re-sends the history plus this
// turn's input. Recording the whole list each time therefore stores the same
// messages once per call, which is what made job_run_events the largest table
// the export produces by two orders of magnitude.
type conversation struct {
	// id is written on every event of this conversation, so a consumer can
	// group the partial message lists back together.
	id string

	mu sync.Mutex
	// recorded is the number of leading messages earlier events already carried.
	recorded int
	// boundary fingerprints the last recorded message. It is what makes the diff
	// self-checking: a request whose message at index recorded-1 does not match
	// is not a continuation of what was recorded — a compacted history, or a
	// second conversation reaching the same handler — so the whole list is
	// recorded again instead of a diff that would silently drop messages.
	boundary string
	// toolsFingerprint is the tool set the conversation last recorded, and
	// toolsRecorded whether it held anything. Together they suppress the
	// unchanged set the following calls re-send, without suppressing a set that
	// actually differs.
	toolsFingerprint string
	toolsRecorded    bool
}

func newConversation() *conversation {
	return &conversation{id: uuid.Must(uuid.NewV7()).String()}
}

// conversationKey is the context key under which a conversation scope travels.
type conversationKey struct{}

// withConversation scopes a fresh LLM conversation to ctx: every LLM call
// recorded under the returned context is diffed against that conversation
// alone. It is how a sub-agent's conversation is kept apart from its caller's.
//
// A call with no scope in context belongs to the handler's own conversation,
// unless it is nested inside a tool execution — see Handler.StartLLMCall, which
// cannot use this because the context a handler is handed at End* is the one
// recorded at Start*, not the caller's current one (trace.Multi).
func withConversation(ctx context.Context) context.Context {
	return context.WithValue(ctx, conversationKey{}, newConversation())
}

func conversationFrom(ctx context.Context) *conversation {
	c, _ := ctx.Value(conversationKey{}).(*conversation)
	return c
}

// apply rewrites p to carry only what this conversation has not recorded yet,
// and records what p now covers.
func (c *conversation) apply(p *model.LLMRequestPayload) {
	if c == nil || p == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	p.ConversationID = c.id
	msgs := p.Messages

	if c.recorded > 0 && c.recorded <= len(msgs) &&
		fingerprint(msgs[c.recorded-1]) == c.boundary {
		p.MessagesPrefixLen = c.recorded
		p.Messages = msgs[c.recorded:]
	} else {
		// Not a continuation. The segment starting here has to stand on its
		// own, tools included, because a consumer has nothing earlier to
		// concatenate it onto.
		p.MessagesPrefixLen = 0
		c.toolsRecorded = false
	}

	if fp := fingerprint(p.Tools); c.toolsRecorded && fp == c.toolsFingerprint {
		p.Tools = nil
	} else {
		c.toolsFingerprint = fp
		c.toolsRecorded = len(p.Tools) > 0
	}

	c.recorded = len(msgs)
	if len(msgs) > 0 {
		c.boundary = fingerprint(msgs[len(msgs)-1])
	} else {
		c.boundary = ""
	}
}

// fingerprint hashes a payload value for equality checks. A value that cannot
// be marshalled returns a unique string rather than a shared sentinel, so it
// compares equal to nothing and the caller records in full — the safe direction
// when the check itself is unavailable.
func fingerprint(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return uuid.Must(uuid.NewV7()).String()
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
