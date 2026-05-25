package planexec_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
)

func validRunRequest() planexec.RunRequest {
	return planexec.RunRequest{
		HistoryKey:   "ssn-1",
		TraceID:      "trace-1",
		UserInput:    "hi",
		SystemPrompt: "you are a planner",
		ToolResolver: stubResolver{},
		KnownToolIDs: []string{"core_ro"},
		Sink:         planexec.SinkFuncs{},
	}
}

func TestRunRequest_Validate_OK(t *testing.T) {
	req := validRunRequest()
	gt.NoError(t, req.Validate())
}

func TestRunRequest_Validate_RequiresHistoryKey(t *testing.T) {
	req := validRunRequest()
	req.HistoryKey = ""
	gt.Error(t, req.Validate())
}

func TestRunRequest_Validate_RequiresTraceID(t *testing.T) {
	req := validRunRequest()
	req.TraceID = ""
	gt.Error(t, req.Validate())
}

func TestRunRequest_Validate_RequiresUserInput(t *testing.T) {
	req := validRunRequest()
	req.UserInput = ""
	gt.Error(t, req.Validate())
}

func TestRunRequest_Validate_RequiresSystemPrompt(t *testing.T) {
	req := validRunRequest()
	req.SystemPrompt = ""
	gt.Error(t, req.Validate())
}

func TestRunRequest_Validate_RequiresToolResolver(t *testing.T) {
	req := validRunRequest()
	req.ToolResolver = nil
	gt.Error(t, req.Validate())
}

func TestRunRequest_Validate_RequiresKnownToolIDs(t *testing.T) {
	req := validRunRequest()
	req.KnownToolIDs = nil
	gt.Error(t, req.Validate())
}

func TestRunRequest_Validate_RequiresSink(t *testing.T) {
	req := validRunRequest()
	req.Sink = nil
	gt.Error(t, req.Validate())
}

func TestRunRequest_Validate_AllowQuestionRequiresCallback(t *testing.T) {
	req := validRunRequest()
	req.AllowQuestion = true
	// OnQuestion is nil — must fail.
	gt.Error(t, req.Validate())

	// With a callback, validation passes again.
	req.OnQuestion = func(_ context.Context, _ planexec.Question) (planexec.QuestionResult, error) {
		return planexec.QuestionResult{Terminate: true}, nil
	}
	gt.NoError(t, req.Validate())
}

func TestRunRequest_Validate_NilReceiver(t *testing.T) {
	var req *planexec.RunRequest
	gt.Error(t, req.Validate())
}
