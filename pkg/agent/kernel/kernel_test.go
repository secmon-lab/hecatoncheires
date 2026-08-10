package kernel_test

import (
	"testing"

	"github.com/gollem-dev/agentkit"
	agentprocmemory "github.com/gollem-dev/agentkit/repository/memory"
	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/budget"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
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
