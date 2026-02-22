package memory

import (
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
)

// Repository is an alias for Memory to match the pattern
type Repository = Memory

type Memory struct {
	caseRepo    *caseRepository
	action      *actionRepository
	tokens      *tokenStore
	slack       *slackRepository
	slackUser   *slackUserRepository
	source      *sourceRepository
	knowledge   *knowledgeRepository
	caseMessage *caseMessageRepository
	memoryStore *memoryRepository
	assistLog   *assistLogRepository
}

var _ interfaces.Repository = &Memory{}

func New() *Memory {
	return &Memory{
		caseRepo:    newCaseRepository(),
		action:      newActionRepository(),
		tokens:      newTokenStore(),
		slack:       newSlackRepository(),
		slackUser:   newSlackUserRepository(),
		source:      newSourceRepository(),
		knowledge:   newKnowledgeRepository(),
		caseMessage: newCaseMessageRepository(),
		memoryStore: newMemoryRepository(),
		assistLog:   newAssistLogRepository(),
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

func (m *Memory) CaseMessage() interfaces.CaseMessageRepository {
	return m.caseMessage
}

func (m *Memory) Memory() interfaces.MemoryRepository {
	return m.memoryStore
}

func (m *Memory) AssistLog() interfaces.AssistLogRepository {
	return m.assistLog
}

func (m *Memory) Close() error {
	// No resources to clean up for in-memory repository
	return nil
}
