package planexec

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

// DirectPromptInputForTest mirrors directPromptInput so tests can build inputs
// without re-importing the internal alias.
type DirectPromptInputForTest = directPromptInput

// RenderDirectPromptForTest exposes buildDirectSystemPrompt.
var RenderDirectPromptForTest = buildDirectSystemPrompt

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
// state carrying nextInput, with toolsAnswered saying whether the previous
// transitions answered the planner's tool calls in the conversation, and reports
// how many gollem inputs it contains.
//
// Both outcomes matter, in opposite directions. Zero is required once the calls
// are answered — the results are the turn the model is waiting on. Zero anywhere
// else is a broken request: gollem appends no user turn for an empty input, so it
// would END on the previous model turn ("Requests ending with a model turn are not
// supported").
func PlannerInputsForTest(nextInput string, toolsAnswered bool) int {
	return len(plannerInput(state{NextInput: nextInput, ToolsAnswered: toolsAnswered}))
}
