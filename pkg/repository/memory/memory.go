package memory

import (
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
)

// Repository is an alias for Memory to match the pattern
type Repository = Memory

type Memory struct {
	risk   *riskRepository
	tokens *tokenStore
}

var _ interfaces.Repository = &Memory{}

func New() *Memory {
	return &Memory{
		risk:   newRiskRepository(),
		tokens: newTokenStore(),
	}
}

func (m *Memory) Risk() interfaces.RiskRepository {
	return m.risk
}
