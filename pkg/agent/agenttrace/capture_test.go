package agenttrace_test

import (
	"context"
	"testing"

	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/agenttrace"
)

func TestModelCaptureKeepsTheReportedModel(t *testing.T) {
	ctx := context.Background()
	c := &agenttrace.ModelCapture{}

	gt.String(t, c.Model()).Equal("")

	c.EndLLMCall(c.StartLLMCall(ctx), &trace.LLMCallData{Model: "claude-opus-4-6"}, nil)
	gt.String(t, c.Model()).Equal("claude-opus-4-6")
}

// TestModelCaptureIgnoresACallThatNamesNothing pins that a provider reporting no
// data — which is what a failed call produces — does not erase a name already
// captured. The recorded model must not depend on how a call ended.
func TestModelCaptureIgnoresACallThatNamesNothing(t *testing.T) {
	ctx := context.Background()
	c := &agenttrace.ModelCapture{}

	c.EndLLMCall(ctx, &trace.LLMCallData{Model: "gemini-2.5-pro"}, nil)
	c.EndLLMCall(ctx, nil, goerr.New("provider refused"))
	c.EndLLMCall(ctx, &trace.LLMCallData{}, nil)

	gt.String(t, c.Model()).Equal("gemini-2.5-pro")
}

// TestModelCaptureRecordsNothingElse pins that every other callback is inert. The
// provider drives the same Start/End pairs the kernel middleware does, so a
// capture that forwarded them would record each call twice.
func TestModelCaptureRecordsNothingElse(t *testing.T) {
	ctx := context.Background()
	c := &agenttrace.ModelCapture{}

	gt.Value(t, c.StartAgentExecute(ctx)).Equal(ctx)
	gt.Value(t, c.StartToolExec(ctx, "slack__search", map[string]any{"q": "x"})).Equal(ctx)
	gt.Value(t, c.StartSubAgent(ctx, "task")).Equal(ctx)
	gt.Value(t, c.StartChildAgent(ctx, "child")).Equal(ctx)

	c.EndAgentExecute(ctx, nil)
	c.EndToolExec(ctx, map[string]any{"ok": true}, nil)
	c.EndSubAgent(ctx, nil)
	c.EndChildAgent(ctx, nil)
	c.AddEvent(ctx, "phase", "plan")
	gt.NoError(t, c.Finish(ctx))

	gt.String(t, c.Model()).Equal("")
}
