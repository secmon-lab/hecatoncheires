package proposal

// Test-only seams for the unexported prompt helpers.
var (
	RenderDurablePromptForTest    = renderDurablePrompt
	WorkspacePromptEntriesForTest = workspacePromptEntries
	PlannerLanguageLabelForTest   = plannerLanguageLabel
	ValidateTurnRequestForTest    = validateTurnRequest
)
