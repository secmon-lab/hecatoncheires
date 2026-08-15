package planexec

import (
	"fmt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/toolcall"
)

// Test-only exports. The compiler enforces these never reach the
// production binary because the file ends in _test.go.

// ParsePlanResultForTest exposes parsePlanResult so external test
// packages (planexec_test) can exercise the parser without bringing
// the Runner online.
var ParsePlanResultForTest = parsePlanResult

// ParseReplanResultForTest exposes parseReplanResult.
var ParseReplanResultForTest = parseReplanResult

// ExtractJSONObjectForTest exposes extractJSONObject.
var ExtractJSONObjectForTest = extractJSONObject

// PlanSchemaForTest exposes planSchema (the first-round schema).
func PlanSchemaForTest(knownToolIDs []string, allowQuestion, allowDirect bool) any {
	return planSchema(schemaOptions{
		knownToolIDs:  knownToolIDs,
		allowQuestion: allowQuestion,
		allowDirect:   allowDirect,
	})
}

// ReplanSchemaForTest exposes replanSchema (the subsequent-round
// schema).
func ReplanSchemaForTest(knownToolIDs []string, allowQuestion bool) any {
	return replanSchema(schemaOptions{
		knownToolIDs:  knownToolIDs,
		allowQuestion: allowQuestion,
	})
}

// RenderSubAgentPromptForTest exposes buildSubAgentSystemPrompt.
var RenderSubAgentPromptForTest = buildSubAgentSystemPrompt

// FormatObservationsForTest exposes formatObservationsAsUserTurn.
var FormatObservationsForTest = formatObservationsAsUserTurn

// PlannerPromptInputForTest mirrors plannerPromptInput so tests can
// build inputs without re-importing the internal alias.
type PlannerPromptInputForTest = plannerPromptInput

// RenderPlannerPromptForTest exposes renderPlannerSystemPrompt.
var RenderPlannerPromptForTest = renderPlannerSystemPrompt

// FinalPromptInputForTest mirrors finalPromptInput.
type FinalPromptInputForTest = finalPromptInput

// RenderFinalUserPromptForTest exposes renderFinalUserPrompt.
var RenderFinalUserPromptForTest = renderFinalUserPrompt

// RenderObservationsForFinalForTest exposes renderObservationsForFinal.
var RenderObservationsForFinalForTest = renderObservationsForFinal

// PlannerSystemPromptForTest renders the planner system prompt for a state whose
// planning phase has spent `rounds` tool rounds, so a test can assert what the
// model is told about its allowance. The notice lives here rather than in a user
// turn because the turn that would carry it reports tool results, and such a turn
// may carry nothing else.
func PlannerSystemPromptForTest(rounds int) (string, error) {
	s := &strategy[TextResult]{cfg: Config[TextResult]{TextOnly: true}}
	return s.plannerPrompt(state{
		Input:             Input{SystemPrompt: "host prompt", KnownToolIDs: []string{"core_ro"}},
		PlannerToolRounds: rounds,
	})
}

// PlannerToolRoundsMaxForTest exposes the tool allowance a planning phase has.
const PlannerToolRoundsMaxForTest = plannerToolRoundsMax

// PlannerInputsForTest builds the user turn a planning call would send for a
// state carrying nextInput and `toolResponses` pending tool results, and reports
// how many gollem inputs it contains.
//
// Zero is the value that matters: gollem appends no user turn for an empty input,
// so the request would END on the previous model turn and the provider rejects it
// ("Requests ending with a model turn are not supported").
func PlannerInputsForTest(nextInput string, toolResponses int) int {
	st := state{NextInput: nextInput}
	for i := range toolResponses {
		st.ToolResponses = append(st.ToolResponses, toolcall.Response{
			ID: fmt.Sprintf("c%d", i), Name: "probe",
		})
	}
	return len(plannerInput(st))
}
