package draft

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

	plannerLoopMax     int
	subAgentMaxPerTurn int
	subAgentLoopMax    int
}

// New builds a draft.UseCase.
//
// plannerLoopMax / subAgentMaxPerTurn / subAgentLoopMax are the budget
// knobs (§5.1). They are caller-controlled — the spec recommends defaults
// of 8 / 16 / 20 wired via CLI flags. Pass 0 to use the package defaults.
func New(deps *agent.CommonDeps, plannerLoopMax, subAgentMaxPerTurn, subAgentLoopMax int) (*UseCase, error) {
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
	if subAgentMaxPerTurn <= 0 {
		subAgentMaxPerTurn = DefaultSubAgentMaxPerTurn
	}
	if subAgentLoopMax <= 0 {
		subAgentLoopMax = DefaultSubAgentLoopMax
	}
	return &UseCase{
		deps:               deps,
		plannerLoopMax:     plannerLoopMax,
		subAgentMaxPerTurn: subAgentMaxPerTurn,
		subAgentLoopMax:    subAgentLoopMax,
	}, nil
}

// PlannerLoopMax / SubAgentMaxPerTurn / SubAgentLoopMax expose the active
// budget so callers (e.g. tests) can assert on the wired configuration.
func (uc *UseCase) PlannerLoopMax() int     { return uc.plannerLoopMax }
func (uc *UseCase) SubAgentMaxPerTurn() int { return uc.subAgentMaxPerTurn }
func (uc *UseCase) SubAgentLoopMax() int    { return uc.subAgentLoopMax }

// Default budget values used when the caller passes 0.
const (
	DefaultPlannerLoopMax     = 8
	DefaultSubAgentMaxPerTurn = 16
	DefaultSubAgentLoopMax    = 20
)

// Sub-agent summary truncation — long summaries are cut to keep planner
// input tokens bounded.
const subAgentSummaryMaxBytes = 8 * 1024
