package job

import (
	"context"
	"sync"

	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// slackProgressHandler is a trace.Handler that surfaces a minimal,
// deduplicated progress line into a Job run's session thread whenever a
// tool is executed. To keep the thread readable it emits at most ONE line
// per distinct tool name per run — repeat calls to the same tool stay
// silent — so a run that hammers one tool in a loop produces a single
// "used tool X" marker rather than dozens.
//
// Every other trace hook is a no-op: LLM calls and sub-agent boundaries are
// recorded to Firestore by jobRunTraceHandler, not echoed to Slack. The two
// handlers are composed via trace.Multi so the Firestore trail and the
// Slack summary stay independent.
//
// Posts are best-effort: failures go through errutil.Handle and never abort
// the run. The handler self-gates on isQuiet(ctx); a quiet Job emits nothing
// even while the handler is still wired into trace.Multi.
type slackProgressHandler struct {
	notifier        SlackNotifier
	channelID       string
	sessionThreadTS string

	mu   sync.Mutex
	seen map[string]struct{}
}

var _ trace.Handler = (*slackProgressHandler)(nil)

// progressSpanKey stashes the executing tool name between this handler's
// StartToolExec and EndToolExec. trace.Multi hands each handler its own
// context, so this never collides with jobRunTraceHandler's span key.
type progressSpanKey struct{}

// newSlackProgressHandler builds a handler bound to one run's session
// thread. notifier may be nil (handler then no-ops); sessionThreadTS empty
// likewise disables posting (e.g. channel-mode run whose starting marker
// failed to post).
func newSlackProgressHandler(notifier SlackNotifier, channelID, sessionThreadTS string) *slackProgressHandler {
	return &slackProgressHandler{
		notifier:        notifier,
		channelID:       channelID,
		sessionThreadTS: sessionThreadTS,
		seen:            make(map[string]struct{}),
	}
}

// firstSeen reports whether toolName has not yet been posted this run and
// records it so later calls report false.
func (h *slackProgressHandler) firstSeen(toolName string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.seen[toolName]; ok {
		return false
	}
	h.seen[toolName] = struct{}{}
	return true
}

func (h *slackProgressHandler) StartToolExec(ctx context.Context, toolName string, _ map[string]any) context.Context {
	return context.WithValue(ctx, progressSpanKey{}, toolName)
}

// EndToolExec posts the first occurrence of each distinct tool name to the
// session thread. Disabled when the handler lacks a notifier / thread, or
// when the run is quiet.
func (h *slackProgressHandler) EndToolExec(ctx context.Context, _ map[string]any, execErr error) {
	if h == nil || h.notifier == nil || h.sessionThreadTS == "" || h.channelID == "" {
		return
	}
	if isQuiet(ctx) {
		return
	}
	toolName, _ := ctx.Value(progressSpanKey{}).(string)
	if toolName == "" || !h.firstSeen(toolName) {
		return
	}

	var text string
	if execErr != nil {
		text = i18n.T(ctx, i18n.MsgJobRunToolFailed, toolName)
	} else {
		text = i18n.T(ctx, i18n.MsgJobRunToolExecuted, toolName)
	}
	if _, err := h.notifier.PostThreadReply(ctx, h.channelID, h.sessionThreadTS, text); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "post job run tool progress",
			goerr.V("channel_id", h.channelID),
			goerr.V("thread_ts", h.sessionThreadTS),
			goerr.V("tool_name", toolName)), "job: post tool progress to slack")
	}
}

// --- no-op hooks ---------------------------------------------------------
// The session thread carries only lifecycle markers (posted by the runner)
// and the tool-progress lines above; LLM / sub-agent spans are left to the
// Firestore handler.

func (h *slackProgressHandler) StartAgentExecute(ctx context.Context) context.Context { return ctx }
func (h *slackProgressHandler) EndAgentExecute(_ context.Context, _ error)            {}
func (h *slackProgressHandler) StartLLMCall(ctx context.Context) context.Context      { return ctx }
func (h *slackProgressHandler) EndLLMCall(_ context.Context, _ *trace.LLMCallData, _ error) {
}
func (h *slackProgressHandler) StartSubAgent(ctx context.Context, _ string) context.Context {
	return ctx
}
func (h *slackProgressHandler) EndSubAgent(_ context.Context, _ error) {}
func (h *slackProgressHandler) StartChildAgent(ctx context.Context, _ string) context.Context {
	return ctx
}
func (h *slackProgressHandler) EndChildAgent(_ context.Context, _ error)    {}
func (h *slackProgressHandler) AddEvent(_ context.Context, _ string, _ any) {}
func (h *slackProgressHandler) Finish(_ context.Context) error              { return nil }
