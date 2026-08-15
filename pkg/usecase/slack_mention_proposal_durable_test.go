package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/proposal"
)

// A durable run's question must be PERSISTED, not just posted.
//
// The form's Submit handler treats a Session with no PendingQuestion as stale and
// drops the user's answer, so a question that is shown but not stored is worse
// than one never asked. The in-process path got the persistence for free — the
// runtime held the same Session instance and wrote it when the turn ended — and
// this is the seam where that stopped being true.
func TestProposalHostPersistsThePendingQuestion(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws-1", Name: "WS"},
	})

	slackMock := &agentTestSlackService{}
	draftUC := usecase.NewMentionProposalUseCase(repo, registry, slackMock)

	d := model.NewCaseProposal(time.Now().UTC(), "U-REQUESTER")
	gt.NoError(t, repo.CaseProposal().Save(ctx, d)).Required()

	ssn := &model.Session{
		ID:            "s-draft-q",
		ChannelID:     "C-DRAFT",
		ThreadTS:      "1700000000.000500",
		CreatorUserID: "U-REQUESTER",
		ProposalID:    d.ID,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	gt.NoError(t, repo.Session().Put(ctx, ssn)).Required()

	gt.NoError(t, usecase.ProposalAskForTest(draftUC, ctx, proposal.Target{
		SessionID:   ssn.ID,
		ChannelID:   ssn.ChannelID,
		ThreadTS:    ssn.ThreadTS,
		ActorUserID: "U-REQUESTER",
		// The draft comes off the run, not off the session.
		ProposalID: d.ID,
	}, proposal.QuestionPayload{
		Reason: "which workspace?",
		Items: []proposal.QuestionItem{
			{ID: "ws", Text: "Which workspace?", Type: proposal.QuestionItemSelect,
				Options: []string{"ws-1", "ws-2"}},
		},
	})).Required()

	stored, err := repo.Session().GetByThread(ctx, ssn.ChannelID, ssn.ThreadTS)
	gt.NoError(t, err).Required()
	gt.Value(t, stored.PendingQuestion).NotNil().Required()
	gt.String(t, stored.PendingQuestion.Reason).Equal("which workspace?")
	// The Submit handler matches the form it is answering by this ts, so a stored
	// question with no posted message would still be refused as stale.
	gt.String(t, stored.PendingQuestion.PostedMessageTS).NotEqual("")
}
