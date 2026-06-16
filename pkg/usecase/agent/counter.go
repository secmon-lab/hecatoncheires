package agent

import (
	"context"
	"sync/atomic"

	"github.com/gollem-dev/gollem/trace"
)

// LLMCallCounter is a minimal gollem trace.Handler that does nothing except
// count StartLLMCall invocations. It is attached to sub-agents (via
// gollem.WithTrace) so the driver can read back how many ReAct loops the
// sub-agent burned through and surface the number in the user-facing trace.
//
// The counter is safe for concurrent use; the underlying value is an
// atomic int64.
type LLMCallCounter struct {
	n atomic.Int64
}

// NewLLMCallCounter returns a fresh counter ready to be passed to gollem.
func NewLLMCallCounter() *LLMCallCounter { return &LLMCallCounter{} }

// LLMCalls returns the number of StartLLMCall invocations seen so far.
func (c *LLMCallCounter) LLMCalls() int64 { return c.n.Load() }

// trace.Handler interface implementation. Only StartLLMCall mutates state;
// every other hook is a no-op.

func (c *LLMCallCounter) StartAgentExecute(ctx context.Context) context.Context {
	return ctx
}

func (c *LLMCallCounter) EndAgentExecute(_ context.Context, _ error) {}

func (c *LLMCallCounter) StartLLMCall(ctx context.Context) context.Context {
	c.n.Add(1)
	return ctx
}

func (c *LLMCallCounter) EndLLMCall(_ context.Context, _ *trace.LLMCallData, _ error) {}

func (c *LLMCallCounter) StartToolExec(ctx context.Context, _ string, _ map[string]any) context.Context {
	return ctx
}

func (c *LLMCallCounter) EndToolExec(_ context.Context, _ map[string]any, _ error) {}

func (c *LLMCallCounter) StartSubAgent(ctx context.Context, _ string) context.Context {
	return ctx
}

func (c *LLMCallCounter) EndSubAgent(_ context.Context, _ error) {}

func (c *LLMCallCounter) StartChildAgent(ctx context.Context, _ string) context.Context {
	return ctx
}

func (c *LLMCallCounter) EndChildAgent(_ context.Context, _ error) {}

func (c *LLMCallCounter) AddEvent(_ context.Context, _ string, _ any) {}

func (c *LLMCallCounter) Finish(_ context.Context) error { return nil }

// Compile-time assertion: LLMCallCounter satisfies trace.Handler.
var _ trace.Handler = (*LLMCallCounter)(nil)
