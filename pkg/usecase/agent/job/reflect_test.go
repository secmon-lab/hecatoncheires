package job_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	jobagent "github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/job"
)

// buildKnowledgeDeps constructs KnowledgeAccessor and KnowledgeMutator backed
// by an in-memory repository.
func buildKnowledgeDeps(t *testing.T) (jobagent.ReflectorDeps, *memory.Memory) {
	t.Helper()
	repo := memory.New()
	kUC := usecase.NewKnowledgeUseCase(repo, nil)
	tUC := usecase.NewTagUseCase(repo)
	acc := usecase.NewKnowledgeToolAccessor(kUC, tUC)
	mut := usecase.NewKnowledgeToolMutator(kUC, tUC)
	return jobagent.ReflectorDeps{
		KnowledgeAccessor: acc,
		KnowledgeMutator:  mut,
	}, repo
}

// textOnlyLLM returns a mock LLM whose session always responds with a single
// text message and no tool calls (simulating a text-only reflection response).
func textOnlyLLM(text string) *mock.LLMClientMock {
	called := atomic.Int32{}
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					if called.Add(1) > 5 {
						return nil, goerr.New("unexpected extra LLM call in test")
					}
					return &gollem.Response{Texts: []string{text}}, nil
				},
				HistoryFunc: func() (*gollem.History, error) {
					return &gollem.History{
						Version: gollem.HistoryVersion,
					}, nil
				},
			}, nil
		},
	}
}

// TestNewLLMReflector_RequiredDeps asserts that NewLLMReflector returns an
// error when any of LLMClient, KnowledgeAccessor, or KnowledgeMutator is nil.
func TestNewLLMReflector_RequiredDeps(t *testing.T) {
	deps, _ := buildKnowledgeDeps(t)
	deps.LLMClient = textOnlyLLM("done")

	t.Run("nil LLMClient", func(t *testing.T) {
		d := deps
		d.LLMClient = nil
		_, err := jobagent.NewLLMReflector(d)
		gt.Error(t, err)
	})

	t.Run("nil KnowledgeAccessor", func(t *testing.T) {
		d := deps
		d.KnowledgeAccessor = nil
		_, err := jobagent.NewLLMReflector(d)
		gt.Error(t, err)
	})

	t.Run("nil KnowledgeMutator", func(t *testing.T) {
		d := deps
		d.KnowledgeMutator = nil
		_, err := jobagent.NewLLMReflector(d)
		gt.Error(t, err)
	})

	t.Run("all deps present", func(t *testing.T) {
		_, err := jobagent.NewLLMReflector(deps)
		gt.NoError(t, err)
	})
}

// TestReflectRequest_Validate checks every required field.
func TestReflectRequest_Validate(t *testing.T) {
	validHistory := &gollem.History{Version: gollem.HistoryVersion}

	t.Run("missing WorkspaceID", func(t *testing.T) {
		req := jobagent.ReflectRequest{
			JobID:   "j",
			History: validHistory,
		}
		gt.Error(t, req.Validate())
	})

	t.Run("missing JobID", func(t *testing.T) {
		req := jobagent.ReflectRequest{
			WorkspaceID: "ws",
			History:     validHistory,
		}
		gt.Error(t, req.Validate())
	})

	t.Run("nil History", func(t *testing.T) {
		req := jobagent.ReflectRequest{
			WorkspaceID: "ws",
			JobID:       "j",
			History:     nil,
		}
		gt.Error(t, req.Validate())
	})

	t.Run("all required fields present", func(t *testing.T) {
		req := jobagent.ReflectRequest{
			WorkspaceID: "ws",
			JobID:       "j",
			History:     validHistory,
		}
		gt.NoError(t, req.Validate())
	})
}

// TestLLMReflector_Reflect_TextOnly verifies that when the LLM returns only
// text (no tool calls), Reflect completes without error and the LLM was
// actually invoked (NewSession called at least once).
func TestLLMReflector_Reflect_TextOnly(t *testing.T) {
	deps, _ := buildKnowledgeDeps(t)
	llm := textOnlyLLM("reflection complete, no knowledge updates needed")
	deps.LLMClient = llm
	deps.LoopMax = 3

	reflector, err := jobagent.NewLLMReflector(deps)
	gt.NoError(t, err).Required()

	req := jobagent.ReflectRequest{
		WorkspaceID: "ws-reflect",
		CaseID:      99,
		JobID:       "summarize",
		JobName:     "Summarize",
		History: &gollem.History{
			Version: gollem.HistoryVersion,
		},
	}
	gt.NoError(t, reflector.Reflect(context.Background(), req))

	// The LLM was actually called.
	gt.Number(t, len(llm.NewSessionCalls())).GreaterOrEqual(1)
}

// TestLLMReflector_Reflect_InvalidRequest asserts that Reflect surfaces
// validation errors on the request before touching the LLM.
func TestLLMReflector_Reflect_InvalidRequest(t *testing.T) {
	deps, _ := buildKnowledgeDeps(t)
	llm := &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			t.Fatal("LLM should not be called for an invalid request")
			return nil, nil
		},
	}
	deps.LLMClient = llm

	reflector, err := jobagent.NewLLMReflector(deps)
	gt.NoError(t, err).Required()

	// Missing WorkspaceID → Validate fails → LLM not touched.
	req := jobagent.ReflectRequest{
		JobID:   "j",
		History: &gollem.History{Version: gollem.HistoryVersion},
	}
	gt.Error(t, reflector.Reflect(context.Background(), req))
	gt.Number(t, len(llm.NewSessionCalls())).Equal(0)
}

func TestRenderReflectInstruction(t *testing.T) {
	t.Run("with description includes name and description", func(t *testing.T) {
		got, err := jobagent.RenderReflectInstructionForTest(jobagent.ReflectRequest{
			JobName:        "Daily Digest",
			JobDescription: "summarise the case",
		})
		gt.NoError(t, err).Required()
		gt.Value(t, got).Equal(
			`The conversation above is a completed run of Job "Daily Digest" (summarise the case).
Reflect on it now and curate the workspace Knowledge according to your instructions.`)
	})

	t.Run("without description omits the parenthetical", func(t *testing.T) {
		got, err := jobagent.RenderReflectInstructionForTest(jobagent.ReflectRequest{
			JobName: "Daily Digest",
		})
		gt.NoError(t, err).Required()
		gt.Value(t, got).Equal(
			`The conversation above is a completed run of Job "Daily Digest".
Reflect on it now and curate the workspace Knowledge according to your instructions.`)
	})

	t.Run("empty job name still renders well-formed output", func(t *testing.T) {
		got, err := jobagent.RenderReflectInstructionForTest(jobagent.ReflectRequest{})
		gt.NoError(t, err).Required()
		gt.Value(t, got).Equal(
			`The conversation above is a completed run of Job "".
Reflect on it now and curate the workspace Knowledge according to your instructions.`)
	})
}
