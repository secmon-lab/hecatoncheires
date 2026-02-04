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
	slack        *slackRepository
	slackUser    *slackUserRepository
	source       *sourceRepository
	knowledge    *knowledgeRepository
}

var _ interfaces.Repository = &Memory{}

func New() *Memory {
	riskRepo := newRiskRepository()
	responseRepo := newResponseRepository()
	riskResponseRepo := newRiskResponseRepository(responseRepo, riskRepo)
	slackRepo := newSlackRepository()
	slackUserRepo := newSlackUserRepository()
	sourceRepo := newSourceRepository()
	knowledgeRepo := newKnowledgeRepository()

	return &Memory{
		risk:         riskRepo,
		response:     responseRepo,
		riskResponse: riskResponseRepo,
		tokens:       newTokenStore(),
		slack:        slackRepo,
		slackUser:    slackUserRepo,
		source:       sourceRepo,
		knowledge:    knowledgeRepo,
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

func (m *Memory) Slack() interfaces.SlackRepository {
	return m.slack
}

func (m *Memory) SlackUser() interfaces.SlackUserRepository {
	return m.slackUser
}

func (m *Memory) Source() interfaces.SourceRepository {
	return m.source
}

func (m *Memory) Knowledge() interfaces.KnowledgeRepository {
	return m.knowledge
}
