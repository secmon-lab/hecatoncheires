package planexec_test

import (
	"context"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
)

func TestDirectUserPrompt_PlainAndLanguage(t *testing.T) {
	got, err := planexec.RenderDirectUserPromptForTest(planexec.DirectPromptInputForTest{
		UserInput: "What is the current status?",
		Language:  "Japanese",
	})
	gt.NoError(t, err).Required()
	gt.String(t, got).Contains("What is the current status?")
	gt.String(t, got).Contains("Answer the request directly")
	gt.String(t, got).Contains("Japanese")
}

func TestDirectUserPrompt_NoLanguageDirective(t *testing.T) {
	got, err := planexec.RenderDirectUserPromptForTest(planexec.DirectPromptInputForTest{
		UserInput: "hi",
	})
	gt.NoError(t, err).Required()
	gt.Bool(t, containsAny(got, "MUST be written in")).False()
}

func TestGenerateDirectResponse_PlainText(t *testing.T) {
	ctx := context.Background()
	llm := &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					return &gollem.Response{Texts: []string{"here is your direct answer"}}, nil
				},
			}, nil
		},
	}
	text, err := planexec.GenerateDirectResponseForTest(
		ctx,
		llm,
		agentarchive.NewMemoryHistoryRepository(),
		nil,
		"sys",
		"hist-direct-1",
		"",
		"answer me",
		nil, // no tools
		8,
	)
	gt.NoError(t, err).Required()
	gt.String(t, text).Equal("here is your direct answer")
}

func TestGenerateDirectResponse_EmptyResponseIsError(t *testing.T) {
	ctx := context.Background()
	llm := &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					return &gollem.Response{}, nil
				},
			}, nil
		},
	}
	_, err := planexec.GenerateDirectResponseForTest(
		ctx,
		llm,
		agentarchive.NewMemoryHistoryRepository(),
		nil,
		"sys",
		"hist-direct-2",
		"",
		"answer me",
		nil,
		8,
	)
	gt.Error(t, err)
}
