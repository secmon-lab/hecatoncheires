package memory

import (
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
)

// Repository is an alias for Memory to match the pattern
type Repository = Memory

type Memory struct {
	caseRepo      *caseRepository
	action        *actionRepository
	tokens        *tokenStore
	slack         *slackRepository
	slackUser     *slackUserRepository
	source        *sourceRepository
	caseMessage   *caseMessageRepository
	actionMessage *actionMessageRepository
	actionEvent   *actionEventRepository
	actionStep    *actionStepRepository
	assistLog     *assistLogRepository
	caseProposal  *caseProposalRepository
	session       *sessionRepository
	notifySlot    *notificationSlotRepository
}

var _ interfaces.Repository = &Memory{}

func New() *Memory {
	return &Memory{
		caseRepo:      newCaseRepository(),
		action:        newActionRepository(),
		tokens:        newTokenStore(),
		slack:         newSlackRepository(),
		slackUser:     newSlackUserRepository(),
		source:        newSourceRepository(),
		caseMessage:   newCaseMessageRepository(),
		actionMessage: newActionMessageRepository(),
		actionEvent:   newActionEventRepository(),
		actionStep:    newActionStepRepository(),
		assistLog:     newAssistLogRepository(),
		caseProposal:  newCaseProposalRepository(),
		session:       newSessionRepository(),
		notifySlot:    newNotificationSlotRepository(),
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

func (m *Memory) CaseMessage() interfaces.CaseMessageRepository {
	return m.caseMessage
}

func (m *Memory) ActionMessage() interfaces.ActionMessageRepository {
	return m.actionMessage
}

func (m *Memory) ActionEvent() interfaces.ActionEventRepository {
	return m.actionEvent
}

func (m *Memory) ActionStep() interfaces.ActionStepRepository {
	return m.actionStep
}

func (m *Memory) AssistLog() interfaces.AssistLogRepository {
	return m.assistLog
}

func (m *Memory) CaseProposal() interfaces.CaseProposalRepository {
	return m.caseProposal
}

func (m *Memory) Session() interfaces.SessionRepository {
	return m.session
}

func (m *Memory) NotificationSlot() interfaces.NotificationSlotRepository {
	return m.notifySlot
}

func (m *Memory) Close() error {
	// No resources to clean up for in-memory repository
	return nil
}
