package proposal_test

import (
	"strings"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/proposal"
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
	p, err := proposal.ParseAndValidateForTest(raw)
	gt.NoError(t, err).Required()
	gt.Value(t, p.Action).Equal(proposal.ActionInvestigateForTest)
	gt.Value(t, p.Investigate).NotNil().Required()
	gt.Array(t, p.Investigate.Tasks).Length(1).Required()
	gt.Value(t, p.Investigate.Tasks[0].ID).Equal("inv-1")
}

func TestParseAndValidate_Question_SingleSelect(t *testing.T) {
	raw := []byte(`{
		"reasoning": "ask user to disambiguate",
		"action": "question",
		"question": {
			"reason": "multiple workspaces match",
			"items": [
				{"id":"q-ws","text":"Which workspace?","type":"select","options":["A","B"]}
			]
		}
	}`)
	p, err := proposal.ParseAndValidateForTest(raw)
	gt.NoError(t, err).Required()
	gt.Value(t, p.Action).Equal(proposal.ActionQuestionForTest)
	gt.Value(t, p.Question).NotNil().Required()
	gt.Array(t, p.Question.Items).Length(1).Required()
	gt.Value(t, p.Question.Items[0].ID).Equal("q-ws")
	gt.Value(t, p.Question.Items[0].Type).Equal(proposal.QuestionTypeSelectForTest)
	gt.Array(t, p.Question.Items[0].Options).Length(2)
}

// TestParseAndValidate_Question_FreeTextNoOptions confirms that a
// `free_text` item is accepted without any `options` field — the
// last-resort prose-input shape is exempt from the ≥2 entries rule.
func TestParseAndValidate_Question_FreeTextNoOptions(t *testing.T) {
	raw := []byte(`{
		"reasoning": "investigation produced nothing usable; the case substance is inherently prose",
		"action": "question",
		"question": {
			"reason": "need a narrative summary",
			"items": [
				{"id":"q-summary","text":"What is the case about?","type":"free_text"}
			]
		}
	}`)
	p, err := proposal.ParseAndValidateForTest(raw)
	gt.NoError(t, err).Required()
	gt.Array(t, p.Question.Items).Length(1).Required()
	gt.Value(t, p.Question.Items[0].Type).Equal(proposal.QuestionTypeFreeTextForTest)
	gt.Array(t, p.Question.Items[0].Options).Length(0)
}

// TestParseAndValidate_Question_FreeTextIgnoresOptions confirms that
// `options` supplied alongside a `free_text` item are tolerated (treated
// as a hint and discarded by the host) rather than triggering a
// validation failure.
func TestParseAndValidate_Question_FreeTextIgnoresOptions(t *testing.T) {
	raw := []byte(`{
		"reasoning": "x",
		"action": "question",
		"question": {
			"reason": "y",
			"items": [
				{"id":"q1","text":"prose?","type":"free_text","options":["this","that"]}
			]
		}
	}`)
	p, err := proposal.ParseAndValidateForTest(raw)
	gt.NoError(t, err).Required()
	gt.Value(t, p.Question.Items[0].Type).Equal(proposal.QuestionTypeFreeTextForTest)
}

// TestParseAndValidate_Question_RejectsSelectWithoutOptions guards the
// closed-list contract: `select` / `multi_select` still require ≥2
// options. The free_text exemption does not apply.
func TestParseAndValidate_Question_RejectsSelectWithoutOptions(t *testing.T) {
	raw := []byte(`{
		"reasoning": "x",
		"action": "question",
		"question": {
			"reason": "y",
			"items": [
				{"id":"q1","text":"choose","type":"select","options":["only-one"]}
			]
		}
	}`)
	_, err := proposal.ParseAndValidateForTest(raw)
	gt.Error(t, err)
}

func TestParseAndValidate_Question_MultiSelectMultipleItems(t *testing.T) {
	raw := []byte(`{
		"reasoning": "two pieces of info missing",
		"action": "question",
		"question": {
			"reason": "we need severity AND categories",
			"items": [
				{"id":"q-sev","text":"Severity?","type":"select","options":["low","high"]},
				{"id":"q-cat","text":"Categories?","type":"multi_select","options":["network","auth","data"]}
			]
		}
	}`)
	p, err := proposal.ParseAndValidateForTest(raw)
	gt.NoError(t, err).Required()
	gt.Array(t, p.Question.Items).Length(2).Required()
	gt.Value(t, p.Question.Items[1].Type).Equal(proposal.QuestionTypeMultiSelectForTest)
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
	p, err := proposal.ParseAndValidateForTest(raw)
	gt.NoError(t, err).Required()
	gt.Value(t, p.Materialize.WorkspaceID).Equal("ws-1")
	gt.Value(t, p.Materialize.Title).Equal("API outage")
	gt.Value(t, p.Materialize.CustomFieldValues["severity"]).Equal("high")
	// is_test omitted defaults to false.
	gt.Bool(t, p.Materialize.IsTest).False()
}

func TestParseAndValidate_MaterializeIsTest(t *testing.T) {
	raw := []byte(`{
		"reasoning": "user explicitly says this is a drill",
		"action": "materialize",
		"materialize": {
			"workspace_id": "ws-1",
			"title": "Test drill",
			"description": "Brief.",
			"custom_field_values": {},
			"is_test": true
		}
	}`)
	p, err := proposal.ParseAndValidateForTest(raw)
	gt.NoError(t, err).Required()
	gt.Bool(t, p.Materialize.IsTest).True()
}

func TestParseAndValidate_RejectsBadJSON(t *testing.T) {
	_, err := proposal.ParseAndValidateForTest([]byte(`{not json`))
	gt.Error(t, err)
}

func TestParseAndValidate_RejectsActionPayloadMismatch(t *testing.T) {
	t.Run("investigate with question payload", func(t *testing.T) {
		raw := []byte(`{
			"reasoning": "x",
			"action": "investigate",
			"question": {"reason":"r","items":[{"id":"q","text":"t","type":"select","options":["a","b"]}]}
		}`)
		_, err := proposal.ParseAndValidateForTest(raw)
		gt.Error(t, err)
	})
	t.Run("question but payload missing", func(t *testing.T) {
		raw := []byte(`{
			"reasoning": "x",
			"action": "question"
		}`)
		_, err := proposal.ParseAndValidateForTest(raw)
		gt.Error(t, err)
	})
	t.Run("multiple payloads set", func(t *testing.T) {
		raw := []byte(`{
			"reasoning": "x",
			"action": "question",
			"question": {"reason":"r","items":[{"id":"q","text":"t","type":"select","options":["a","b"]}]},
			"materialize": {"workspace_id": "ws", "title": "t"}
		}`)
		_, err := proposal.ParseAndValidateForTest(raw)
		gt.Error(t, err)
	})
}

func TestParseAndValidate_RejectsEmptyReasoning(t *testing.T) {
	raw := []byte(`{
		"reasoning": "   ",
		"action": "post_message",
		"post_message": {"text": "hi"}
	}`)
	_, err := proposal.ParseAndValidateForTest(raw)
	gt.Error(t, err)
}

func TestParseAndValidate_RejectsZeroOrTooManyTasks(t *testing.T) {
	t.Run("zero tasks", func(t *testing.T) {
		raw := []byte(`{
			"reasoning": "x",
			"action": "investigate",
			"investigate": {"tasks": []}
		}`)
		_, err := proposal.ParseAndValidateForTest(raw)
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
		_, err := proposal.ParseAndValidateForTest(raw)
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
	_, err := proposal.ParseAndValidateForTest(raw)
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
	_, err := proposal.ParseAndValidateForTest(raw)
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
	_, err := proposal.ParseAndValidateForTest(raw)
	gt.Error(t, err)
}

func TestParseAndValidate_RejectsQuestionOptionsTooFew(t *testing.T) {
	raw := []byte(`{
		"reasoning": "x",
		"action": "question",
		"question": {"reason":"y","items":[{"id":"q","text":"?","type":"select","options":["only"]}]}
	}`)
	_, err := proposal.ParseAndValidateForTest(raw)
	gt.Error(t, err)
}

func TestParseAndValidate_RejectsQuestionWithoutItems(t *testing.T) {
	raw := []byte(`{
		"reasoning": "x",
		"action": "question",
		"question": {"reason":"y","items":[]}
	}`)
	_, err := proposal.ParseAndValidateForTest(raw)
	gt.Error(t, err)
}

func TestParseAndValidate_RejectsQuestionUnknownType(t *testing.T) {
	raw := []byte(`{
		"reasoning": "x",
		"action": "question",
		"question": {"reason":"y","items":[{"id":"q","text":"?","type":"radio","options":["a","b"]}]}
	}`)
	_, err := proposal.ParseAndValidateForTest(raw)
	gt.Error(t, err)
}

func TestParseAndValidate_RejectsMaterializeMissingWorkspace(t *testing.T) {
	raw := []byte(`{
		"reasoning": "x",
		"action": "materialize",
		"materialize": {"title": "t"}
	}`)
	_, err := proposal.ParseAndValidateForTest(raw)
	gt.Error(t, err)
}

func TestPlanSchema_Shape(t *testing.T) {
	schema := proposal.PlanSchemaForTest()
	gt.Value(t, schema.Type).Equal(gollem.TypeObject)
	gt.Map(t, schema.Properties).HasKey("action")
	gt.Map(t, schema.Properties).HasKey("reasoning")
	gt.Map(t, schema.Properties).HasKey("investigate")
	gt.Map(t, schema.Properties).HasKey("question")
	gt.Map(t, schema.Properties).HasKey("materialize")
	// materialize exposes the optional is_test boolean to the planner.
	gt.Map(t, schema.Properties["materialize"].Properties).HasKey("is_test")
	gt.Value(t, schema.Properties["materialize"].Properties["is_test"].Type).Equal(gollem.TypeBoolean)
	// action enum covers the three planActions.
	actionEnum := schema.Properties["action"].Enum
	gt.Array(t, actionEnum).Length(3)
}

func TestFormatObservations_RendersStatusAndCriteria(t *testing.T) {
	inv := &proposal.PlanInvestigateForTest{
		Tasks: []proposal.PlanInvestigateTaskForTest{
			{ID: "inv-1", Title: "A", AcceptanceCriteria: "X identified", Tools: []string{"slack_ro"}},
		},
	}
	results := []proposal.InvestigationResultForTest{
		{
			TaskID: "inv-1", Title: "A", AcceptanceCriteria: "X identified",
			Status: proposal.InvestigationCompletedForTest, Summary: "We found the cause.",
		},
	}
	got := proposal.FormatObservationsForTest(inv, results)
	gt.String(t, got).Contains("# Observations from prior investigations")
	gt.String(t, got).Contains("## inv-1: A")
	gt.String(t, got).Contains("**Status**: completed")
	gt.String(t, got).Contains("**Acceptance criteria**: X identified")
	gt.String(t, got).Contains("We found the cause.")
}

func TestFormatObservations_FailedHasErrorBlock(t *testing.T) {
	inv := &proposal.PlanInvestigateForTest{
		Tasks: []proposal.PlanInvestigateTaskForTest{
			{ID: "inv-2", Title: "B", AcceptanceCriteria: "Y resolved", Tools: []string{"github"}},
		},
	}
	results := []proposal.InvestigationResultForTest{
		{
			TaskID: "inv-2", Title: "B", AcceptanceCriteria: "Y resolved",
			Status: proposal.InvestigationFailedForTest, Error: "rate limited",
		},
	}
	got := proposal.FormatObservationsForTest(inv, results)
	gt.String(t, got).Contains("**Status**: failed")
	gt.String(t, got).Contains("**Error**: rate limited")
}

// TestExtractJSONObject pins the LLM-noise tolerance added to
// parseAndValidate's decode path. Each case represents a real shape
// observed (or plausibly observable) in real-LLM planner output —
// clean object, ```json``` fence, bare ``` fence, prose preamble,
// nested braces inside string values. A regression here would
// re-introduce the "decode plan json: invalid character 'I' looking
// for beginning of value" flake that previously made the realLLM
// suite intermittently fail on a single sloppy turn.
func TestExtractJSONObject(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "clean object is returned unchanged",
			in:   `{"action":"investigate"}`,
			want: `{"action":"investigate"}`,
		},
		{
			name: "object with surrounding whitespace is trimmed",
			in:   "  \n {\"action\":\"question\"}\n  ",
			want: `{"action":"question"}`,
		},
		{
			name: "object wrapped in json code fence is unwrapped",
			in:   "```json\n{\"action\":\"materialize\"}\n```",
			want: `{"action":"materialize"}`,
		},
		{
			name: "object wrapped in bare code fence is unwrapped",
			in:   "```\n{\"action\":\"materialize\"}\n```",
			want: `{"action":"materialize"}`,
		},
		{
			name: "prose prefix before object is stripped",
			in:   `I'll respond with: {"action":"investigate","reasoning":"ok"}`,
			want: `{"action":"investigate","reasoning":"ok"}`,
		},
		{
			name: "object containing braces inside a string value is preserved",
			in:   `{"reasoning":"contains } and { in text","action":"question"}`,
			want: `{"reasoning":"contains } and { in text","action":"question"}`,
		},
		{
			name: "object with escaped quote in string is preserved",
			in:   `prefix {"reasoning":"a \"quoted\" word"}`,
			want: `{"reasoning":"a \"quoted\" word"}`,
		},
		{
			// Pins the removal of the first-and-last-char fast path
			// (gemini-code-assist PR #112 review). A previous
			// implementation returned this entire string because the
			// trimmed input started with `{` and ended with `}` —
			// json.Unmarshal then failed with "extra data after value".
			// The scanner must return only the first object.
			name: "multiple top-level objects keep only the first",
			in:   `{"action":"investigate"} {"action":"materialize"}`,
			want: `{"action":"investigate"}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := proposal.ExtractJSONObjectForTest([]byte(c.in))
			gt.String(t, string(got)).Equal(c.want)
		})
	}
}

// TestParseAndValidate_TolerantOfPreamble verifies the integration:
// a real planner output that starts with a prose preamble should
// still parse and validate, not fall through to the retry path.
func TestParseAndValidate_TolerantOfPreamble(t *testing.T) {
	raw := []byte(`I'll respond with: {
		"reasoning": "need more context",
		"action": "investigate",
		"investigate": {
			"message": "looking",
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
	p, err := proposal.ParseAndValidateForTest(raw)
	gt.NoError(t, err).Required()
	gt.Value(t, p.Action).Equal(proposal.ActionInvestigateForTest)
}
