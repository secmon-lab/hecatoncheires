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
func PlanSchemaForTest(knownToolIDs []string, allowQuestion bool) any {
	return planSchema(schemaOptions{
		knownToolIDs:  knownToolIDs,
		allowQuestion: allowQuestion,
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

// ExecutePhaseForTest exposes executePhase.
var ExecutePhaseForTest = executePhase

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

// GenerateFinalResponseForTest exposes generateFinalResponse.
var GenerateFinalResponseForTest = generateFinalResponse
