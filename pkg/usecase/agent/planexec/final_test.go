package planexec_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
)

func TestFinalUserPrompt_Plain(t *testing.T) {
	got, err := planexec.RenderFinalUserPromptForTest(planexec.FinalPromptInputForTest{
		Observations:    "## phase 1\n(found stuff)\n",
		StructuredFinal: false,
		Language:        "",
	})
	gt.NoError(t, err).Required()
	gt.String(t, got).Contains("(found stuff)")
	gt.String(t, got).Contains("Emit plain natural-language text")
	gt.Bool(t, containsAny(got, "Emit a single JSON object")).False()
	gt.Bool(t, containsAny(got, "MUST be written in")).False()
}

func TestFinalUserPrompt_StructuredAndLanguage(t *testing.T) {
	got, err := planexec.RenderFinalUserPromptForTest(planexec.FinalPromptInputForTest{
		Observations:    "(obs)",
		StructuredFinal: true,
		Language:        "Japanese",
	})
	gt.NoError(t, err).Required()
	gt.String(t, got).Contains("Emit a single JSON object")
	gt.String(t, got).Contains("Japanese")
	gt.Bool(t, containsAny(got, "Emit plain natural-language text")).False()
}

func TestFinalUserPrompt_EmptyObservationsLabel(t *testing.T) {
	got := planexec.RenderObservationsForFinalForTest(nil)
	gt.String(t, got).Contains("no investigations were run")
}

func TestGenerateFinalResponse_PlainText(t *testing.T) {
	ctx := context.Background()
	llm := &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					return &gollem.Response{Texts: []string{"the final answer"}}, nil
				},
			}, nil
		},
	}
	text, raw, err := planexec.GenerateFinalResponseForTest(
		ctx,
		llm,
		agentarchive.NewMemoryHistoryRepository(),
		nil,
		"sys",
		"hist-1",
		"",
		nil, // no phase results
		nil, // no schema
	)
	gt.NoError(t, err).Required()
	gt.String(t, text).Equal("the final answer")
	gt.Value(t, raw).Nil()
}

func TestGenerateFinalResponse_StructuredJSON(t *testing.T) {
	ctx := context.Background()
	llm := &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					return &gollem.Response{Texts: []string{`{"title":"hi","desc":"there"}`}}, nil
				},
			}, nil
		},
	}
	schema := &gollem.Parameter{
		Type: gollem.TypeObject,
		Properties: map[string]*gollem.Parameter{
			"title": {Type: gollem.TypeString},
			"desc":  {Type: gollem.TypeString},
		},
	}
	text, raw, err := planexec.GenerateFinalResponseForTest(
		ctx,
		llm,
		agentarchive.NewMemoryHistoryRepository(),
		nil,
		"sys",
		"hist-1",
		"",
		nil,
		schema,
	)
	gt.NoError(t, err).Required()
	gt.String(t, text).Equal("")
	var got struct {
		Title string `json:"title"`
		Desc  string `json:"desc"`
	}
	gt.NoError(t, json.Unmarshal(raw, &got)).Required()
	gt.String(t, got.Title).Equal("hi")
	gt.String(t, got.Desc).Equal("there")
}

// containsAny is a thin substring helper used by negative-presence
// assertions in this file.
func containsAny(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
