package job_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/job"
)

// notifierCall records one Slack post made through fakeNotifier.
type notifierCall struct {
	method    string // "root" | "reply"
	channelID string
	threadTS  string
	text      string
}

// fakeNotifier records every job.SlackNotifier call so tests can assert
// count, ordering, and exact field values. Optional errors let tests drive
// the non-fatal failure paths.
type fakeNotifier struct {
	mu       sync.Mutex
	calls    []notifierCall
	rootErr  error
	replyErr error
	rootTS   string
}

func (f *fakeNotifier) PostMessage(_ context.Context, channelID, text string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, notifierCall{method: "root", channelID: channelID, text: text})
	if f.rootErr != nil {
		return "", f.rootErr
	}
	if f.rootTS == "" {
		return "root-ts", nil
	}
	return f.rootTS, nil
}

func (f *fakeNotifier) PostThreadReply(_ context.Context, channelID, threadTS, text string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, notifierCall{method: "reply", channelID: channelID, threadTS: threadTS, text: text})
	if f.replyErr != nil {
		return "", f.replyErr
	}
	return "reply-ts", nil
}

func (f *fakeNotifier) snapshot() []notifierCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]notifierCall, len(f.calls))
	copy(out, f.calls)
	return out
}

// runTool drives one tool span through the handler.
func runTool(h interface {
	StartToolExec(context.Context, string, map[string]any) context.Context
	EndToolExec(context.Context, map[string]any, error)
}, ctx context.Context, toolName string, execErr error) {
	tctx := h.StartToolExec(ctx, toolName, map[string]any{"q": "x"})
	h.EndToolExec(tctx, map[string]any{"ok": true}, execErr)
}

func TestSlackProgressHandler_DedupesByToolName(t *testing.T) {
	ctx := context.Background()
	fake := &fakeNotifier{}
	h := job.NewSlackProgressHandlerForTest(fake, "C1", "T1")

	// Same tool three times → a single post. A different tool → one more.
	runTool(h, ctx, "slack_search", nil)
	runTool(h, ctx, "slack_search", nil)
	runTool(h, ctx, "slack_search", nil)
	runTool(h, ctx, "case_writer", nil)

	calls := fake.snapshot()
	gt.Array(t, calls).Length(2).Required()

	gt.String(t, calls[0].method).Equal("reply")
	gt.String(t, calls[0].channelID).Equal("C1")
	gt.String(t, calls[0].threadTS).Equal("T1")
	gt.String(t, calls[0].text).Equal(i18n.T(ctx, i18n.MsgJobRunToolExecuted, "slack_search"))

	gt.String(t, calls[1].text).Equal(i18n.T(ctx, i18n.MsgJobRunToolExecuted, "case_writer"))
}

func TestSlackProgressHandler_ToolFailureText(t *testing.T) {
	ctx := context.Background()
	fake := &fakeNotifier{}
	h := job.NewSlackProgressHandlerForTest(fake, "C1", "T1")

	runTool(h, ctx, "case_writer", errors.New("boom"))

	calls := fake.snapshot()
	gt.Array(t, calls).Length(1).Required()
	gt.String(t, calls[0].text).Equal(i18n.T(ctx, i18n.MsgJobRunToolFailed, "case_writer"))
}

func TestSlackProgressHandler_QuietSuppresses(t *testing.T) {
	ctx := job.WithQuietForTest(context.Background(), true)
	fake := &fakeNotifier{}
	h := job.NewSlackProgressHandlerForTest(fake, "C1", "T1")

	runTool(h, ctx, "slack_search", nil)

	gt.Array(t, fake.snapshot()).Length(0)
}

func TestSlackProgressHandler_NoSessionThreadSuppresses(t *testing.T) {
	ctx := context.Background()
	fake := &fakeNotifier{}
	// Empty sessionThreadTS (e.g. channel-mode starting marker failed).
	h := job.NewSlackProgressHandlerForTest(fake, "C1", "")

	runTool(h, ctx, "slack_search", nil)

	gt.Array(t, fake.snapshot()).Length(0)
}

func TestSlackProgressHandler_NilNotifierSuppresses(t *testing.T) {
	ctx := context.Background()
	h := job.NewSlackProgressHandlerForTest(nil, "C1", "T1")

	// Must not panic; simply no-op.
	runTool(h, ctx, "slack_search", nil)
}

func TestSlackProgressHandler_OtherHooksNoOp(t *testing.T) {
	ctx := context.Background()
	fake := &fakeNotifier{}
	h := job.NewSlackProgressHandlerForTest(fake, "C1", "T1")

	// Every non-tool hook must be inert (no Slack traffic).
	h.StartAgentExecute(ctx)
	h.EndAgentExecute(ctx, nil)
	lc := h.StartLLMCall(ctx)
	h.EndLLMCall(lc, nil, nil)
	sc := h.StartSubAgent(ctx, "sub")
	h.EndSubAgent(sc, nil)
	cc := h.StartChildAgent(ctx, "child")
	h.EndChildAgent(cc, nil)
	h.AddEvent(ctx, "kind", nil)
	gt.NoError(t, h.Finish(ctx))

	gt.Array(t, fake.snapshot()).Length(0)
}
