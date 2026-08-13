package threadcase

// Test-only seams for unit-testing the unexported prompt / decision helpers
// without exporting them into the production API.
var (
	BuildSystemPromptForTest      = buildSystemPrompt
	BuildUserInputForTest         = buildUserInput
	ValidateCreateDecisionForTest = validateCreateDecision
	ValidateRequestForTest        = validateRequest
)

// CreateDecisionForTest re-exports the create decision struct for tests.
type CreateDecisionForTest = CreateDecision
