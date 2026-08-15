package kernel_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gollem-dev/agentkit"
	agentprocmemory "github.com/gollem-dev/agentkit/repository/memory"
	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/budget"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
)

func validBudgets() kernel.Budgets {
	return kernel.Budgets{
		Root: budget.Config{MaxSteps: 64, MaxInputTokens: 1000, MaxOutputTokens: 200, NoticeRatio: 0.8},
		Task: budget.Config{MaxSteps: 48, MaxInputTokens: 500, MaxOutputTokens: 100, NoticeRatio: 0.8},
	}
}

func validDeps() kernel.Deps {
	return kernel.Deps{
		Repo:    agentprocmemory.New(),
		History: agentarchive.NewMemoryHistoryStore(),
		LLM:     &mock.LLMClientMock{},
		Trace:   agentarchive.NewMemoryTraceRepository(),
		Budgets: validBudgets(),
		Agents:  agentkit.NewRegistry(),
		Tools: kernel.ToolDeps{
			Repo:     memory.New(),
			Registry: testRegistry(),
		},
	}
}

func TestBuild(t *testing.T) {
	k, err := kernel.Build(validDeps())
	gt.NoError(t, err).Required()
	gt.Value(t, k).NotNil()
}

// TestBuildRejectsIncompleteDeps pins that every dependency the runtime cannot
// work without is checked at startup. Each of these would otherwise surface as a
// nil dereference at the first mention, on a background worker, where the only
// evidence is a panic in a log.
func TestBuildRejectsIncompleteDeps(t *testing.T) {
	testCases := map[string]func(kernel.Deps) kernel.Deps{
		"no process repository": func(d kernel.Deps) kernel.Deps { d.Repo = nil; return d },
		"no history store":      func(d kernel.Deps) kernel.Deps { d.History = nil; return d },
		"no llm client":         func(d kernel.Deps) kernel.Deps { d.LLM = nil; return d },
		"no trace repository":   func(d kernel.Deps) kernel.Deps { d.Trace = nil; return d },
		"no agent registry":     func(d kernel.Deps) kernel.Deps { d.Agents = nil; return d },
		"no tool repository": func(d kernel.Deps) kernel.Deps {
			d.Tools.Repo = nil
			return d
		},
		"no workspace registry": func(d kernel.Deps) kernel.Deps {
			d.Tools.Registry = nil
			return d
		},
		"invalid root budget": func(d kernel.Deps) kernel.Deps {
			d.Budgets.Root.MaxOutputTokens = 0
			return d
		},
		"invalid task budget": func(d kernel.Deps) kernel.Deps {
			d.Budgets.Task.NoticeRatio = 1
			return d
		},
	}

	for name, mutate := range testCases {
		t.Run(name, func(t *testing.T) {
			k, err := kernel.Build(mutate(validDeps()))
			gt.Value(t, err).NotNil()
			gt.Value(t, k).Nil()
		})
	}
}

func TestBudgetsValidate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		gt.NoError(t, validBudgets().Validate())
	})
	t.Run("reports which ceiling is wrong", func(t *testing.T) {
		b := validBudgets()
		b.Task.MaxSteps = 0
		err := b.Validate()
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("task budget")
	})
}

// TestNoDuplicateSideEffects is the test for the guard that stops a tool from
// running twice.
//
// A transition performs its effect and is checkpointed AFTERWARDS, so a claim
// that dies in between leaves a Process whose last checkpoint still asks for the
// call that already happened. agentkit's default lets three such takeovers re-run
// it, and none of this application's tools is idempotent —
// core__create_action, slack__post_to_case_channel and the memo / knowledge
// creators all take effect on the first call. A replay means a second Action or a
// second Slack post.
//
// The two subtests are a pair on purpose: the second shows the default WOULD
// re-run the work, so the first is measuring the option rather than some other
// property of the setup.
func TestNoDuplicateSideEffects(t *testing.T) {
	testCases := map[string]struct {
		opts []agentkit.ServeOption
		// wantStepRan is whether the reclaiming worker was allowed to run Step —
		// which, for a Process that died mid-transition, means re-performing its
		// side effect.
		wantStepRan bool
		wantStatus  agentkit.ProcessStatus
	}{
		"guarded: the reclaim runs nothing": {
			opts:        []agentkit.ServeOption{kernel.NoDuplicateSideEffects()},
			wantStepRan: false,
			wantStatus:  agentkit.ProcessFailed,
		},
		"unguarded: the reclaim re-runs the transition": {
			opts:        nil,
			wantStepRan: true,
			wantStatus:  agentkit.ProcessSucceeded,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			procRepo := agentprocmemory.New()

			// A strategy that records every Step. Standing in for a transition with
			// a side effect: if Step runs again, the effect happened twice.
			var steps atomic.Int32
			reg := agentkit.NewRegistry()
			handle, err := agentkit.Register(reg, "probe", 1,
				countingStrategy{limiter: validBudgets().Root.Limiter(), steps: &steps})
			gt.NoError(t, err).Required()

			k, err := kernel.Build(kernel.Deps{
				Repo:    procRepo,
				History: agentarchive.NewMemoryHistoryStore(),
				LLM:     probeLLM(),
				Trace:   agentarchive.NewMemoryTraceRepository(),
				Budgets: validBudgets(),
				Agents:  reg,
				Tools:   kernel.ToolDeps{Repo: memory.New(), Registry: testRegistry()},
			})
			gt.NoError(t, err).Required()

			// An explicit toolset, not "*": the probe agent has no declared palette,
			// so expanding "*" would fail the tool factory and the Process would
			// requeue instead of reaching the reclaim this test is about.
			pid, err := handle.Spawn(ctx, k, struct{}{},
				agentkit.WithMetadata(kernel.Scope{ToolSets: []string{agent.ToolSetJira}}.Metadata()))
			gt.NoError(t, err).Required()

			// Simulate a worker that claimed the Process and died mid-transition:
			// the row is left running with a lease that has already expired, which
			// is exactly what the next claimer sees.
			expired := time.Now().Add(-time.Minute)
			claimed, err := procRepo.ClaimNextProcess(ctx, "worker-that-died", expired, time.Now())
			gt.NoError(t, err).Required()
			gt.Value(t, claimed).NotNil().Required()
			gt.Value(t, claimed.Status).Equal(agentkit.ProcessRunning)
			gt.Number(t, steps.Load()).Equal(0) // nothing has run yet

			serveCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			served := make(chan error, 1)
			opts := append([]agentkit.ServeOption{
				agentkit.WithPollInterval(5 * time.Millisecond),
			}, tc.opts...)
			go func() { served <- k.Serve(serveCtx, opts...) }()

			var proc *agentkit.Process
			for {
				got, gerr := k.GetProcess(serveCtx, pid)
				gt.NoError(t, gerr).Required()
				if got.Status.Terminal() {
					proc = got
					break
				}
				select {
				case <-serveCtx.Done():
					gt.NoError(t, serveCtx.Err()).Required()
					return
				case <-time.After(2 * time.Millisecond):
				}
			}
			cancel()
			<-served

			gt.Value(t, proc.Status).Equal(tc.wantStatus)
			if tc.wantStepRan {
				gt.Number(t, steps.Load()).GreaterOrEqual(1)
				return
			}
			// The decisive assertion: the reclaiming worker performed no work at
			// all, so the effect of the dead claim was not repeated.
			gt.Number(t, steps.Load()).Equal(0)
			gt.Value(t, proc.Failure).NotNil().Required()
			gt.Value(t, proc.Failure.Code).Equal(agentkit.FailureUncleanReclaim)
		})
	}
}

// countingStrategy records how many times Step ran and then finishes.
type countingStrategy struct {
	limiter agentkit.Limiter
	steps   *atomic.Int32
}

func (countingStrategy) Version() int { return 1 }

func (s countingStrategy) Limit(ctx context.Context, proc *agentkit.Process, m agentkit.Metrics) agentkit.LimitDecision {
	return s.limiter(ctx, proc, m)
}

func (countingStrategy) Init(struct{}) (probeState, error) { return probeState{}, nil }

func (s countingStrategy) Step(_ context.Context, _ agentkit.Syscalls, st probeState) (probeState, agentkit.Decision[string], error) {
	s.steps.Add(1)
	st.Done = true
	return st, agentkit.Done("done"), nil
}

func (countingStrategy) EncodeState(st probeState) ([]byte, error) { return json.Marshal(st) }

func (countingStrategy) DecodeState(_ int, raw []byte) (probeState, error) {
	var st probeState
	err := json.Unmarshal(raw, &st)
	return st, err
}

func (countingStrategy) EncodeOutput(out string) ([]byte, error) { return json.Marshal(out) }
