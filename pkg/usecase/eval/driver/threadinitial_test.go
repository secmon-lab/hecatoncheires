package driver_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gollem/mock"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/driver"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/env"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/evaltype"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/scenario"
)

// scriptedLLM drives the planexec loop to a materialize decision with no
// question: plan (1 task) -> sub-agent summary -> replan done -> final decision.
// Routing is by input markers since the response schema is not visible here.
func scriptedLLM() *mock.LLMClientMock {
	gen := func(_ context.Context, input []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
		var sb strings.Builder
		for _, in := range input {
			sb.WriteString(in.String())
			sb.WriteString("\n")
		}
		text := sb.String()
		switch {
		case strings.Contains(text, "investigation loop has finished"):
			return &gollem.Response{Texts: []string{`{"kind":"materialize","title":"Portal login 503","description":"Users get 503 on portal login since this morning.","fields":[{"field_id":"severity","value":"high"}]}`}}, nil
		case strings.Contains(text, "Observations from prior"):
			return &gollem.Response{Texts: []string{`{"message":"done","tasks":[]}`}}, nil
		case strings.Contains(text, "Thread so far"):
			return &gollem.Response{Texts: []string{`{"message":"investigate","tasks":[{"id":"t1","title":"look","description":"investigate the report","acceptance_criteria":"understood","tools":["core_ro"]}]}`}}, nil
		default:
			return &gollem.Response{Texts: []string{"The report describes a portal 503 login failure since this morning; severity appears high."}}, nil
		}
	}
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc:      gen,
				HistoryFunc:       func() (*gollem.History, error) { return &gollem.History{Version: gollem.HistoryVersion}, nil },
				AppendHistoryFunc: func(_ *gollem.History) error { return nil },
				CountTokenFunc:    func(_ context.Context, _ ...gollem.Input) (int, error) { return 0, nil },
			}, nil
		},
	}
}

type fakeCompleter struct{}

func (fakeCompleter) Complete(_ context.Context, _, _ string, _ *gollem.Parameter) (string, error) {
	return "", nil
}

// recordingSim records whether the simulator was asked to answer. In the
// no-question path it must never be called.
type recordingSim struct{ called bool }

func (s *recordingSim) Answer(_ context.Context, _ evaltype.Question) (evaltype.Answers, error) {
	s.called = true
	return evaltype.Answers{}, nil
}

func TestThreadInitial_MaterializeNoQuestion(t *testing.T) {
	sc, err := scenario.Load(filepath.Join("..", "scenario", "testdata", "valid_thread_initial.toml"))
	gt.NoError(t, err)

	e, err := env.Build(context.Background(), sc, env.Options{
		LLM:       scriptedLLM(),
		Completer: fakeCompleter{},
	})
	gt.NoError(t, err)

	d := driver.NewThreadInitial()
	gt.V(t, d.Kind()).Equal("thread_mode_initial")

	sim := &recordingSim{}
	art, err := d.Run(context.Background(), e, sc, sim)
	gt.NoError(t, err)
	gt.B(t, sim.called).False() // agent asked no question -> simulator unused

	ca, ok := art.(*evaltype.CaseArtifact)
	gt.B(t, ok).True()
	gt.V(t, ca.Case).NotNil()
	gt.V(t, ca.Case.Title).Equal("Portal login 503")
	// Materialized field value applied through the case usecase + validator.
	fv, ok := ca.Case.FieldValues["severity"]
	gt.B(t, ok).True()
	gt.V(t, fv.Value).Equal("high")
	gt.A(t, ca.Transcript).Length(1)
}

func TestRegistry_LookupAndKinds(t *testing.T) {
	r := driver.Default()
	_, ok := r.Lookup("thread_mode_initial")
	gt.B(t, ok).True()
	_, ok = r.Lookup("job")
	gt.B(t, ok).True()
	_, ok = r.Lookup("nope")
	gt.B(t, ok).False()
	gt.A(t, r.Kinds()).Length(2)
}
