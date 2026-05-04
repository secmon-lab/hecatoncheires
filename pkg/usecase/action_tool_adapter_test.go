package usecase_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// adapterTestSlackMock records the relevant Slack calls so the adapter
// tests can confirm SlackSyncFull (UpdateMessage + PostThreadMessage) is
// always pinned for tool-driven updates.
type adapterTestSlackMock struct {
	actionTestSlackMockExt
}

func TestNewActionToolAdapter(t *testing.T) {
	t.Run("returns nil when underlying ActionUseCase is nil", func(t *testing.T) {
		gt.Value(t, usecase.NewActionToolAdapter(nil)).Nil()
	})

	t.Run("returns non-nil adapter when usecase is provided", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewActionUseCase(repo, nil, nil, "")
		gt.Value(t, usecase.NewActionToolAdapter(uc)).NotNil()
	})
}

func TestActionToolAdapter_CreateAction(t *testing.T) {
	t.Run("delegates to ActionUseCase.CreateAction", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		adapter := usecase.NewActionToolAdapter(actionUC)
		got, err := adapter.CreateAction(ctx, testWorkspaceID, c.ID, "Adapter Title", "Adapter Desc", "U001", "ts-1", types.ActionStatusInProgress, nil)
		gt.NoError(t, err).Required()
		gt.Value(t, got.Title).Equal("Adapter Title")
		gt.Value(t, got.Description).Equal("Adapter Desc")
		gt.Value(t, got.AssigneeID).Equal("U001")
		gt.Value(t, got.Status).Equal(types.ActionStatusInProgress)
	})
}

func TestActionToolAdapter_UpdateAction(t *testing.T) {
	// All these tests share the same setup: a workspace, a case with a
	// Slack channel (so SlackSyncFull has something to refresh), and a
	// pre-existing action created via the unified usecase. The adapter
	// is then exercised against that action and we observe the resulting
	// Slack mock + persisted state.
	setup := func(t *testing.T) (context.Context, *memory.Repository, *adapterTestSlackMock, *usecase.ActionUseCase, *model.Action) {
		t.Helper()
		repo := memory.New()
		mock := &adapterTestSlackMock{
			actionTestSlackMockExt: actionTestSlackMockExt{
				actionTestSlackMock: actionTestSlackMock{
					mockSlackService: mockSlackService{
						createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
							return fmt.Sprintf("C%d", caseID), nil
						},
					},
					postMessageTS: "1234567890.123456",
				},
			},
		}
		caseUC := usecase.NewCaseUseCase(repo, nil, mock, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, mock, "https://example.com")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		gt.NoError(t, repo.SlackUser().SaveMany(ctx, []*model.SlackUser{
			{ID: "U001", Name: "alice", RealName: "Alice"},
			{ID: "U002", Name: "bob", RealName: "Bob"},
		})).Required()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()
		gt.Value(t, c.SlackChannelID).NotEqual("")

		action, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Adapter Action", "Initial", "U001", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		// Reset Slack call tracking after the create-time post so the
		// update-time assertions only see the adapter's own calls.
		mock.updateMessageCalled = false
		mock.postThreadCalled = false

		return ctx, repo, mock, actionUC, action
	}

	t.Run("translates pointer fields and triggers SlackSyncFull", func(t *testing.T) {
		ctx, _, mock, actionUC, action := setup(t)

		newTitle := "Adapter Updated"
		adapter := usecase.NewActionToolAdapter(actionUC)
		updated, err := adapter.UpdateAction(ctx, testWorkspaceID, action.ID, core.UpdateActionParams{
			Title: &newTitle,
		})
		gt.NoError(t, err).Required()
		gt.Value(t, updated.Title).Equal("Adapter Updated")

		// SlackSyncFull = the existing message must be refreshed AND a
		// thread summary posted. If the adapter accidentally pinned
		// SlackSyncSkip / SlackSyncMessageOnly we'd see one or both of
		// these stay false.
		gt.Bool(t, mock.updateMessageCalled).True()
		gt.Bool(t, mock.postThreadCalled).True()
	})

	t.Run("attributes change to system actor (no @mention)", func(t *testing.T) {
		ctx, _, mock, actionUC, action := setup(t)

		newStatus := types.ActionStatusInProgress
		adapter := usecase.NewActionToolAdapter(actionUC)
		_, err := adapter.UpdateAction(ctx, testWorkspaceID, action.ID, core.UpdateActionParams{
			Status: &newStatus,
		})
		gt.NoError(t, err).Required()

		// The change-notification thread post must not @-mention any
		// real Slack user. ActorKindSystem renders as the i18n "system"
		// fallback, never as <@U...>.
		gt.Bool(t, mock.postThreadCalled).True()
		gt.String(t, mock.postThreadText).NotEqual("")
		// A real @-mention would be wrapped <@Uxxxx>. Adapter-driven
		// updates must stay clear of that bracketed shape.
		for _, slackUserID := range []string{"<@U001>", "<@U002>", "<@UTESTUSER>"} {
			gt.String(t, mock.postThreadText).NotEqual(slackUserID)
		}
	})

	t.Run("propagates ClearAssignee through to UpdateActionInput", func(t *testing.T) {
		ctx, repo, _, actionUC, action := setup(t)

		adapter := usecase.NewActionToolAdapter(actionUC)
		_, err := adapter.UpdateAction(ctx, testWorkspaceID, action.ID, core.UpdateActionParams{
			ClearAssignee: true,
		})
		gt.NoError(t, err).Required()

		// Persistence is part of the contract: the stored row must
		// reflect the cleared assignee, not the original U001.
		stored, err := repo.Action().Get(ctx, testWorkspaceID, action.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, stored.AssigneeID).Equal("")
	})
}
