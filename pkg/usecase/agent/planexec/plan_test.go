package planexec_test

import (
	"strings"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
)

var knownTools = []string{"core_ro", "slack_ro", "notion", "github"}

// ----- parsePlanResult (first round) -------------------------------

func TestParsePlanResult_OneTask(t *testing.T) {
	raw := []byte(`{
		"message": "looking into the thread",
		"tasks": [
			{
				"id": "t-1",
				"title": "Recent thread",
				"description": "Read the parent thread.",
				"acceptance_criteria": "Recent ten messages summarised.",
				"tools": ["slack_ro"]
			}
		]
	}`)
	p, err := planexec.ParsePlanResultForTest(raw, knownTools)
	gt.NoError(t, err).Required()
	gt.String(t, p.Message).Equal("looking into the thread")
	gt.Array(t, p.Tasks).Length(1).Required()
	gt.String(t, p.Tasks[0].ID).Equal("t-1")
	gt.String(t, p.Tasks[0].Title).Equal("Recent thread")
	gt.Array(t, p.Tasks[0].Tools).Length(1)
}

func TestParsePlanResult_RejectsZeroTasks(t *testing.T) {
	raw := []byte(`{"tasks": []}`)
	_, err := planexec.ParsePlanResultForTest(raw, knownTools)
	gt.Error(t, err)
}

func TestParsePlanResult_RejectsTooManyTasks(t *testing.T) {
	var parts []string
	for i := range 6 {
		parts = append(parts, `{"id":"t-`+string(rune('0'+i))+`","title":"t","description":"d","acceptance_criteria":"a","tools":["slack_ro"]}`)
	}
	raw := []byte(`{"tasks":[` + strings.Join(parts, ",") + `]}`)
	_, err := planexec.ParsePlanResultForTest(raw, knownTools)
	gt.Error(t, err)
}

func TestParsePlanResult_RejectsDuplicateTaskID(t *testing.T) {
	raw := []byte(`{"tasks":[
		{"id":"t-1","title":"a","description":"d","acceptance_criteria":"a","tools":["slack_ro"]},
		{"id":"t-1","title":"b","description":"d","acceptance_criteria":"a","tools":["slack_ro"]}
	]}`)
	_, err := planexec.ParsePlanResultForTest(raw, knownTools)
	gt.Error(t, err)
}

func TestParsePlanResult_RejectsUnknownToolID(t *testing.T) {
	raw := []byte(`{"tasks":[
		{"id":"t-1","title":"a","description":"d","acceptance_criteria":"a","tools":["fake_set"]}
	]}`)
	_, err := planexec.ParsePlanResultForTest(raw, knownTools)
	gt.Error(t, err)
}

func TestParsePlanResult_RejectsEmptyToolsList(t *testing.T) {
	raw := []byte(`{"tasks":[
		{"id":"t-1","title":"a","description":"d","acceptance_criteria":"a","tools":[]}
	]}`)
	_, err := planexec.ParsePlanResultForTest(raw, knownTools)
	gt.Error(t, err)
}

func TestParsePlanResult_RejectsMissingTitle(t *testing.T) {
	raw := []byte(`{"tasks":[
		{"id":"t-1","title":"","description":"d","acceptance_criteria":"a","tools":["slack_ro"]}
	]}`)
	_, err := planexec.ParsePlanResultForTest(raw, knownTools)
	gt.Error(t, err)
}

func TestParsePlanResult_RejectsBadJSON(t *testing.T) {
	_, err := planexec.ParsePlanResultForTest([]byte(`{not json`), knownTools)
	gt.Error(t, err)
}

// ----- parseReplanResult (subsequent rounds) -----------------------

func TestParseReplanResult_ContinueTasks(t *testing.T) {
	raw := []byte(`{"message":"need more","tasks":[
		{"id":"t-2","title":"Deeper dig","description":"d","acceptance_criteria":"a","tools":["slack_ro","github"]}
	]}`)
	r, err := planexec.ParseReplanResultForTest(raw, knownTools, false)
	gt.NoError(t, err).Required()
	gt.Array(t, r.Tasks).Length(1)
	gt.Value(t, r.Question).Nil()
}

func TestParseReplanResult_Terminate(t *testing.T) {
	// Tasks empty + Question nil → loop should exit.
	raw := []byte(`{"message":"done"}`)
	r, err := planexec.ParseReplanResultForTest(raw, knownTools, false)
	gt.NoError(t, err).Required()
	gt.Array(t, r.Tasks).Length(0)
	gt.Value(t, r.Question).Nil()
}

func TestParseReplanResult_QuestionAllowed(t *testing.T) {
	raw := []byte(`{"question":{
		"reason":"need disambiguation",
		"items":[{"id":"q1","text":"Which?","type":"select","options":["A","B"]}]
	}}`)
	r, err := planexec.ParseReplanResultForTest(raw, knownTools, true)
	gt.NoError(t, err).Required()
	gt.Value(t, r.Question).NotNil().Required()
	gt.String(t, r.Question.Reason).Equal("need disambiguation")
	gt.Array(t, r.Question.Items).Length(1)
}

func TestParseReplanResult_QuestionRejectedWhenDisabled(t *testing.T) {
	raw := []byte(`{"question":{
		"reason":"x",
		"items":[{"id":"q1","text":"?","type":"select","options":["a","b"]}]
	}}`)
	_, err := planexec.ParseReplanResultForTest(raw, knownTools, false)
	gt.Error(t, err)
}

func TestParseReplanResult_QuestionPriorityOverTasks(t *testing.T) {
	// Even if Tasks is set alongside Question, the parser should drop
	// Tasks because Question is the priority signal.
	raw := []byte(`{
		"question":{
			"reason":"x",
			"items":[{"id":"q1","text":"?","type":"select","options":["a","b"]}]
		},
		"tasks":[{"id":"t-1","title":"a","description":"d","acceptance_criteria":"a","tools":["slack_ro"]}]
	}`)
	r, err := planexec.ParseReplanResultForTest(raw, knownTools, true)
	gt.NoError(t, err).Required()
	gt.Value(t, r.Question).NotNil()
	gt.Array(t, r.Tasks).Length(0)
}

func TestParseReplanResult_FreeTextNoOptions(t *testing.T) {
	raw := []byte(`{"question":{
		"reason":"need a narrative",
		"items":[{"id":"q-summary","text":"What happened?","type":"free_text"}]
	}}`)
	r, err := planexec.ParseReplanResultForTest(raw, knownTools, true)
	gt.NoError(t, err).Required()
	gt.Value(t, r.Question.Items[0].Type).Equal(planexec.QuestionItemFreeText)
}

func TestParseReplanResult_FreeTextIgnoresOptions(t *testing.T) {
	// Options supplied alongside free_text are tolerated as a hint.
	raw := []byte(`{"question":{
		"reason":"x",
		"items":[{"id":"q1","text":"?","type":"free_text","options":["a","b"]}]
	}}`)
	r, err := planexec.ParseReplanResultForTest(raw, knownTools, true)
	gt.NoError(t, err).Required()
	gt.Value(t, r.Question.Items[0].Type).Equal(planexec.QuestionItemFreeText)
}

func TestParseReplanResult_RejectsSelectWithoutEnoughOptions(t *testing.T) {
	raw := []byte(`{"question":{
		"reason":"x",
		"items":[{"id":"q1","text":"?","type":"select","options":["only-one"]}]
	}}`)
	_, err := planexec.ParseReplanResultForTest(raw, knownTools, true)
	gt.Error(t, err)
}

func TestParseReplanResult_RejectsUnknownQuestionType(t *testing.T) {
	raw := []byte(`{"question":{
		"reason":"x",
		"items":[{"id":"q1","text":"?","type":"radio","options":["a","b"]}]
	}}`)
	_, err := planexec.ParseReplanResultForTest(raw, knownTools, true)
	gt.Error(t, err)
}

func TestParseReplanResult_RejectsDuplicateQuestionItemID(t *testing.T) {
	raw := []byte(`{"question":{
		"reason":"x",
		"items":[
			{"id":"q1","text":"?","type":"select","options":["a","b"]},
			{"id":"q1","text":"!","type":"select","options":["c","d"]}
		]
	}}`)
	_, err := planexec.ParseReplanResultForTest(raw, knownTools, true)
	gt.Error(t, err)
}

func TestParseReplanResult_RejectsTooManyQuestionItems(t *testing.T) {
	var items []string
	for i := range 6 {
		items = append(items, `{"id":"q-`+string(rune('0'+i))+`","text":"?","type":"select","options":["a","b"]}`)
	}
	raw := []byte(`{"question":{"reason":"x","items":[` + strings.Join(items, ",") + `]}}`)
	_, err := planexec.ParseReplanResultForTest(raw, knownTools, true)
	gt.Error(t, err)
}

// ----- extractJSONObject (LLM noise tolerance) ---------------------

func TestExtractJSONObject(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "clean object is returned unchanged",
			in:   `{"tasks":[]}`,
			want: `{"tasks":[]}`,
		},
		{
			name: "object with surrounding whitespace is trimmed",
			in:   "  \n {\"tasks\":[]}\n  ",
			want: `{"tasks":[]}`,
		},
		{
			name: "object wrapped in json code fence is unwrapped",
			in:   "```json\n{\"tasks\":[]}\n```",
			want: `{"tasks":[]}`,
		},
		{
			name: "object wrapped in bare code fence is unwrapped",
			in:   "```\n{\"tasks\":[]}\n```",
			want: `{"tasks":[]}`,
		},
		{
			name: "prose prefix before object is stripped",
			in:   `I'll respond with: {"tasks":[],"message":"ok"}`,
			want: `{"tasks":[],"message":"ok"}`,
		},
		{
			name: "object containing braces inside a string value is preserved",
			in:   `{"message":"contains } and { in text","tasks":[]}`,
			want: `{"message":"contains } and { in text","tasks":[]}`,
		},
		{
			name: "object with escaped quote in string is preserved",
			in:   `prefix {"message":"a \"quoted\" word","tasks":[]}`,
			want: `{"message":"a \"quoted\" word","tasks":[]}`,
		},
		{
			// Pins the removal of the first-and-last-char fast path
			// (proposal-side regression). Multiple top-level objects:
			// keep only the first.
			name: "multiple top-level objects keep only the first",
			in:   `{"tasks":[]} {"tasks":[{"id":"x"}]}`,
			want: `{"tasks":[]}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := planexec.ExtractJSONObjectForTest([]byte(c.in))
			gt.String(t, string(got)).Equal(c.want)
		})
	}
}

func TestParsePlanResult_TolerantOfPreamble(t *testing.T) {
	raw := []byte(`I'll respond with: {
		"message": "looking",
		"tasks": [
			{
				"id":"t-1","title":"Recent thread","description":"Read parent.",
				"acceptance_criteria":"Top ten summarised.","tools":["slack_ro"]
			}
		]
	}`)
	p, err := planexec.ParsePlanResultForTest(raw, knownTools)
	gt.NoError(t, err).Required()
	gt.Array(t, p.Tasks).Length(1)
}

// ----- schema shape ------------------------------------------------

func TestPlanSchema_Shape(t *testing.T) {
	raw := planexec.PlanSchemaForTest(knownTools, false)
	schema, ok := raw.(*gollem.Parameter)
	gt.Bool(t, ok).True().Required()
	gt.Value(t, schema.Type).Equal(gollem.TypeObject)
	gt.Map(t, schema.Properties).HasKey("tasks")
	gt.Map(t, schema.Properties).HasKey("message")
}

func TestReplanSchema_HasQuestionWhenAllowed(t *testing.T) {
	rawAllow := planexec.ReplanSchemaForTest(knownTools, true)
	schemaAllow := rawAllow.(*gollem.Parameter)
	gt.Map(t, schemaAllow.Properties).HasKey("question")

	rawDisallow := planexec.ReplanSchemaForTest(knownTools, false)
	schemaDisallow := rawDisallow.(*gollem.Parameter)
	_, has := schemaDisallow.Properties["question"]
	gt.Bool(t, has).False()
}
