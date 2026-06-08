package llmrun_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gollem/mock"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/llmrun"
)

// fakeLLM returns a session whose Generate yields the given texts.
func fakeLLM(texts ...string) *mock.LLMClientMock {
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					return &gollem.Response{Texts: texts}, nil
				},
				HistoryFunc: func() (*gollem.History, error) {
					return &gollem.History{}, nil
				},
				AppendHistoryFunc: func(_ *gollem.History) error { return nil },
			}, nil
		},
	}
}

func TestComplete_PlainText(t *testing.T) {
	c := llmrun.New(fakeLLM("hello world"))
	out, err := c.Complete(context.Background(), "sys", "user", nil)
	gt.NoError(t, err)
	gt.V(t, out).Equal("hello world")
}

func TestComplete_JSONExtraction(t *testing.T) {
	schema := &gollem.Parameter{Type: gollem.TypeObject}
	c := llmrun.New(fakeLLM("Sure, here you go:\n```json\n{\"ok\":true}\n```"))
	out, err := c.Complete(context.Background(), "sys", "user", schema)
	gt.NoError(t, err)
	gt.V(t, out).Equal("{\"ok\":true}")
}

func TestComplete_EmptyResponse(t *testing.T) {
	c := llmrun.New(fakeLLM())
	_, err := c.Complete(context.Background(), "sys", "user", nil)
	gt.Error(t, err)
}
