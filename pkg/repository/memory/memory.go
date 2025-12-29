package memory

import (
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
)

// Repository is an alias for Memory to match the pattern
type Repository = Memory

type Memory struct {
	risk         *riskRepository
	response     *responseRepository
	riskResponse *riskResponseRepository
	tokens       *tokenStore
}

var _ interfaces.Repository = &Memory{}

func New() *Memory {
	riskRepo := newRiskRepository()
	responseRepo := newResponseRepository()
	riskResponseRepo := newRiskResponseRepository(responseRepo, riskRepo)

	return &Memory{
		risk:         riskRepo,
		response:     responseRepo,
		riskResponse: riskResponseRepo,
		tokens:       newTokenStore(),
	}
}

func (m *Memory) Risk() interfaces.RiskRepository {
	return m.risk
}

func (m *Memory) Response() interfaces.ResponseRepository {
	return m.response
}

func (m *Memory) RiskResponse() interfaces.RiskResponseRepository {
	return m.riskResponse
}
