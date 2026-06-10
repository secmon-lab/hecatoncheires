package proposal

import (
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
)

// UseCase orchestrates a single open-mode (case-draft) turn: planner LLM
// loop → parallel sub-agent investigations → terminal action via Handler.
// Construction is via New, which validates the configurable budget knobs
// and embeds the shared agent CommonDeps.
type UseCase struct {
	deps *agent.CommonDeps

	plannerLoopMax  int
	subAgentLoopMax int
}

// New builds a proposal.UseCase.
//
// plannerLoopMax / subAgentLoopMax are the budget knobs. They are
// caller-controlled — the recommended defaults are 8 / 20 wired via CLI
// flags. Pass 0 to use the package defaults. There is no per-turn total
// sub-agent count: the round-count limit (plannerLoopMax) plus the
// per-sub-agent budget (subAgentLoopMax) are the only knobs, with per-round
// fan-out bounded by plan validation.
func New(deps *agent.CommonDeps, plannerLoopMax, subAgentLoopMax int) (*UseCase, error) {
	if deps == nil {
		return nil, goerr.New("CommonDeps is required")
	}
	if deps.LLMClient == nil {
		return nil, goerr.New("LLMClient is required")
	}
	if deps.HistoryRepo == nil {
		return nil, goerr.New("HistoryRepo is required")
	}
	if deps.TraceRepo == nil {
		return nil, goerr.New("TraceRepo is required")
	}
	if plannerLoopMax <= 0 {
		plannerLoopMax = DefaultPlannerLoopMax
	}
	if subAgentLoopMax <= 0 {
		subAgentLoopMax = DefaultSubAgentLoopMax
	}
	return &UseCase{
		deps:            deps,
		plannerLoopMax:  plannerLoopMax,
		subAgentLoopMax: subAgentLoopMax,
	}, nil
}

// PlannerLoopMax / SubAgentLoopMax expose the active budget so callers
// (e.g. tests) can assert on the wired configuration.
func (uc *UseCase) PlannerLoopMax() int  { return uc.plannerLoopMax }
func (uc *UseCase) SubAgentLoopMax() int { return uc.subAgentLoopMax }

// Default budget values used when the caller passes 0.
const (
	DefaultPlannerLoopMax  = 8
	DefaultSubAgentLoopMax = 20
)

// Sub-agent summary truncation — long summaries are cut to keep planner
// input tokens bounded.
const subAgentSummaryMaxBytes = 8 * 1024
