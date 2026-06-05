package threadcase

// Test-only seams for unit-testing the unexported prompt / decision helpers
// without exporting them into the production API.
var (
	BuildSystemPromptForTest = buildSystemPrompt
	BuildUserInputForTest    = buildUserInput
	ParseDecisionForTest     = parseDecision
)
