package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/threadcase"
)

// A durable run's question must be PERSISTED, not just posted.
//
// The form's Submit handler treats a Session with no PendingQuestion as stale and
// drops the user's answer, so a question that is shown but not stored is worse
// than one never asked: the user answers into a void. The in-process path got the
// persistence for free — the runtime held the same Session instance and wrote it
// when the turn ended — and this is the seam where that stopped being true.
func TestThreadcaseHostPersistsThePendingQuestion(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws-1", Name: "WS"},
		CaseMode:  model.CaseModeThread,
	})

	slackMock := &agentTestSlackService{}
	agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
		Repo:         repo,
		Registry:     registry,
		SlackService: slackMock,
		HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:    agentarchive.NewMemoryTraceRepository(),
	})

	// A create turn's session: it predates the case, so CaseID is 0.
	ssn := &model.Session{
		ID:            "s-thread-q",
		ChannelID:     "C-MONITOR",
		ThreadTS:      "1700000000.000100",
		WorkspaceID:   "ws-1",
		CreatorUserID: "U-REPORTER",
		Kind:          model.SessionKindCase,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	gt.NoError(t, repo.Session().Put(ctx, ssn)).Required()

	target := threadcase.Target{
		SessionID:   ssn.ID,
		ChannelID:   ssn.ChannelID,
		ThreadTS:    ssn.ThreadTS,
		UIChannelID: ssn.ChannelID,
		UIThreadTS:  ssn.ThreadTS,
	}
	q := threadcase.QuestionPayload{
		Reason: "which environment?",
		Items: []threadcase.QuestionItem{
			{ID: "env", Text: "Which environment?", Type: threadcase.QuestionItemSelect,
				Options: []string{"staging", "production"}},
		},
	}
	gt.NoError(t, usecase.ThreadcaseAskQuestionForTest(agentUC, ctx, target, q)).Required()

	stored, err := repo.Session().GetByThread(ctx, ssn.ChannelID, ssn.ThreadTS)
	gt.NoError(t, err).Required()
	gt.Value(t, stored.PendingQuestion).NotNil().Required()
	gt.String(t, stored.PendingQuestion.Reason).Equal("which environment?")
	gt.Array(t, stored.PendingQuestion.Items).Length(1).Required()
	gt.String(t, stored.PendingQuestion.Items[0].ID).Equal("env")
	// The Submit handler matches the form it is answering by this ts, so a stored
	// question with no posted message would still be refused as stale.
	gt.String(t, stored.PendingQuestion.PostedMessageTS).NotEqual("")
}
