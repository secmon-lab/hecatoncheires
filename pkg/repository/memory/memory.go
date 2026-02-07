package memory

import (
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
)

// Repository is an alias for Memory to match the pattern
type Repository = Memory

type Memory struct {
	caseRepo  *caseRepository
	action    *actionRepository
	tokens    *tokenStore
	slack     *slackRepository
	slackUser *slackUserRepository
	source    *sourceRepository
	knowledge *knowledgeRepository
}

var _ interfaces.Repository = &Memory{}

func New() *Memory {
	caseRepo := newCaseRepository()
	actionRepo := newActionRepository()
	slackRepo := newSlackRepository()
	slackUserRepo := newSlackUserRepository()
	sourceRepo := newSourceRepository()
	knowledgeRepo := newKnowledgeRepository()

	return &Memory{
		caseRepo:  caseRepo,
		action:    actionRepo,
		tokens:    newTokenStore(),
		slack:     slackRepo,
		slackUser: slackUserRepo,
		source:    sourceRepo,
		knowledge: knowledgeRepo,
	}
}

func (m *Memory) Case() interfaces.CaseRepository {
	return m.caseRepo
}

func (m *Memory) Action() interfaces.ActionRepository {
	return m.action
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
