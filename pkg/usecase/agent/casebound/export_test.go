package casebound

import (
	"github.com/gollem-dev/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
)

// BuildSystemPromptForTest exposes the unexported buildSystemPrompt for
// tests in the external package.
var BuildSystemPromptForTest = buildSystemPrompt

// BuildUserInputForTest exposes the unexported buildUserInput for tests.
var BuildUserInputForTest = buildUserInput

// BuildToolsForTest exposes the unexported buildTools so tests can assert the
// tool wiring (e.g. that casewriter tools appear only when CaseUC is set)
// without standing up the full New() dependency set.
func BuildToolsForTest(deps *agent.CommonDeps, req TurnRequest) []gollem.Tool {
	uc := &UseCase{deps: deps}
	return uc.buildTools(req)
}
