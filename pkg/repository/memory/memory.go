package memory

import (
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
)

type Memory struct {
	risk *riskRepository
}

var _ interfaces.Repository = &Memory{}

func New() *Memory {
	return &Memory{
		risk: newRiskRepository(),
	}
}

func (m *Memory) Risk() interfaces.RiskRepository {
	return m.risk
}
