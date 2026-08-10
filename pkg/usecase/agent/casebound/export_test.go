package casebound

// BuildSystemPromptForTest exposes the unexported buildSystemPrompt for
// tests in the external package.
var BuildSystemPromptForTest = buildSystemPrompt

// BuildUserInputForTest exposes the unexported buildUserInput for tests.
var BuildUserInputForTest = buildUserInput

// ValidateRequestForTest exposes the unexported validateRequest so tests can
// pin the invariants StartTurn refuses to run without.
var ValidateRequestForTest = validateRequest
