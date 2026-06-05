package threadcase

import (
	"context"
	"fmt"

	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
)

// newHandlerSink adapts a threadcase Handler into a planexec.Sink, forwarding
// planner progress as trace lines to the thread. It is intentionally coarse:
// thread-mode shows a flat activity trace rather than the per-task progress
// cards proposal renders.
func newHandlerSink(h Handler) planexec.Sink {
	return planexec.SinkFuncs{
		NotifyFn: func(ctx context.Context, line string) {
			h.Trace(ctx, line)
		},
		PlanProposedFn: func(ctx context.Context, info planexec.PlanInfo) {
			label := "🧭 Planning"
			if info.IsReplan {
				label = "🧭 Re-planning"
			}
			if info.Reasoning != "" {
				h.Trace(ctx, fmt.Sprintf("%s — %s", label, info.Reasoning))
			} else {
				h.Trace(ctx, label)
			}
		},
		PhaseStartedFn: func(ctx context.Context, phase int, tasks []planexec.TaskInfo) {
			h.Trace(ctx, fmt.Sprintf("🔎 Investigating (%d task(s))", len(tasks)))
		},
		TaskFinishedFn: func(ctx context.Context, result planexec.TaskResult) {
			h.Trace(ctx, fmt.Sprintf("✓ %s", result.Title))
		},
	}
}
