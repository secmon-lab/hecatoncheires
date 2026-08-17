package planexec_test

import (
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
)

// ----- Sub-agent prompt template (prompts/subagent.md) --------------

func TestSubAgentPrompt_RendersTaskFields(t *testing.T) {
	task := planexec.TaskPlan{
		ID:                 "t-1",
		Title:              "Recent thread",
		Description:        "Read the parent thread.",
		AcceptanceCriteria: "Top ten messages summarised.",
		Tools:              []string{"slack_ro"},
	}
	got, err := planexec.RenderSubAgentPromptForTest(task, false, "")
	gt.NoError(t, err).Required()
	gt.String(t, got).Contains("- ID: t-1")
	gt.String(t, got).Contains("- Title: Recent thread")
	gt.String(t, got).Contains("Read the parent thread.")
	gt.String(t, got).Contains("Top ten messages summarised.")
	gt.String(t, got).Contains("investigation sub-agent")
}

func TestSubAgentPrompt_EmptyFieldsStillWellFormed(t *testing.T) {
	// All required fields are usually enforced by TaskPlan validation
	// before this template is reached, but the template itself must
	// not panic on zero values.
	got, err := planexec.RenderSubAgentPromptForTest(planexec.TaskPlan{}, false, "")
	gt.NoError(t, err).Required()
	gt.String(t, got).Contains("## Your Task")
	gt.String(t, got).Contains("Output rules")
}

func TestSubAgentPrompt_ObservationOnlyByDefault(t *testing.T) {
	task := planexec.TaskPlan{ID: "t-1", Title: "Investigate", Description: "d", AcceptanceCriteria: "a"}
	got, err := planexec.RenderSubAgentPromptForTest(task, false, "")
	gt.NoError(t, err).Required()
	// allowWrites=false keeps the observation-only prohibition.
	gt.String(t, got).Contains("observation-only")
	gt.String(t, got).Contains("Do NOT post messages or mutate")
}

func TestSubAgentPrompt_WriteModeGrantsWrites(t *testing.T) {
	task := planexec.TaskPlan{ID: "t-1", Title: "Post", Description: "post the summary", AcceptanceCriteria: "posted"}
	got, err := planexec.RenderSubAgentPromptForTest(task, true, "")
	gt.NoError(t, err).Required()
	// allowWrites=true drops the observation-only prohibition entirely and
	// grants the write permission (guarded by "after ... enough supporting
	// information").
	gt.Bool(t, contains(got, "observation-only")).False()
	gt.Bool(t, contains(got, "Do NOT post messages or mutate")).False()
	gt.String(t, got).Contains("you MAY use it")
	gt.String(t, got).Contains("enough supporting information")
}

// A sub-agent's tools are pinned to the run's subject while its prompt is built
// from the planner's task text alone, so the host-supplied context block is the
// only way it learns the ids those tools need.
func TestSubAgentPrompt_RendersHostTaskContext(t *testing.T) {
	task := planexec.TaskPlan{ID: "t-1", Title: "Read", Description: "read the case thread", AcceptanceCriteria: "summarised"}
	ctxBlock := "- slack_channel_id: C0123456789\n- slack_thread_ts: 1700000000.000100"

	got, err := planexec.RenderSubAgentPromptForTest(task, false, ctxBlock)
	gt.NoError(t, err).Required()
	gt.String(t, got).Contains("## Run context")
	gt.String(t, got).Contains("- slack_channel_id: C0123456789")
	gt.String(t, got).Contains("- slack_thread_ts: 1700000000.000100")
	gt.String(t, got).Contains("never invent a Slack")
	// The task section must survive alongside it.
	gt.String(t, got).Contains("- ID: t-1")
}

// An empty block omits the section entirely rather than leaving an empty
// heading a model could read as "there is no such thing".
func TestSubAgentPrompt_OmitsEmptyTaskContext(t *testing.T) {
	task := planexec.TaskPlan{ID: "t-1", Title: "Read", Description: "d", AcceptanceCriteria: "a"}
	got, err := planexec.RenderSubAgentPromptForTest(task, false, "")
	gt.NoError(t, err).Required()
	gt.Bool(t, contains(got, "## Run context")).False()
	gt.String(t, got).Contains("## Your Task")
}

// ----- formatObservationsAsUserTurn ---------------------------------

func TestFormatObservations_RendersStatusAndCriteria(t *testing.T) {
	tasks := []planexec.TaskPlan{
		{ID: "t-1", Title: "A", AcceptanceCriteria: "X identified", Tools: []string{"slack_ro"}},
	}
	results := []planexec.TaskResult{
		{
			TaskID: "t-1", Title: "A", AcceptanceCriteria: "X identified",
			Status: planexec.TaskStatusCompleted, Summary: "We found the cause.",
		},
	}
	got := planexec.FormatObservationsForTest(tasks, results)
	gt.String(t, got).Contains("# Observations from prior investigations")
	gt.String(t, got).Contains("## t-1: A")
	gt.String(t, got).Contains("**Status**: completed")
	gt.String(t, got).Contains("**Acceptance criteria**: X identified")
	gt.String(t, got).Contains("We found the cause.")
}

func TestFormatObservations_FailedHasErrorBlock(t *testing.T) {
	tasks := []planexec.TaskPlan{
		{ID: "t-2", Title: "B", AcceptanceCriteria: "Y resolved", Tools: []string{"github"}},
	}
	results := []planexec.TaskResult{
		{
			TaskID: "t-2", Title: "B", AcceptanceCriteria: "Y resolved",
			Status: planexec.TaskStatusFailed, Error: "rate limited",
		},
	}
	got := planexec.FormatObservationsForTest(tasks, results)
	gt.String(t, got).Contains("**Status**: failed")
	gt.String(t, got).Contains("**Error**: rate limited")
}

// Running the tasks themselves is the strategy's job, not a helper's: a round
// spawns one child Process per task and folds their outcomes back in. That path
// is covered by TestPlanCollectReplanFinal, TestAFailedChildIsReportedToThePlanner
// and TestChildInheritsTheParentScopeWithItsOwnToolsets in strategy_test.go.
