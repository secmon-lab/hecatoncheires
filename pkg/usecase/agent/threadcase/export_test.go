package threadcase

import "github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"

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

// BuildToolResolverForTest exposes the unexported sub-agent tool resolver
// builder so tests can assert that thread-mode turns withhold the core (action)
// toolset.
func (uc *UseCase) BuildToolResolverForTest(req TurnRequest) *agent.ToolSetResolver {
	return uc.buildToolResolver(req)
}
