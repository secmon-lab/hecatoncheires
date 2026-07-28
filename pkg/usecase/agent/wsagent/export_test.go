package wsagent

// BuildSystemPromptForTest re-exports buildSystemPrompt so external tests can
// exercise the safety-rule guardrail directly.
var BuildSystemPromptForTest = buildSystemPrompt

// BuildToolResolverForTest re-exports buildToolResolver so external tests can
// assert which tools a turn would hand to its sub-agents.
var BuildToolResolverForTest = (*UseCase).buildToolResolver

// ValidateRequestForTest re-exports validateRequest so external tests can
// exercise TurnRequest validation directly.
var ValidateRequestForTest = validateRequest
