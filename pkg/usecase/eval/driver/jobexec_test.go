package driver_test

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/driver"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/env"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/evaltype"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/scenario"
)

// actionCreatingJobLLM drives the (single-loop) job to call core__create_action
// once, then return a final summary. The created action is an observable,
// trace-independent side effect we can assert on via the repository.
func actionCreatingJobLLM(actionTitle, summary string) *mock.LLMClientMock {
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			var calls atomic.Int32
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					if calls.Add(1) == 1 {
						return &gollem.Response{FunctionCalls: []*gollem.FunctionCall{{
							ID:        "c1",
							Name:      "core__create_action",
							Arguments: map[string]any{"title": actionTitle},
						}}}, nil
					}
					return &gollem.Response{Texts: []string{summary}}, nil
				},
				HistoryFunc:       func() (*gollem.History, error) { return &gollem.History{Version: gollem.HistoryVersion}, nil },
				AppendHistoryFunc: func(_ *gollem.History) error { return nil },
				CountTokenFunc:    func(_ context.Context, _ ...gollem.Input) (int, error) { return 0, nil },
			}, nil
		},
	}
}

func loadJobScenario(t *testing.T) *scenario.Scenario {
	t.Helper()
	sc, err := scenario.Load(filepath.Join("..", "scenario", "testdata", "job_simple.toml"))
	gt.NoError(t, err)
	return sc
}

func TestJobExecution_RunsJobAndCreatesAction(t *testing.T) {
	sc := loadJobScenario(t)

	e, err := env.Build(context.Background(), sc, env.Options{
		LLM:       actionCreatingJobLLM("Investigate portal 503 login", "Created an action to investigate the 503 login issue."),
		Completer: fakeCompleter{},
	})
	gt.NoError(t, err)
	gt.V(t, e.JobRunner).NotNil()

	d := driver.NewJobExecution()
	gt.V(t, d.Kind()).Equal("job")

	art, err := d.Run(context.Background(), e, sc, &recordingSim{})
	gt.NoError(t, err)

	ja, ok := art.(*evaltype.JobArtifact)
	gt.B(t, ok).True()
	gt.V(t, ja.JobID).Equal("triage_summary")
	gt.V(t, ja.Outcome.Stage).Equal("SUCCESS")
	gt.V(t, ja.Case).NotNil()
	gt.V(t, ja.Case.Title).Equal("Cannot log in to portal (503)")

	// Observable side effect: the job created an action against the target case.
	gt.A(t, ja.Actions).Length(1)
	gt.V(t, ja.Actions[0].Title).Equal("Investigate portal 503 login")
	gt.V(t, ja.Actions[0].CaseID).Equal(ja.Case.ID)

	// The judge-facing snapshot reflects the outcome and the created action.
	render := ja.Render()
	gt.String(t, render).Contains("Outcome: SUCCESS")
	gt.String(t, render).Contains("Investigate portal 503 login")
}

func TestJobExecution_UnknownJob(t *testing.T) {
	sc := loadJobScenario(t)
	sc.Job.ID = "no-such-job"

	e, err := env.Build(context.Background(), sc, env.Options{
		LLM:       actionCreatingJobLLM("x", "y"),
		Completer: fakeCompleter{},
	})
	gt.NoError(t, err)

	_, err = driver.NewJobExecution().Run(context.Background(), e, sc, &recordingSim{})
	gt.Error(t, err)
}
