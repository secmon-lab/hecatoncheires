package planexec_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/pricing"
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
	p, err := planexec.ParsePlanResultForTest(raw, knownTools, false, nil)
	gt.NoError(t, err).Required()
	gt.String(t, p.Message).Equal("looking into the thread")
	gt.Array(t, p.Tasks).Length(1).Required()
	gt.String(t, p.Tasks[0].ID).Equal("t-1")
	gt.String(t, p.Tasks[0].Title).Equal("Recent thread")
	gt.Array(t, p.Tasks[0].Tools).Length(1)
}

func TestParsePlanResult_RejectsZeroTasks(t *testing.T) {
	raw := []byte(`{"tasks": []}`)
	_, err := planexec.ParsePlanResultForTest(raw, knownTools, false, nil)
	gt.Error(t, err)
}

func TestParsePlanResult_RejectsTooManyTasks(t *testing.T) {
	var parts []string
	for i := range 6 {
		parts = append(parts, `{"id":"t-`+string(rune('0'+i))+`","title":"t","description":"d","acceptance_criteria":"a","tools":["slack_ro"]}`)
	}
	raw := []byte(`{"tasks":[` + strings.Join(parts, ",") + `]}`)
	_, err := planexec.ParsePlanResultForTest(raw, knownTools, false, nil)
	gt.Error(t, err)
}

func TestParsePlanResult_RejectsDuplicateTaskID(t *testing.T) {
	raw := []byte(`{"tasks":[
		{"id":"t-1","title":"a","description":"d","acceptance_criteria":"a","tools":["slack_ro"]},
		{"id":"t-1","title":"b","description":"d","acceptance_criteria":"a","tools":["slack_ro"]}
	]}`)
	_, err := planexec.ParsePlanResultForTest(raw, knownTools, false, nil)
	gt.Error(t, err)
}

func TestParsePlanResult_RejectsUnknownToolID(t *testing.T) {
	raw := []byte(`{"tasks":[
		{"id":"t-1","title":"a","description":"d","acceptance_criteria":"a","tools":["fake_set"]}
	]}`)
	_, err := planexec.ParsePlanResultForTest(raw, knownTools, false, nil)
	gt.Error(t, err)
}

func TestParsePlanResult_RejectsEmptyToolsList(t *testing.T) {
	raw := []byte(`{"tasks":[
		{"id":"t-1","title":"a","description":"d","acceptance_criteria":"a","tools":[]}
	]}`)
	_, err := planexec.ParsePlanResultForTest(raw, knownTools, false, nil)
	gt.Error(t, err)
}

func TestParsePlanResult_RejectsMissingTitle(t *testing.T) {
	raw := []byte(`{"tasks":[
		{"id":"t-1","title":"","description":"d","acceptance_criteria":"a","tools":["slack_ro"]}
	]}`)
	_, err := planexec.ParsePlanResultForTest(raw, knownTools, false, nil)
	gt.Error(t, err)
}

func TestParsePlanResult_RejectsBadJSON(t *testing.T) {
	_, err := planexec.ParsePlanResultForTest([]byte(`{not json`), knownTools, false, nil)
	gt.Error(t, err)
}

// ----- parsePlanResult: direct path --------------------------------

func TestParsePlanResult_DirectWithTools(t *testing.T) {
	raw := []byte(`{"message":"answering now","direct":{"tools":["slack_ro"]}}`)
	p, err := planexec.ParsePlanResultForTest(raw, knownTools, true, nil)
	gt.NoError(t, err).Required()
	gt.Value(t, p.Direct).NotNil().Required()
	gt.Array(t, p.Direct.Tools).Length(1)
	gt.String(t, p.Direct.Tools[0]).Equal("slack_ro")
	gt.Array(t, p.Tasks).Length(0)
	gt.String(t, p.Message).Equal("answering now")
}

func TestParsePlanResult_DirectWithoutTools(t *testing.T) {
	// A pure conversational reply needs no tools — empty tools is valid.
	raw := []byte(`{"message":"ok","direct":{}}`)
	p, err := planexec.ParsePlanResultForTest(raw, knownTools, true, nil)
	gt.NoError(t, err).Required()
	gt.Value(t, p.Direct).NotNil().Required()
	gt.Array(t, p.Direct.Tools).Length(0)
}

func TestParsePlanResult_RejectsDirectWhenNotAllowed(t *testing.T) {
	raw := []byte(`{"direct":{"tools":["slack_ro"]}}`)
	_, err := planexec.ParsePlanResultForTest(raw, knownTools, false, nil)
	gt.Error(t, err)
}

func TestParsePlanResult_RejectsDirectAndTasksTogether(t *testing.T) {
	raw := []byte(`{
		"direct":{"tools":["slack_ro"]},
		"tasks":[{"id":"t-1","title":"a","description":"d","acceptance_criteria":"a","tools":["slack_ro"]}]
	}`)
	_, err := planexec.ParsePlanResultForTest(raw, knownTools, true, nil)
	gt.Error(t, err)
}

func TestParsePlanResult_RejectsDirectUnknownToolID(t *testing.T) {
	raw := []byte(`{"direct":{"tools":["fake_set"]}}`)
	_, err := planexec.ParsePlanResultForTest(raw, knownTools, true, nil)
	gt.Error(t, err)
}

func TestParsePlanResult_RejectsDirectTooManyTools(t *testing.T) {
	raw := []byte(`{"direct":{"tools":["core_ro","slack_ro","notion","github","core_ro"]}}`)
	_, err := planexec.ParsePlanResultForTest(raw, knownTools, true, nil)
	gt.Error(t, err)
}

// ----- parseReplanResult (subsequent rounds) -----------------------

func TestParseReplanResult_ContinueTasks(t *testing.T) {
	raw := []byte(`{"message":"need more","tasks":[
		{"id":"t-2","title":"Deeper dig","description":"d","acceptance_criteria":"a","tools":["slack_ro","github"]}
	]}`)
	r, err := planexec.ParseReplanResultForTest(raw, knownTools, false, nil)
	gt.NoError(t, err).Required()
	gt.Array(t, r.Tasks).Length(1)
	gt.Value(t, r.Question).Nil()
}

func TestParseReplanResult_Finalize(t *testing.T) {
	// An explicit finalize is the ONLY way to terminate; it carries an optional
	// reason and leaves tasks / question empty.
	raw := []byte(`{"message":"done","finalize":{"reason":"goal met"}}`)
	r, err := planexec.ParseReplanResultForTest(raw, knownTools, false, nil)
	gt.NoError(t, err).Required()
	gt.Array(t, r.Tasks).Length(0)
	gt.Value(t, r.Question).Nil()
	gt.Value(t, r.Finalize).NotNil().Required()
	gt.String(t, r.Finalize.Reason).Equal("goal met")
}

func TestParseReplanResult_NoActionRejected(t *testing.T) {
	// The old implicit "empty tasks + no question = terminate" is gone: an
	// output that sets none of tasks / question / finalize is rejected so the
	// caller folds it back into another replan round.
	raw := []byte(`{"message":"done"}`)
	_, err := planexec.ParseReplanResultForTest(raw, knownTools, false, nil)
	gt.Error(t, err)
}

func TestParseReplanResult_MultipleActionsRejected(t *testing.T) {
	// Setting more than one action is ambiguous and rejected.
	raw := []byte(`{
		"tasks":[{"id":"t-1","title":"a","description":"d","acceptance_criteria":"a","tools":["slack_ro"]}],
		"finalize":{"reason":"done"}
	}`)
	_, err := planexec.ParseReplanResultForTest(raw, knownTools, true, nil)
	gt.Error(t, err)
}

func TestParseReplanResult_QuestionAllowed(t *testing.T) {
	raw := []byte(`{"question":{
		"reason":"need disambiguation",
		"items":[{"id":"q1","text":"Which?","type":"select","options":["A","B"]}]
	}}`)
	r, err := planexec.ParseReplanResultForTest(raw, knownTools, true, nil)
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
	_, err := planexec.ParseReplanResultForTest(raw, knownTools, false, nil)
	gt.Error(t, err)
}

func TestParseReplanResult_QuestionAndTasksRejected(t *testing.T) {
	// Question set alongside Tasks is two actions → rejected as ambiguous
	// (the old behaviour silently dropped Tasks; now the planner must pick one).
	raw := []byte(`{
		"question":{
			"reason":"x",
			"items":[{"id":"q1","text":"?","type":"select","options":["a","b"]}]
		},
		"tasks":[{"id":"t-1","title":"a","description":"d","acceptance_criteria":"a","tools":["slack_ro"]}]
	}`)
	_, err := planexec.ParseReplanResultForTest(raw, knownTools, true, nil)
	gt.Error(t, err)
}

func TestParseReplanResult_FreeTextNoOptions(t *testing.T) {
	raw := []byte(`{"question":{
		"reason":"need a narrative",
		"items":[{"id":"q-summary","text":"What happened?","type":"free_text"}]
	}}`)
	r, err := planexec.ParseReplanResultForTest(raw, knownTools, true, nil)
	gt.NoError(t, err).Required()
	gt.Value(t, r.Question.Items[0].Type).Equal(planexec.QuestionItemFreeText)
}

func TestParseReplanResult_FreeTextIgnoresOptions(t *testing.T) {
	// Options supplied alongside free_text are tolerated as a hint.
	raw := []byte(`{"question":{
		"reason":"x",
		"items":[{"id":"q1","text":"?","type":"free_text","options":["a","b"]}]
	}}`)
	r, err := planexec.ParseReplanResultForTest(raw, knownTools, true, nil)
	gt.NoError(t, err).Required()
	gt.Value(t, r.Question.Items[0].Type).Equal(planexec.QuestionItemFreeText)
}

func TestParseReplanResult_RejectsSelectWithoutEnoughOptions(t *testing.T) {
	raw := []byte(`{"question":{
		"reason":"x",
		"items":[{"id":"q1","text":"?","type":"select","options":["only-one"]}]
	}}`)
	_, err := planexec.ParseReplanResultForTest(raw, knownTools, true, nil)
	gt.Error(t, err)
}

func TestParseReplanResult_RejectsUnknownQuestionType(t *testing.T) {
	raw := []byte(`{"question":{
		"reason":"x",
		"items":[{"id":"q1","text":"?","type":"radio","options":["a","b"]}]
	}}`)
	_, err := planexec.ParseReplanResultForTest(raw, knownTools, true, nil)
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
	_, err := planexec.ParseReplanResultForTest(raw, knownTools, true, nil)
	gt.Error(t, err)
}

func TestParseReplanResult_RejectsTooManyQuestionItems(t *testing.T) {
	var items []string
	for i := range 6 {
		items = append(items, `{"id":"q-`+string(rune('0'+i))+`","text":"?","type":"select","options":["a","b"]}`)
	}
	raw := []byte(`{"question":{"reason":"x","items":[` + strings.Join(items, ",") + `]}}`)
	_, err := planexec.ParseReplanResultForTest(raw, knownTools, true, nil)
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
	p, err := planexec.ParsePlanResultForTest(raw, knownTools, false, nil)
	gt.NoError(t, err).Required()
	gt.Array(t, p.Tasks).Length(1)
}

// ----- per-task budgets --------------------------------------------

// taskWithBudget renders one task, with a budget when usd is positive and without
// one otherwise, so the cases below differ only in the thing under test.
func taskWithBudget(id string, usd float64) string {
	base := `{"id":"` + id + `","title":"t","description":"d",` +
		`"acceptance_criteria":"a","tools":["slack_ro"]`
	if usd == 0 {
		return base + `}`
	}
	return base + `,"budget_usd":` + strconv.FormatFloat(usd, 'f', -1, 64) + `}`
}

func plannedTasks(tasks ...string) []byte {
	return []byte(`{"tasks":[` + strings.Join(tasks, ",") + `]}`)
}

// TestTaskBudgetsAreValidatedAgainstWhatIsLeft covers the whole contract the
// planner is held to once a host wires Config.Remaining: every task carries a
// positive budget, and a round's budgets add up to no more than the run has left.
//
// The remaining figure is a POINTER rather than a plain amount because zero is a
// legitimate allowance: reading zero as "do not check" would drop the check at
// exactly the moment it matters.
func TestTaskBudgetsAreValidatedAgainstWhatIsLeft(t *testing.T) {
	remaining := pricing.FromUSD(0.20)

	testCases := map[string]struct {
		raw     []byte
		wantErr bool
	}{
		"one task inside the allowance": {
			raw: plannedTasks(taskWithBudget("t-1", 0.10)),
		},
		"two tasks summing to less than the allowance": {
			raw: plannedTasks(taskWithBudget("t-1", 0.10), taskWithBudget("t-2", 0.05)),
		},
		// The boundary is inclusive: spending exactly what is left is a legitimate
		// plan, and rejecting it would leave an allowance nothing could ever claim.
		"two tasks summing to exactly the allowance": {
			raw: plannedTasks(taskWithBudget("t-1", 0.12), taskWithBudget("t-2", 0.08)),
		},
		"a missing budget is rejected": {
			raw:     plannedTasks(taskWithBudget("t-1", 0)),
			wantErr: true,
		},
		"a zero budget is rejected": {
			raw:     plannedTasks(`{"id":"t-1","title":"t","description":"d","acceptance_criteria":"a","tools":["slack_ro"],"budget_usd":0}`),
			wantErr: true,
		},
		"a negative budget is rejected": {
			raw:     plannedTasks(taskWithBudget("t-1", -0.05)),
			wantErr: true,
		},
		// The per-task check alone would let every one of five tasks claim the whole
		// allowance, which is the state this field exists to end.
		"budgets summing past the allowance are rejected": {
			raw:     plannedTasks(taskWithBudget("t-1", 0.15), taskWithBudget("t-2", 0.10)),
			wantErr: true,
		},
		"one task past the allowance is rejected": {
			raw:     plannedTasks(taskWithBudget("t-1", 0.30)),
			wantErr: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			_, planErr := planexec.ParsePlanResultForTest(tc.raw, knownTools, false, &remaining)
			// The replan round takes the same tasks through the same validation, so a
			// check that held on one round and not the other would be a gap.
			_, replanErr := planexec.ParseReplanResultForTest(tc.raw, knownTools, false, &remaining)
			if tc.wantErr {
				gt.Value(t, planErr).NotNil()
				gt.Value(t, replanErr).NotNil()
				return
			}
			gt.NoError(t, planErr)
			gt.NoError(t, replanErr)
		})
	}
}

// TestAnUnconvertibleTaskBudgetIsRejected pins the direction that matters about a
// bad figure: it must not grant MORE money than a good one.
//
// `budget_usd` is a float64 from a model and pricing.FromUSD does not saturate, so
// a figure above roughly 9.2e9 lands negative in NanoUSD. Checked as a float it
// looks positive; unchecked in NanoUSD it would make the round's sum negative and
// clear the sum check, and WithBudget would then read the non-positive amount as
// "unset" and hand the child the deployment default.
func TestAnUnconvertibleTaskBudgetIsRejected(t *testing.T) {
	remaining := pricing.FromUSD(1)
	raw := plannedTasks(
		`{"id":"t-1","title":"t","description":"d","acceptance_criteria":"a",` +
			`"tools":["slack_ro"],"budget_usd":1e13}`)

	_, err := planexec.ParsePlanResultForTest(raw, knownTools, false, &remaining)
	gt.Value(t, err).NotNil()
	_, err = planexec.ParseReplanResultForTest(raw, knownTools, false, &remaining)
	gt.Value(t, err).NotNil()
}

// TestTaskBudgetsAreNotCheckedWithoutARemainingFigure pins the backward-compatible
// half: a host that wired no Config.Remaining is not asking the planner to
// allocate anything, so a plan without budgets is accepted and its children keep
// inheriting the run's own figure.
func TestTaskBudgetsAreNotCheckedWithoutARemainingFigure(t *testing.T) {
	raw := plannedTasks(taskWithBudget("t-1", 0))

	p, err := planexec.ParsePlanResultForTest(raw, knownTools, false, nil)
	gt.NoError(t, err).Required()
	gt.Array(t, p.Tasks).Length(1).Required()
	gt.Number(t, p.Tasks[0].BudgetUSD).Equal(0)

	_, err = planexec.ParseReplanResultForTest(raw, knownTools, false, nil)
	gt.NoError(t, err)
}

// TestTaskBudgetIsCarriedThroughTheParser pins that the figure survives decoding:
// it is what the strategy stamps onto the child's metadata, so a dropped value
// would silently hand the child the run's whole budget.
func TestTaskBudgetIsCarriedThroughTheParser(t *testing.T) {
	remaining := pricing.FromUSD(1)
	p, err := planexec.ParsePlanResultForTest(
		plannedTasks(taskWithBudget("t-1", 0.25), taskWithBudget("t-2", 0.5)),
		knownTools, false, &remaining)
	gt.NoError(t, err).Required()
	gt.Array(t, p.Tasks).Length(2).Required()
	gt.Number(t, p.Tasks[0].BudgetUSD).Equal(0.25)
	gt.Number(t, p.Tasks[1].BudgetUSD).Equal(0.5)
}

// TestBudgetPropertyFollowsTheRemainingFigure pins that the prompt's instruction
// and the schema property that makes it actionable travel together. A schema
// offering a field the host never reads invites a number nothing uses; one
// withholding a field the prompt demands drives the planner into a rejection loop.
func TestBudgetPropertyFollowsTheRemainingFigure(t *testing.T) {
	budgetProp := func(raw any) (*gollem.Parameter, bool) {
		schema, ok := raw.(*gollem.Parameter)
		gt.Bool(t, ok).True().Required()
		tasks := schema.Properties["tasks"]
		gt.Value(t, tasks).NotNil().Required()
		p, has := tasks.Items.Properties["budget_usd"]
		return p, has
	}

	for name, raw := range map[string]any{
		"plan":   planexec.PlanSchemaForTest(knownTools, false, false, true),
		"replan": planexec.ReplanSchemaForTest(knownTools, false, true),
	} {
		t.Run(name+" offers it when a remaining figure is supplied", func(t *testing.T) {
			p, has := budgetProp(raw)
			gt.Bool(t, has).True().Required()
			gt.Value(t, p.Type).Equal(gollem.TypeNumber)
			// Required for the same reason every other task field is: a model that
			// omits it sends the round into a retry loop, and the retries are paid for
			// out of the allowance being divided.
			gt.Bool(t, p.Required).True()
		})
	}

	for name, raw := range map[string]any{
		"plan":   planexec.PlanSchemaForTest(knownTools, false, false, false),
		"replan": planexec.ReplanSchemaForTest(knownTools, false, false),
	} {
		t.Run(name+" withholds it when no remaining figure is supplied", func(t *testing.T) {
			_, has := budgetProp(raw)
			gt.Bool(t, has).False()
		})
	}
}

// TestDirectCarriesNoBudgetProperty pins that the direct path is not asked to
// allocate. It spawns ONE child whose text is the reply, so there is nothing to
// divide and no decision for the planner to make — the child gets what is left.
func TestDirectCarriesNoBudgetProperty(t *testing.T) {
	raw := planexec.PlanSchemaForTest(knownTools, false, true, true)
	schema, ok := raw.(*gollem.Parameter)
	gt.Bool(t, ok).True().Required()
	direct := schema.Properties["direct"]
	gt.Value(t, direct).NotNil().Required()
	_, has := direct.Properties["budget_usd"]
	gt.Bool(t, has).False()
}

// ----- schema shape ------------------------------------------------

func TestPlanSchema_Shape(t *testing.T) {
	raw := planexec.PlanSchemaForTest(knownTools, false, false, false)
	schema, ok := raw.(*gollem.Parameter)
	gt.Bool(t, ok).True().Required()
	gt.Value(t, schema.Type).Equal(gollem.TypeObject)
	gt.Map(t, schema.Properties).HasKey("tasks")
	gt.Map(t, schema.Properties).HasKey("message")
	// direct is absent unless allowDirect is set.
	_, hasDirect := schema.Properties["direct"]
	gt.Bool(t, hasDirect).False()
}

func TestPlanSchema_HasDirectWhenAllowed(t *testing.T) {
	rawAllow := planexec.PlanSchemaForTest(knownTools, false, true, false)
	schemaAllow := rawAllow.(*gollem.Parameter)
	gt.Map(t, schemaAllow.Properties).HasKey("direct")
	direct := schemaAllow.Properties["direct"]
	gt.Value(t, direct.Type).Equal(gollem.TypeObject)
	gt.Map(t, direct.Properties).HasKey("tools")

	rawDisallow := planexec.PlanSchemaForTest(knownTools, false, false, false)
	schemaDisallow := rawDisallow.(*gollem.Parameter)
	_, has := schemaDisallow.Properties["direct"]
	gt.Bool(t, has).False()
}

// The `message` description must tell the planner both things, on both rounds,
// because each half prevents a different failure. NOT user-facing: no code reads
// the field, so a planner that thinks it is writing to the user puts the answer
// somewhere discarded and the turn ends with the reply nowhere — which is what the
// previous wording ("rationale shown to the user") invited. Kept: the planner reply
// carrying it is committed to the run's conversation and recorded as that
// transition's LLM_RESPONSE, so it is where an operator reads why a turn decided as
// it did, and a planner told only "nobody reads this" has every reason to emit a
// placeholder instead.
//
// It must not drift from prompts/planner.md either — a schema contradicting the
// prompt on this exact point decides where the answer goes.
func TestSchemas_DescribeMessageAsInternalButKept(t *testing.T) {
	for name, raw := range map[string]any{
		"plan":   planexec.PlanSchemaForTest(knownTools, false, true, false),
		"replan": planexec.ReplanSchemaForTest(knownTools, true, false),
	} {
		t.Run(name, func(t *testing.T) {
			schema, ok := raw.(*gollem.Parameter)
			gt.Bool(t, ok).True().Required()
			msg, has := schema.Properties["message"]
			gt.Bool(t, has).True().Required()
			gt.String(t, msg.Description).Contains("NOT shown to the user")
			gt.String(t, msg.Description).Contains("kept in this run's record")
			gt.String(t, msg.Description).NotContains("rationale shown to the user")
		})
	}
}

func TestReplanSchema_HasQuestionWhenAllowed(t *testing.T) {
	rawAllow := planexec.ReplanSchemaForTest(knownTools, true, false)
	schemaAllow := rawAllow.(*gollem.Parameter)
	gt.Map(t, schemaAllow.Properties).HasKey("question")

	rawDisallow := planexec.ReplanSchemaForTest(knownTools, false, false)
	schemaDisallow := rawDisallow.(*gollem.Parameter)
	_, has := schemaDisallow.Properties["question"]
	gt.Bool(t, has).False()
}
