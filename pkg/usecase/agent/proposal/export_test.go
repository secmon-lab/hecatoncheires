package proposal

// DraftFieldForTest lets an external test build a Draft's field list.
type DraftFieldForTest = draftField

// Test-only seams for the unexported prompt helpers.
var (
	RenderDurablePromptForTest    = renderDurablePrompt
	WorkspacePromptEntriesForTest = workspacePromptEntries
	PlannerLanguageLabelForTest   = plannerLanguageLabel
	ValidateTurnRequestForTest    = validateTurnRequest
)
