package draft

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
)

// ParseAndValidateForTest exposes the unexported parseAndValidate for tests
// in the external draft_test package.
var ParseAndValidateForTest = parseAndValidate

// ExtractJSONObjectForTest exposes the JSON extraction helper so the
// test suite can pin its tolerance to LLM noise (prose prefixes,
// ```json fences) without going through a full parseAndValidate call.
var ExtractJSONObjectForTest = extractJSONObject

// PlanSchemaForTest exposes the unexported planSchema constructor for tests.
var PlanSchemaForTest = planSchema

// FormatObservationsForTest exposes the unexported formatObservationsAsUserTurn.
var FormatObservationsForTest = formatObservationsAsUserTurn

// RunInvestigationsParallelForTest invokes the unexported method for tests.
func RunInvestigationsParallelForTest(uc *UseCase, ctx context.Context, p *planInvestigate, h Handler, resolver *agent.ToolSetResolver) []investigationResult {
	return uc.runInvestigationsParallel(ctx, p, h, resolver)
}

// RunOneInvestigationForTest invokes the unexported method for tests.
func RunOneInvestigationForTest(uc *UseCase, ctx context.Context, task planInvestigateTask, h Handler, resolver *agent.ToolSetResolver) investigationResult {
	return uc.runOneInvestigation(ctx, task, h, resolver)
}

// Re-export internal constants used in test assertions.
const (
	ActionInvestigateForTest = actionInvestigate
	ActionQuestionForTest    = actionQuestion
	ActionMaterializeForTest = actionMaterialize
)

const (
	InvestigationCompletedForTest = investigationCompleted
	InvestigationFailedForTest    = investigationFailed
)

const (
	QuestionTypeSelectForTest      = questionTypeSelect
	QuestionTypeMultiSelectForTest = questionTypeMultiSelect
	QuestionTypeFreeTextForTest    = questionTypeFreeText
)

// Type aliases for tests that need to construct internal shapes.
type (
	PlanForTest                = plan
	PlanInvestigateForTest     = planInvestigate
	PlanInvestigateTaskForTest = planInvestigateTask
	PlanQuestionForTest        = planQuestion
	PlanQuestionItemForTest    = planQuestionItem
	PlanMaterializeForTest     = planMaterialize
	InvestigationResultForTest = investigationResult
)

// RenderPlannerPromptForTest exposes the prompt template render so tests
// can assert on the workspace context injected into the system prompt.
var RenderPlannerPromptForTest = renderPlannerPrompt
