package wsagent

// BuildSystemPromptForTest re-exports buildSystemPrompt so external tests can
// exercise the safety-rule guardrail directly.
var BuildSystemPromptForTest = buildSystemPrompt

// ValidateRequestForTest re-exports validateRequest so external tests can
// exercise TurnRequest validation directly.
var ValidateRequestForTest = validateRequest
