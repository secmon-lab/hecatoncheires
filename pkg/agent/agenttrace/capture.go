package agenttrace

import (
	"context"
	"sync"

	"github.com/gollem-dev/gollem/trace"
)

// ModelCapture is a trace.Handler that keeps the model name reported for an LLM
// call and discards everything else.
//
// It exists because agentkit.GenerateResult carries the token counts but not the
// model that produced them, so LLMCallData has nothing to fill
// trace.LLMCallData.Model from. gollem's provider clients DO know it: each builds
// its own trace.LLMCallData naming the model it called and hands it to whatever
// trace.Handler the context carries. Installing one of these for the duration of
// one Generate is how that name is obtained.
//
// Every other callback is deliberately a no-op. Installing the run's real handler
// there instead would make the provider record the call a second time, so the
// timeline and the archive would hold two entries for one call — the kernel's
// Generate middleware is what records it.
type ModelCapture struct {
	mu    sync.Mutex
	model string
}

var _ trace.Handler = (*ModelCapture)(nil)

// Model returns the captured model name, or "" when the provider reported none.
func (c *ModelCapture) Model() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.model
}

// EndLLMCall is the only callback that keeps anything. A failed call reports nil
// data, and a provider that names no model reports an empty one; neither
// overwrites a name already captured.
func (c *ModelCapture) EndLLMCall(_ context.Context, data *trace.LLMCallData, _ error) {
	if data == nil || data.Model == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.model = data.Model
}

func (c *ModelCapture) StartAgentExecute(ctx context.Context) context.Context { return ctx }
func (c *ModelCapture) EndAgentExecute(context.Context, error)                {}
func (c *ModelCapture) StartLLMCall(ctx context.Context) context.Context      { return ctx }
func (c *ModelCapture) StartToolExec(ctx context.Context, _ string, _ map[string]any) context.Context {
	return ctx
}
func (c *ModelCapture) EndToolExec(context.Context, map[string]any, error)            {}
func (c *ModelCapture) StartSubAgent(ctx context.Context, _ string) context.Context   { return ctx }
func (c *ModelCapture) EndSubAgent(context.Context, error)                            {}
func (c *ModelCapture) StartChildAgent(ctx context.Context, _ string) context.Context { return ctx }
func (c *ModelCapture) EndChildAgent(context.Context, error)                          {}
func (c *ModelCapture) AddEvent(context.Context, string, any)                         {}
func (c *ModelCapture) Finish(context.Context) error                                  { return nil }
