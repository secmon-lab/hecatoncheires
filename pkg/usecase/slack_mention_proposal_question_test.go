package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	goslack "github.com/slack-go/slack"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/proposal"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

// Answering a draft question starts a new turn, and that turn is offered only
// the workspaces the answering user may access. When the answering user may
// use no workspace — or the decision cannot be made — the answer is not
// consumed: the form is left as it is and the question stays pending, so
// someone the policy allows can still answer it.
func TestHandleQuestionSubmit_NoAccessibleWorkspace(t *testing.T) {
	const formTS = "1700000015.000000"
	setup := func(t *testing.T) *dispatcherFixture {
		t.Helper()
		f := newDispatcherWithOpenSession(t, "C-OPEN", "1700000010.000000", model.SessionEndedWithQuestion)
		gt.NoError(t, f.repo.Session().SetPendingQuestion(context.Background(), f.channelID, f.threadTS,
			&model.PendingQuestion{
				PostedChannelID: f.channelID,
				PostedMessageTS: formTS,
				Reason:          "need severity",
				Items:           []model.PendingQuestionItem{{ID: "q-sev", Text: "What is the severity?", Type: "select", Options: []string{"low", "high"}}},
			})).Required()
		return f
	}
	assertQuestionStillOpen := func(t *testing.T, f *dispatcherFixture) {
		t.Helper()
		f.assertNoTurn(t, formTS)
		gt.Array(t, f.slackMock.updates()).Length(0)
		ssn, err := f.repo.Session().GetByThread(context.Background(), f.channelID, f.threadTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn.PendingQuestion).NotNil().Required()
		gt.Value(t, ssn.PendingQuestion.PostedMessageTS).Equal(formTS)
	}

	t.Run("every workspace denied", func(t *testing.T) {
		f := setup(t)
		registry := newRegistryWithSchema("ws-1", "ws", &config.FieldSchema{})
		usecase.SetMentionProposalWorkspaceAccessForTest(f.mentionProposal, denyingAccess(t, registry, "ws-1"))

		cb := newDraftQuestionSubmitCallback(f.channelID, f.threadTS, formTS)
		gt.NoError(t, f.mentionProposal.HandleQuestionSubmit(context.Background(), cb, cb.ActionCallback.BlockActions[0])).Required()
		async.Wait()

		assertQuestionStillOpen(t, f)
		gt.Array(t, f.slackMock.texts()).Length(1).Required()
		gt.String(t, f.slackMock.texts()[0]).Contains("No workspace is available to you")
	})

	t.Run("decision fails", func(t *testing.T) {
		f := setup(t)
		access := newAccessFixture(t, time.Minute, map[string]string{"ws-1": policyAllowAll}, "ws-1")
		access.users.failing.Store(true)
		usecase.SetMentionProposalWorkspaceAccessForTest(f.mentionProposal, access.uc)

		cb := newDraftQuestionSubmitCallback(f.channelID, f.threadTS, formTS)
		err := f.mentionProposal.HandleQuestionSubmit(context.Background(), cb, cb.ActionCallback.BlockActions[0])
		gt.Error(t, err).Is(errUserStoreDown)
		async.Wait()

		assertQuestionStillOpen(t, f)
		gt.Array(t, f.slackMock.texts()).Length(0)
	})
}

func newDraftQuestionSubmitCallback(channelID, threadTS, formTS string) *goslack.InteractionCallback {
	return &goslack.InteractionCallback{
		Type:    goslack.InteractionTypeBlockActions,
		User:    goslack.User{ID: "U1"},
		Channel: goslack.Channel{GroupConversation: goslack.GroupConversation{Conversation: goslack.Conversation{ID: channelID}}},
		Message: goslack.Message{Msg: goslack.Msg{Timestamp: formTS, ThreadTimestamp: threadTS}},
		BlockActionState: &goslack.BlockActionStates{
			Values: map[string]map[string]goslack.BlockAction{
				usecase.BlockIDDraftQuestionItemPrefix + "q-sev": {
					usecase.ActionIDDraftQuestionChoice: {SelectedOption: goslack.OptionBlockObject{Value: "high"}},
				},
			},
		},
		ActionCallback: goslack.ActionCallbacks{
			BlockActions: []*goslack.BlockAction{{ActionID: usecase.ActionIDDraftQuestionSubmit}},
		},
	}
}

// TestBuildProposalQuestionBlocks_Fallback locks the notification fallback
// of the mention-draft question form to the i18n layer: the English text is
// the historical hardcoded string, and a Japanese-locale context must yield
// the Japanese translation (pulled from the same i18n source the production
// code reads, not hardcoded here).
func TestBuildProposalQuestionBlocks_Fallback(t *testing.T) {
	q := proposal.QuestionPayload{
		Reason: "need severity",
		Items: []proposal.QuestionItem{
			{ID: "severity", Text: "How severe is it?", Type: proposal.QuestionItemSelect, Options: []string{"high", "low"}},
		},
	}

	t.Run("default context yields English fallback", func(t *testing.T) {
		blocks, fallback := usecase.BuildProposalQuestionBlocksForTest(context.Background(), q, "draft-1", "U123")
		gt.Number(t, len(blocks)).GreaterOrEqual(1)
		gt.Value(t, fallback).Equal("We need a bit more info to draft this case.")
	})

	t.Run("Japanese context yields localized fallback", func(t *testing.T) {
		jaCtx := i18n.ContextWithLang(context.Background(), i18n.LangJA)
		blocks, fallback := usecase.BuildProposalQuestionBlocksForTest(jaCtx, q, "draft-1", "U123")
		gt.Number(t, len(blocks)).GreaterOrEqual(1)
		gt.Value(t, fallback).Equal(i18n.T(jaCtx, i18n.MsgMentionQuestionFallback))
		gt.Value(t, fallback).NotEqual("We need a bit more info to draft this case.")
	})
}
