package agent_test

import (
	"context"
	"sync"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
)

func TestLLMCallCounter_StartLLMCallIncrements(t *testing.T) {
	c := agent.NewLLMCallCounter()
	gt.Number(t, c.LLMCalls()).Equal(int64(0))

	ctx := context.Background()
	c.StartLLMCall(ctx)
	c.StartLLMCall(ctx)
	c.StartLLMCall(ctx)

	gt.Number(t, c.LLMCalls()).Equal(int64(3))
}

func TestLLMCallCounter_OtherHooksAreNoOps(t *testing.T) {
	c := agent.NewLLMCallCounter()
	ctx := context.Background()

	c.StartAgentExecute(ctx)
	c.EndAgentExecute(ctx, nil)
	c.EndLLMCall(ctx, nil, nil)
	c.StartToolExec(ctx, "x", nil)
	c.EndToolExec(ctx, nil, nil)
	c.StartSubAgent(ctx, "sub")
	c.EndSubAgent(ctx, nil)
	c.StartChildAgent(ctx, "child")
	c.EndChildAgent(ctx, nil)
	c.AddEvent(ctx, "kind", nil)
	gt.NoError(t, c.Finish(ctx))

	gt.Number(t, c.LLMCalls()).Equal(int64(0))
}

func TestLLMCallCounter_ConcurrentSafe(t *testing.T) {
	c := agent.NewLLMCallCounter()
	ctx := context.Background()

	const N = 200
	var wg sync.WaitGroup
	wg.Add(N)
	for range N {
		go func() {
			defer wg.Done()
			c.StartLLMCall(ctx)
		}()
	}
	wg.Wait()

	gt.Number(t, c.LLMCalls()).Equal(int64(N))
}
