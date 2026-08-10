package kernel

import (
	"context"
	"fmt"

	"github.com/gollem-dev/agentkit"
	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/agenttrace"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/runtrace"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

type traceHandlerKey struct{}

// withTraceHandler stashes the claim's trace handler so the effect middleware
// can find it. It is claim-scoped state living in a context, not cross-request
// state: it is created when a worker takes a Process and gone when it lets go.
func withTraceHandler(ctx context.Context, h trace.Handler) context.Context {
	return context.WithValue(ctx, traceHandlerKey{}, h)
}

func traceHandlerFrom(ctx context.Context) trace.Handler {
	h, _ := ctx.Value(traceHandlerKey{}).(trace.Handler)
	return h
}

// claimMiddleware brackets a worker's whole run on one Process. It is where the
// request-scoped context a transition needs is assembled — logger fields, the
// user's language, the access actor — and where the trace sinks are opened and
// flushed.
//
// agentkit calls it once per claim, not once per transition, which is exactly
// the lifetime a trace span and a language binding want.
func claimMiddleware(d Deps) agentkit.ClaimMiddleware {
	return func(next agentkit.ClaimHandler) agentkit.ClaimHandler {
		return func(ctx context.Context, req *agentkit.ClaimRequest) (agentkit.ClaimOutcome, error) {
			proc := req.Process
			sc := ScopeFrom(proc.Metadata)

			ctx = logging.With(ctx, logging.From(ctx).With(
				"process_id", string(proc.ID),
				"root_id", string(proc.RootID),
				"agent", string(proc.Agent),
				"state_seq", proc.StateSeq,
			))
			if sc.Lang != "" {
				ctx = i18n.ContextWithLang(ctx, i18n.Lang(sc.Lang))
			}
			// The actor was validated before Spawn; this is where it becomes the
			// access scope every tool in the claim runs under. A run without one
			// acts with no user scope, which the private-case gates read as "no
			// membership".
			if sc.ActorUserID != "" {
				ctx = auth.ContextWithToken(ctx, &auth.Token{Sub: sc.ActorUserID})
			}

			recorder := trace.New(
				trace.WithRepository(d.Trace),
				trace.WithTraceID(claimTraceID(proc)),
				trace.WithMetadata(claimTraceMetadata(proc, sc)),
			)
			handlers := []trace.Handler{recorder}
			if rt := newRunTraceHandler(ctx, d.Tools.Repo, sc); rt != nil {
				handlers = append(handlers, rt)
			}
			handler := trace.Multi(handlers...)
			ctx = withTraceHandler(ctx, handler)

			// The root span has to be opened here, and it has to bracket the
			// whole claim. gollem's recorder only starts a trace on
			// StartAgentExecute, and a child span with no trace to hang off is
			// dropped — so without this the archive of every run would be empty.
			ctx = handler.StartAgentExecute(ctx)
			outcome, err := next(ctx, req)
			handler.EndAgentExecute(ctx, err)

			// context.WithoutCancel so the flush still reaches storage when the
			// claim ended because the caller's context was cancelled — which is
			// exactly when the trace is worth the most.
			flushCtx := context.WithoutCancel(ctx)
			if ferr := recorder.Finish(flushCtx); ferr != nil {
				errutil.Handle(flushCtx, goerr.Wrap(ferr, "persist agent claim trace",
					goerr.V("process", proc.ID)), "persist agent claim trace")
			}
			return outcome, err
		}
	}
}

// claimTraceID names one claim's archive object. A Process runs over many
// claims, possibly on different instances, and trace.Repository.Save overwrites
// by id — so a per-Process id would let a later, partial claim replace the
// complete archive of an earlier one. The committed transition count makes each
// claim's id distinct and orders them.
func claimTraceID(proc *agentkit.Process) string {
	return fmt.Sprintf("%s.%d", proc.ID, proc.StateSeq)
}

func claimTraceMetadata(proc *agentkit.Process, sc Scope) trace.TraceMetadata {
	labels := map[string]string{
		// session_id is required by the memory trace repository and is what
		// existing trace consumers key on, so the root Process id takes that
		// slot: it is the identifier that spans a whole run.
		"session_id":   string(proc.RootID),
		"process_id":   string(proc.ID),
		"agent":        string(proc.Agent),
		"workspace_id": sc.WorkspaceID,
		"channel_id":   sc.ChannelID,
		"thread_ts":    sc.ThreadTS,
	}
	if sc.SessionID != "" {
		labels["slack_session_id"] = sc.SessionID
	}
	if sc.CaseID != 0 {
		labels["case_id"] = fmt.Sprintf("%d", sc.CaseID)
	}
	if sc.JobID != "" {
		labels["job_id"] = sc.JobID
	}
	if sc.JobRunID != "" {
		labels["job_run_id"] = sc.JobRunID
	}
	return trace.TraceMetadata{Labels: labels}
}

// newRunTraceHandler rebuilds the JobRunEvent writer for this claim, or returns
// nil when the run keeps no such record.
//
// The sequence allocator has to continue where the previous claim stopped —
// event sequence numbers are per run, not per claim — so it is seeded from the
// events already stored. Failing to read them is not worth failing the claim
// for: the timeline is observability, and losing it must never cost the turn.
func newRunTraceHandler(ctx context.Context, repo interfaces.Repository, sc Scope) *runtrace.Handler {
	if sc.JobRunID == "" || sc.WorkspaceID == "" || sc.CaseID == 0 {
		return nil
	}
	key := model.JobRunKey{WorkspaceID: sc.WorkspaceID, CaseID: sc.CaseID, JobID: sc.JobID}
	events, err := repo.JobRunEvent().List(ctx, key, sc.JobRunID)
	if err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "read the run timeline to continue its sequence",
			goerr.V("job_run_id", sc.JobRunID)), "read the run timeline to continue its sequence")
		return nil
	}
	var next int64
	for _, ev := range events {
		if ev.Sequence >= next {
			next = ev.Sequence + 1
		}
	}
	return runtrace.NewHandler(
		repo.JobRunEvent(),
		runtrace.Routing{
			WorkspaceID: sc.WorkspaceID,
			CaseID:      sc.CaseID,
			JobID:       sc.JobID,
			RunID:       sc.JobRunID,
			TraceID:     sc.JobRunID,
		},
		runtrace.NewSequencerStartingAt(next),
		nil,
	)
}

// generateMiddleware records one LLM call onto the claim's trace handler.
//
// agentkit builds its own gollem session per Generate and never runs
// gollem.WithTrace, so the Start/End pair the trace consumers expect has to be
// driven from here.
func generateMiddleware() agentkit.GenerateMiddleware {
	return func(next agentkit.GenerateHandler) agentkit.GenerateHandler {
		return func(ctx context.Context, req *agentkit.GenerateRequest) (*agentkit.GenerateResult, error) {
			h := traceHandlerFrom(ctx)
			if h == nil {
				return next(ctx, req)
			}
			spanCtx := h.StartLLMCall(ctx)
			res, err := next(spanCtx, req)
			h.EndLLMCall(spanCtx, agenttrace.LLMCallData(req, res), err)
			return res, err
		}
	}
}

// toolCallMiddleware records one tool execution onto the claim's trace handler.
func toolCallMiddleware() agentkit.ToolCallMiddleware {
	return func(next agentkit.ToolCallHandler) agentkit.ToolCallHandler {
		return func(ctx context.Context, req *agentkit.ToolCallRequest) (map[string]any, error) {
			h := traceHandlerFrom(ctx)
			if h == nil {
				return next(ctx, req)
			}
			spanCtx := h.StartToolExec(ctx, req.Call.Name, req.Call.Arguments)
			out, err := next(spanCtx, req)
			h.EndToolExec(spanCtx, out, err)
			return out, err
		}
	}
}
