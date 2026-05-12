package usecase_test

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/draft"
	goslack "github.com/slack-go/slack"
)

// traceCapture wraps the existing mockSlackService (defined in
// source_test.go) and records every PostThreadMessage / UpdateMessage
// call so the per-task trace tests can assert on the rendered text and
// call sequence. All other Service methods fall through to mockSlackService.
type traceCapture struct {
	mockSlackService
	mu     sync.Mutex
	posts  []traceCall
	postID atomic.Int32
}

type traceCall struct {
	method  string
	ts      string
	blocks  []goslack.Block
	text    string
	channel string
}

func (s *traceCapture) calls() []traceCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]traceCall, len(s.posts))
	copy(out, s.posts)
	return out
}

func (s *traceCapture) PostThreadMessage(_ context.Context, channelID, _ string, blocks []goslack.Block, text string, _ ...slacksvc.PostThreadOption) (string, error) {
	// Use a deterministic, monotonically-increasing TS per post so
	// per-task vs. phase-trace messages can be distinguished by the
	// test without relying on wall-clock timing.
	id := s.postID.Add(1)
	ts := tsFromID(int(id))
	s.mu.Lock()
	defer s.mu.Unlock()
	s.posts = append(s.posts, traceCall{method: "post", ts: ts, blocks: blocks, text: text, channel: channelID})
	return ts, nil
}

func (s *traceCapture) UpdateMessage(_ context.Context, channelID, ts string, blocks []goslack.Block, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.posts = append(s.posts, traceCall{method: "update", ts: ts, blocks: blocks, text: text, channel: channelID})
	return nil
}

func tsFromID(id int) string {
	return "ts-" + string(rune('0'+id))
}

// TestSlackDraftHandler_TraceAndTaskBlocksArePerMessage exercises the
// per-message contract for both phase trace and per-task trace:
//
//   - Each Trace call posts a NEW thread reply, never appends to an
//     existing message. This is what stops the trace from rendering as
//     a single growing context block that pushes downstream content
//     (task blocks, the question form, the preview) around.
//   - RegisterTasks posts one fresh thread reply per task at the
//     moment of registration, so each task block is anchored at its
//     spot in the thread.
//   - TraceTask updates the matching task message in place by TS,
//     never posting a new one.
//   - Different Trace lines occupy different TS; a Trace line never
//     accidentally lands on a task's TS or vice versa.
//   - Empty Trace lines are dropped (nothing to render).
//   - Calling TraceTask with an unknown taskID is a no-op.
func TestSlackDraftHandler_TraceAndTaskBlocksArePerMessage(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	registry := model.NewWorkspaceRegistry()
	cap := &traceCapture{}

	h := usecase.NewSlackDraftHandlerForTest(repo, registry, cap, "C-TRACE", "1700000000.000001")

	// Phase line 1 — fresh post.
	h.Trace(ctx, "Planning round 1")
	calls := cap.calls()
	gt.Array(t, calls).Length(1).Required()
	gt.Value(t, calls[0].method).Equal("post")
	phase1TS := calls[0].ts
	gt.String(t, calls[0].text).Equal("Planning round 1")

	// Phase line 2 — fresh post (NOT an update of phase1TS).
	h.Trace(ctx, "→ investigate — picking up recent context")
	calls = cap.calls()
	gt.Array(t, calls).Length(2).Required()
	gt.Value(t, calls[1].method).Equal("post")
	phase2TS := calls[1].ts
	gt.Value(t, phase2TS).NotEqual(phase1TS)
	gt.String(t, calls[1].text).Contains("investigate")

	// Register two tasks. Each gets its OWN fresh post.
	h.RegisterTasks(ctx, []draft.TaskInfo{
		{ID: "inv-1", Title: "Recent thread"},
		{ID: "inv-2", Title: "Service team"},
	})
	calls = cap.calls()
	gt.Array(t, calls).Length(4).Required()
	gt.Value(t, calls[2].method).Equal("post")
	gt.Value(t, calls[3].method).Equal("post")
	task1TS := calls[2].ts
	task2TS := calls[3].ts
	gt.Value(t, task1TS).NotEqual(phase1TS)
	gt.Value(t, task1TS).NotEqual(phase2TS)
	gt.Value(t, task2TS).NotEqual(task1TS)
	gt.String(t, calls[2].text).Contains("Recent thread")
	gt.String(t, calls[3].text).Contains("Service team")

	// Out-of-order TraceTask updates. Each lands on the matching task's
	// own message (Update of task1TS / task2TS), never opens a new post
	// and never lands on a phase TS.
	h.TraceTask(ctx, "inv-2", "🔍 Task: Service team — running…")
	h.TraceTask(ctx, "inv-1", "🔍 Task: Recent thread — 🛠 calling slack_search")
	h.TraceTask(ctx, "inv-1", "✅ Task: Recent thread — done")
	calls = cap.calls()
	gt.Array(t, calls).Length(7).Required()
	gt.Value(t, calls[4].method).Equal("update")
	gt.Value(t, calls[4].ts).Equal(task2TS)
	gt.Value(t, calls[5].method).Equal("update")
	gt.Value(t, calls[5].ts).Equal(task1TS)
	gt.Value(t, calls[6].method).Equal("update")
	gt.Value(t, calls[6].ts).Equal(task1TS)
	gt.String(t, calls[6].text).Contains("done")

	// Another phase line — NEW post (not an update of phase1TS or
	// phase2TS, and definitely not a task TS).
	h.Trace(ctx, "Planning round 2")
	calls = cap.calls()
	gt.Array(t, calls).Length(8).Required()
	gt.Value(t, calls[7].method).Equal("post")
	gt.Value(t, calls[7].ts).NotEqual(phase1TS)
	gt.Value(t, calls[7].ts).NotEqual(phase2TS)
	gt.Value(t, calls[7].ts).NotEqual(task1TS)
	gt.Value(t, calls[7].ts).NotEqual(task2TS)
	gt.String(t, calls[7].text).Equal("Planning round 2")
	// And the new phase line does NOT carry the prior phase content —
	// each Trace call is its own atomic message.
	gt.Bool(t, strings.Contains(calls[7].text, "Planning round 1")).False()
	gt.Bool(t, strings.Contains(calls[7].text, "investigate")).False()

	// Empty Trace lines are silently dropped — no extra Slack post.
	h.Trace(ctx, "")
	calls = cap.calls()
	gt.Array(t, calls).Length(8)
}

// TestSlackDraftHandler_TraceTaskUnknownIDIsNoOp confirms that a
// sub-agent emitting TraceTask with a taskID that was never registered
// does NOT post a fresh Slack message. Block creation is the parent's
// contract; sub-agents only update existing slots.
func TestSlackDraftHandler_TraceTaskUnknownIDIsNoOp(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	registry := model.NewWorkspaceRegistry()
	cap := &traceCapture{}

	h := usecase.NewSlackDraftHandlerForTest(repo, registry, cap, "C-TRACE", "1700000000.000001")
	h.TraceTask(ctx, "ghost-task", "should-not-render")

	gt.Array(t, cap.calls()).Length(0)
}

// TestSlackDraftHandler_TraceRoundReplacesInPlace exercises the
// per-round contract introduced for the planner trace UI:
//
//   - The first TraceRound call for a given roundKey posts a NEW
//     thread reply and remembers its TS.
//   - Every subsequent call with the same roundKey REPLACES the prior
//     content of that message via UpdateMessage; it must NOT post a
//     fresh thread reply, so the
//     "Planning… → retry → Planning… → action" sequence renders as a
//     single self-updating context block.
//   - A different roundKey opens a new message (separate TS), since
//     the runtime treats each logical planner round as its own
//     boundary.
//   - Empty roundKey or empty line is silently dropped.
//   - The roundKey state does NOT leak into Trace / TraceTask: a
//     subsequent Trace call posts a fresh message even when the most
//     recent round message was just updated.
func TestSlackDraftHandler_TraceRoundReplacesInPlace(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	registry := model.NewWorkspaceRegistry()
	cap := &traceCapture{}

	h := usecase.NewSlackDraftHandlerForTest(repo, registry, cap, "C-TRACE", "1700000000.000001")

	// Round 1, line 1 — fresh post.
	h.TraceRound(ctx, "plan-1", "🤔 Planning…")
	calls := cap.calls()
	gt.Array(t, calls).Length(1).Required()
	gt.Value(t, calls[0].method).Equal("post")
	round1TS := calls[0].ts
	gt.String(t, calls[0].text).Equal("🤔 Planning…")

	// Round 1, line 2 — UPDATE round1TS, NOT a new post.
	h.TraceRound(ctx, "plan-1", "⚠️ Planner output rejected; retrying")
	calls = cap.calls()
	gt.Array(t, calls).Length(2).Required()
	gt.Value(t, calls[1].method).Equal("update")
	gt.Value(t, calls[1].ts).Equal(round1TS)
	gt.String(t, calls[1].text).Contains("retrying")

	// Round 1, line 3 — still UPDATE round1TS.
	h.TraceRound(ctx, "plan-1", "🤔 Planning…")
	calls = cap.calls()
	gt.Array(t, calls).Length(3).Required()
	gt.Value(t, calls[2].method).Equal("update")
	gt.Value(t, calls[2].ts).Equal(round1TS)

	// Round 1, action line — same round, same TS, REPLACES the prior
	// "Planning…" content (no append).
	h.TraceRound(ctx, "plan-1", "→ investigate — picking up recent context")
	calls = cap.calls()
	gt.Array(t, calls).Length(4).Required()
	gt.Value(t, calls[3].method).Equal("update")
	gt.Value(t, calls[3].ts).Equal(round1TS)
	gt.String(t, calls[3].text).Contains("investigate")
	// The replaced content must be the new line ALONE — the prior
	// "Planning…" must not be carried forward.
	gt.Bool(t, strings.Contains(calls[3].text, "Planning")).False()

	// Round 2 — different roundKey opens a NEW post, separate TS.
	h.TraceRound(ctx, "plan-2", "🤔 Planning…")
	calls = cap.calls()
	gt.Array(t, calls).Length(5).Required()
	gt.Value(t, calls[4].method).Equal("post")
	round2TS := calls[4].ts
	gt.Value(t, round2TS).NotEqual(round1TS)

	// Empty roundKey / empty line — silently dropped.
	h.TraceRound(ctx, "", "ignored")
	h.TraceRound(ctx, "plan-2", "")
	gt.Array(t, cap.calls()).Length(5)

	// A regular Trace call does NOT reuse round1TS / round2TS — it
	// posts a fresh thread reply with a new TS.
	h.Trace(ctx, "phase-line")
	calls = cap.calls()
	gt.Array(t, calls).Length(6).Required()
	gt.Value(t, calls[5].method).Equal("post")
	gt.Value(t, calls[5].ts).NotEqual(round1TS)
	gt.Value(t, calls[5].ts).NotEqual(round2TS)
}
