package draft_test

import (
	"strings"
	"testing"

	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/draft"
)

func TestParseAndValidate_Investigate(t *testing.T) {
	raw := []byte(`{
		"reasoning": "need more context",
		"action": "investigate",
		"investigate": {
			"message": "Looking into A",
			"tasks": [
				{
					"id": "inv-1",
					"title": "Recent thread",
					"description": "Read the parent thread.",
					"acceptance_criteria": "Recent ten messages summarised.",
					"tools": ["slack_ro"]
				}
			]
		}
	}`)
	p, err := draft.ParseAndValidateForTest(raw)
	gt.NoError(t, err).Required()
	gt.Value(t, p.Action).Equal(draft.ActionInvestigateForTest)
	gt.Value(t, p.Investigate).NotNil().Required()
	gt.Array(t, p.Investigate.Tasks).Length(1).Required()
	gt.Value(t, p.Investigate.Tasks[0].ID).Equal("inv-1")
}

func TestParseAndValidate_PostMessage(t *testing.T) {
	raw := []byte(`{
		"reasoning": "we already know enough",
		"action": "post_message",
		"post_message": { "text": "Here is the summary." }
	}`)
	p, err := draft.ParseAndValidateForTest(raw)
	gt.NoError(t, err).Required()
	gt.Value(t, p.Action).Equal(draft.ActionPostMessageForTest)
	gt.Value(t, p.PostMessage).NotNil().Required()
	gt.Value(t, p.PostMessage.Text).Equal("Here is the summary.")
}

func TestParseAndValidate_PostQuestion_WithOptions(t *testing.T) {
	raw := []byte(`{
		"reasoning": "ask user to disambiguate",
		"action": "post_question",
		"post_question": {
			"text": "Which workspace?",
			"reason": "multiple workspaces match",
			"options": ["A", "B"]
		}
	}`)
	p, err := draft.ParseAndValidateForTest(raw)
	gt.NoError(t, err).Required()
	gt.Value(t, p.PostQuestion).NotNil().Required()
	gt.Array(t, p.PostQuestion.Options).Length(2)
}

func TestParseAndValidate_Materialize(t *testing.T) {
	raw := []byte(`{
		"reasoning": "all info gathered",
		"action": "materialize",
		"materialize": {
			"workspace_id": "ws-1",
			"title": "API outage",
			"description": "Brief.",
			"custom_field_values": {"severity": "high"}
		}
	}`)
	p, err := draft.ParseAndValidateForTest(raw)
	gt.NoError(t, err).Required()
	gt.Value(t, p.Materialize.WorkspaceID).Equal("ws-1")
	gt.Value(t, p.Materialize.Title).Equal("API outage")
	gt.Value(t, p.Materialize.CustomFieldValues["severity"]).Equal("high")
}

func TestParseAndValidate_RejectsBadJSON(t *testing.T) {
	_, err := draft.ParseAndValidateForTest([]byte(`{not json`))
	gt.Error(t, err)
}

func TestParseAndValidate_RejectsActionPayloadMismatch(t *testing.T) {
	t.Run("investigate with post_message payload", func(t *testing.T) {
		raw := []byte(`{
			"reasoning": "x",
			"action": "investigate",
			"post_message": {"text": "hi"}
		}`)
		_, err := draft.ParseAndValidateForTest(raw)
		gt.Error(t, err)
	})
	t.Run("post_message but payload missing", func(t *testing.T) {
		raw := []byte(`{
			"reasoning": "x",
			"action": "post_message"
		}`)
		_, err := draft.ParseAndValidateForTest(raw)
		gt.Error(t, err)
	})
	t.Run("multiple payloads set", func(t *testing.T) {
		raw := []byte(`{
			"reasoning": "x",
			"action": "post_message",
			"post_message": {"text": "hi"},
			"materialize": {"workspace_id": "ws", "title": "t"}
		}`)
		_, err := draft.ParseAndValidateForTest(raw)
		gt.Error(t, err)
	})
}

func TestParseAndValidate_RejectsEmptyReasoning(t *testing.T) {
	raw := []byte(`{
		"reasoning": "   ",
		"action": "post_message",
		"post_message": {"text": "hi"}
	}`)
	_, err := draft.ParseAndValidateForTest(raw)
	gt.Error(t, err)
}

func TestParseAndValidate_RejectsZeroOrTooManyTasks(t *testing.T) {
	t.Run("zero tasks", func(t *testing.T) {
		raw := []byte(`{
			"reasoning": "x",
			"action": "investigate",
			"investigate": {"tasks": []}
		}`)
		_, err := draft.ParseAndValidateForTest(raw)
		gt.Error(t, err)
	})
	t.Run("six tasks", func(t *testing.T) {
		var tasks []string
		for i := range 6 {
			tasks = append(tasks, `{"id":"inv-`+string(rune('0'+i))+`","title":"t","description":"d","acceptance_criteria":"a","tools":["slack_ro"]}`)
		}
		raw := []byte(`{
			"reasoning": "x",
			"action": "investigate",
			"investigate": {"tasks": [` + strings.Join(tasks, ",") + `]}
		}`)
		_, err := draft.ParseAndValidateForTest(raw)
		gt.Error(t, err)
	})
}

func TestParseAndValidate_RejectsDuplicateTaskID(t *testing.T) {
	raw := []byte(`{
		"reasoning": "x",
		"action": "investigate",
		"investigate": {"tasks": [
			{"id":"inv-1","title":"a","description":"d","acceptance_criteria":"a","tools":["slack_ro"]},
			{"id":"inv-1","title":"b","description":"d","acceptance_criteria":"a","tools":["slack_ro"]}
		]}
	}`)
	_, err := draft.ParseAndValidateForTest(raw)
	gt.Error(t, err)
}

func TestParseAndValidate_RejectsUnknownToolID(t *testing.T) {
	raw := []byte(`{
		"reasoning": "x",
		"action": "investigate",
		"investigate": {"tasks": [
			{"id":"inv-1","title":"a","description":"d","acceptance_criteria":"a","tools":["fake_set"]}
		]}
	}`)
	_, err := draft.ParseAndValidateForTest(raw)
	gt.Error(t, err)
}

func TestParseAndValidate_RejectsEmptyToolsList(t *testing.T) {
	raw := []byte(`{
		"reasoning": "x",
		"action": "investigate",
		"investigate": {"tasks": [
			{"id":"inv-1","title":"a","description":"d","acceptance_criteria":"a","tools":[]}
		]}
	}`)
	_, err := draft.ParseAndValidateForTest(raw)
	gt.Error(t, err)
}

func TestParseAndValidate_RejectsQuestionOptionsTooFew(t *testing.T) {
	raw := []byte(`{
		"reasoning": "x",
		"action": "post_question",
		"post_question": {"text": "?", "reason": "y", "options": ["only"]}
	}`)
	_, err := draft.ParseAndValidateForTest(raw)
	gt.Error(t, err)
}

func TestParseAndValidate_QuestionWithoutOptionsOK(t *testing.T) {
	raw := []byte(`{
		"reasoning": "x",
		"action": "post_question",
		"post_question": {"text": "?", "reason": "y"}
	}`)
	p, err := draft.ParseAndValidateForTest(raw)
	gt.NoError(t, err).Required()
	gt.Array(t, p.PostQuestion.Options).Length(0)
}

func TestParseAndValidate_RejectsMaterializeMissingWorkspace(t *testing.T) {
	raw := []byte(`{
		"reasoning": "x",
		"action": "materialize",
		"materialize": {"title": "t"}
	}`)
	_, err := draft.ParseAndValidateForTest(raw)
	gt.Error(t, err)
}

func TestPlanSchema_Shape(t *testing.T) {
	schema := draft.PlanSchemaForTest()
	gt.Value(t, schema.Type).Equal(gollem.TypeObject)
	gt.Map(t, schema.Properties).HasKey("action")
	gt.Map(t, schema.Properties).HasKey("reasoning")
	gt.Map(t, schema.Properties).HasKey("investigate")
	gt.Map(t, schema.Properties).HasKey("materialize")
	// action enum covers all four planActions.
	actionEnum := schema.Properties["action"].Enum
	gt.Array(t, actionEnum).Length(4)
}

func TestFormatObservations_RendersStatusAndCriteria(t *testing.T) {
	inv := &draft.PlanInvestigateForTest{
		Tasks: []draft.PlanInvestigateTaskForTest{
			{ID: "inv-1", Title: "A", AcceptanceCriteria: "X identified", Tools: []string{"slack_ro"}},
		},
	}
	results := []draft.InvestigationResultForTest{
		{
			TaskID: "inv-1", Title: "A", AcceptanceCriteria: "X identified",
			Status: draft.InvestigationCompletedForTest, Summary: "We found the cause.",
		},
	}
	got := draft.FormatObservationsForTest(inv, results)
	gt.String(t, got).Contains("# Observations from prior investigations")
	gt.String(t, got).Contains("## inv-1: A")
	gt.String(t, got).Contains("**Status**: completed")
	gt.String(t, got).Contains("**Acceptance criteria**: X identified")
	gt.String(t, got).Contains("We found the cause.")
}

func TestFormatObservations_FailedHasErrorBlock(t *testing.T) {
	inv := &draft.PlanInvestigateForTest{
		Tasks: []draft.PlanInvestigateTaskForTest{
			{ID: "inv-2", Title: "B", AcceptanceCriteria: "Y resolved", Tools: []string{"github"}},
		},
	}
	results := []draft.InvestigationResultForTest{
		{
			TaskID: "inv-2", Title: "B", AcceptanceCriteria: "Y resolved",
			Status: draft.InvestigationFailedForTest, Error: "rate limited",
		},
	}
	got := draft.FormatObservationsForTest(inv, results)
	gt.String(t, got).Contains("**Status**: failed")
	gt.String(t, got).Contains("**Error**: rate limited")
}
