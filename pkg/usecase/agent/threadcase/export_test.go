package threadcase

// Test-only seams for unit-testing the unexported prompt / decision helpers
// without exporting them into the production API.
var (
	BuildSystemPromptForTest      = buildSystemPrompt
	BuildUserInputForTest         = buildUserInput
	ParseDecisionForTest          = parseDecision
	ValidateCreateDecisionForTest = validateCreateDecision
)

// CreateDecisionForTest re-exports the create decision struct for tests.
type CreateDecisionForTest = CreateDecision
