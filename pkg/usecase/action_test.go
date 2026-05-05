package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	goslack "github.com/slack-go/slack"
)

func TestActionUseCase_CreateAction(t *testing.T) {
	t.Run("create action with valid case", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		// Create case first
		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		// Create action
		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Action Description", "U001", "msg-123", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		gt.Number(t, created.ID).NotEqual(0)
		gt.Value(t, created.CaseID).Equal(c.ID)
		gt.Value(t, created.Title).Equal("Test Action")
		gt.Value(t, created.SlackMessageTS).Equal("msg-123")
		gt.Value(t, created.Status).Equal(types.ActionStatusTodo)
	})

	t.Run("create action without title fails", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "", "Description", "", "", types.ActionStatusTodo, nil)
		gt.Value(t, err).NotNil()
	})

	t.Run("create action with non-existent case fails", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := actionUC.CreateAction(ctx, testWorkspaceID, 999, "Test Action", "Description", "", "", types.ActionStatusTodo, nil)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})

	t.Run("create action with invalid status fails", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Description", "", "", "invalid-status", nil)
		gt.Value(t, err).NotNil()
	})

	t.Run("create action with default status", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Description", "", "", "", nil)
		gt.NoError(t, err).Required()

		// Default workspace falls back to model.DefaultActionStatusSet whose
		// initial id is BACKLOG.
		gt.Value(t, created.Status).Equal(types.ActionStatusBacklog)
	})
}

func TestActionUseCase_UpdateAction(t *testing.T) {
	t.Run("update action title and status", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Original Title", "Original Description", "U001", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		newTitle := "Updated Title"
		newStatus := types.ActionStatusInProgress
		updated, err := actionUC.UpdateAction(ctx, testWorkspaceID, usecase.UpdateActionInput{
			ID: created.ID, Title: &newTitle, Status: &newStatus, SlackSync: usecase.SlackSyncSkip,
		})
		gt.NoError(t, err).Required()

		gt.Value(t, updated.Title).Equal("Updated Title")
		gt.Value(t, updated.Status).Equal(types.ActionStatusInProgress)
		gt.Value(t, updated.CaseID).Equal(c.ID)
	})

	t.Run("update action caseID", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c1, err := caseUC.CreateCase(ctx, testWorkspaceID, "Case 1", "Description 1", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		c2, err := caseUC.CreateCase(ctx, testWorkspaceID, "Case 2", "Description 2", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c1.ID, "Test Action", "Description", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		updated, err := actionUC.UpdateAction(ctx, testWorkspaceID, usecase.UpdateActionInput{
			ID: created.ID, CaseID: &c2.ID, SlackSync: usecase.SlackSyncSkip,
		})
		gt.NoError(t, err).Required()

		gt.Value(t, updated.CaseID).Equal(c2.ID)
	})

	t.Run("update action with non-existent case fails", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Description", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		newCaseID := int64(999)
		_, err = actionUC.UpdateAction(ctx, testWorkspaceID, usecase.UpdateActionInput{
			ID: created.ID, CaseID: &newCaseID, SlackSync: usecase.SlackSyncSkip,
		})
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})

	t.Run("update non-existent action fails", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := actionUC.UpdateAction(ctx, testWorkspaceID, usecase.UpdateActionInput{
			ID: 999, SlackSync: usecase.SlackSyncSkip,
		})
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrActionNotFound)
	})
}

func TestActionUseCase_ArchiveAction(t *testing.T) {
	t.Run("archive action sets ArchivedAt and hides from default list", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Description", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		archived, err := actionUC.ArchiveAction(ctx, testWorkspaceID, created.ID, usecase.ActorRef{Kind: usecase.ActorKindSystem})
		gt.NoError(t, err).Required()
		gt.Value(t, archived.ArchivedAt).NotNil()
		gt.Bool(t, archived.IsArchived()).True()

		// Default list excludes archived
		got, err := actionUC.GetActionsByCase(ctx, testWorkspaceID, c.ID, interfaces.ActionListOptions{})
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(0)

		// IncludeArchived returns it
		gotAll, err := actionUC.GetActionsByCase(ctx, testWorkspaceID, c.ID, interfaces.ActionListOptions{IncludeArchived: true})
		gt.NoError(t, err).Required()
		gt.Array(t, gotAll).Length(1).Required()
		gt.Value(t, gotAll[0].ID).Equal(created.ID)

		// Get still returns archived action
		fetched, err := actionUC.GetAction(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()
		gt.Bool(t, fetched.IsArchived()).True()

		// ActionEvent recorded
		events, _, err := repo.ActionEvent().List(ctx, testWorkspaceID, created.ID, 100, "")
		gt.NoError(t, err).Required()
		archiveEventCount := 0
		for _, e := range events {
			if e.Kind == types.ActionEventArchived {
				archiveEventCount++
			}
		}
		gt.Number(t, archiveEventCount).Equal(1)
	})

	t.Run("archive already archived action returns ErrActionAlreadyArchived", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Description", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		_, err = actionUC.ArchiveAction(ctx, testWorkspaceID, created.ID, usecase.ActorRef{Kind: usecase.ActorKindSystem})
		gt.NoError(t, err).Required()

		_, err = actionUC.ArchiveAction(ctx, testWorkspaceID, created.ID, usecase.ActorRef{Kind: usecase.ActorKindSystem})
		gt.Error(t, err).Is(usecase.ErrActionAlreadyArchived)
	})

	t.Run("archive non-existent action returns ErrActionNotFound", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := actionUC.ArchiveAction(ctx, testWorkspaceID, 999, usecase.ActorRef{Kind: usecase.ActorKindSystem})
		gt.Error(t, err).Is(usecase.ErrActionNotFound)
	})
}

func TestActionUseCase_UnarchiveAction(t *testing.T) {
	t.Run("unarchive restores active state and records event", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Description", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		_, err = actionUC.ArchiveAction(ctx, testWorkspaceID, created.ID, usecase.ActorRef{Kind: usecase.ActorKindSystem})
		gt.NoError(t, err).Required()

		restored, err := actionUC.UnarchiveAction(ctx, testWorkspaceID, created.ID, usecase.ActorRef{Kind: usecase.ActorKindSystem})
		gt.NoError(t, err).Required()
		gt.Value(t, restored.ArchivedAt).Nil()
		gt.Bool(t, restored.IsArchived()).False()

		// Default list now includes it again
		got, err := actionUC.GetActionsByCase(ctx, testWorkspaceID, c.ID, interfaces.ActionListOptions{})
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(1).Required()
		gt.Value(t, got[0].ID).Equal(created.ID)

		events, _, err := repo.ActionEvent().List(ctx, testWorkspaceID, created.ID, 100, "")
		gt.NoError(t, err).Required()
		unarchiveCount := 0
		for _, e := range events {
			if e.Kind == types.ActionEventUnarchived {
				unarchiveCount++
			}
		}
		gt.Number(t, unarchiveCount).Equal(1)
	})

	t.Run("unarchive non-archived action returns ErrActionNotArchived", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Description", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		_, err = actionUC.UnarchiveAction(ctx, testWorkspaceID, created.ID, usecase.ActorRef{Kind: usecase.ActorKindSystem})
		gt.Error(t, err).Is(usecase.ErrActionNotArchived)
	})
}

func TestActionUseCase_GetAction(t *testing.T) {
	t.Run("get existing action", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Description", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		retrieved, err := actionUC.GetAction(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, retrieved.ID).Equal(created.ID)
		gt.Value(t, retrieved.Title).Equal(created.Title)
	})

	t.Run("get non-existent action fails", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := actionUC.GetAction(ctx, testWorkspaceID, 999)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrActionNotFound)
	})
}

func TestActionUseCase_ListActions(t *testing.T) {
	t.Run("list actions", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Action 1", "Description 1", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Action 2", "Description 2", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		actions, err := actionUC.ListActions(ctx, testWorkspaceID, interfaces.ActionListOptions{})
		gt.NoError(t, err).Required()

		gt.Array(t, actions).Length(2)
	})
}

func TestActionUseCase_GetActionsByCase(t *testing.T) {
	t.Run("get actions by case", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c1, err := caseUC.CreateCase(ctx, testWorkspaceID, "Case 1", "Description 1", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		c2, err := caseUC.CreateCase(ctx, testWorkspaceID, "Case 2", "Description 2", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c1.ID, "Action 1-1", "Description", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c1.ID, "Action 1-2", "Description", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c2.ID, "Action 2-1", "Description", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		actions, err := actionUC.GetActionsByCase(ctx, testWorkspaceID, c1.ID, interfaces.ActionListOptions{})
		gt.NoError(t, err).Required()

		gt.Array(t, actions).Length(2)

		for _, action := range actions {
			gt.Value(t, action.CaseID).Equal(c1.ID)
		}
	})
}

// actionTestSlackMock tracks PostMessage and UpdateMessage calls for action tests
type actionTestSlackMock struct {
	mockSlackService
	postMessageCalled   bool
	postMessageChannel  string
	postMessageBlocks   []goslack.Block
	postMessageText     string
	postMessageTS       string
	postMessageErr      error
	updateMessageCalled bool
	updateMessageTS     string
	updateMessageBlocks []goslack.Block
	updateMessageErr    error
}

func (m *actionTestSlackMock) PostMessage(ctx context.Context, channelID string, blocks []goslack.Block, text string) (string, error) {
	m.postMessageCalled = true
	m.postMessageChannel = channelID
	m.postMessageBlocks = blocks
	m.postMessageText = text
	if m.postMessageErr != nil {
		return "", m.postMessageErr
	}
	return m.postMessageTS, nil
}

func (m *actionTestSlackMock) UpdateMessage(ctx context.Context, channelID string, timestamp string, blocks []goslack.Block, text string) error {
	m.updateMessageCalled = true
	m.updateMessageTS = timestamp
	m.updateMessageBlocks = blocks
	return m.updateMessageErr
}

func TestActionUseCase_CreateAction_SlackNotification(t *testing.T) {
	t.Run("posts Slack message when case has channel", func(t *testing.T) {
		repo := memory.New()
		mock := &actionTestSlackMock{
			mockSlackService: mockSlackService{
				createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
					return fmt.Sprintf("C%d", caseID), nil
				},
			},
			postMessageTS: "1234567890.123456",
		}
		caseUC := usecase.NewCaseUseCase(repo, nil, mock, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, mock, "https://example.com")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		// Create case with Slack channel
		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()
		gt.Value(t, c.SlackChannelID).NotEqual("")

		// Create action
		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Action Desc", "U001", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		// Verify Slack message was posted
		gt.Value(t, mock.postMessageCalled).Equal(true)
		gt.Value(t, mock.postMessageChannel).Equal(c.SlackChannelID)

		// Verify SlackMessageTS was saved
		gt.Value(t, created.SlackMessageTS).Equal("1234567890.123456")

		// Verify the saved action has the timestamp
		retrieved, err := actionUC.GetAction(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, retrieved.SlackMessageTS).Equal("1234567890.123456")
	})

	t.Run("does not post when case has no channel", func(t *testing.T) {
		repo := memory.New()
		mock := &actionTestSlackMock{
			postMessageTS: "1234567890.123456",
		}
		// Create case without Slack service (no channel)
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, mock, "https://example.com")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()
		gt.Value(t, c.SlackChannelID).Equal("")

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Desc", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		// Slack message should NOT have been posted
		gt.Value(t, mock.postMessageCalled).Equal(false)
	})

	t.Run("does not post when slack service is nil", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "https://example.com")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Desc", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()
		gt.Value(t, created.SlackMessageTS).Equal("")
	})

	t.Run("action creation succeeds even when Slack posting fails", func(t *testing.T) {
		repo := memory.New()
		mock := &actionTestSlackMock{
			mockSlackService: mockSlackService{
				createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
					return fmt.Sprintf("C%d", caseID), nil
				},
			},
			postMessageErr: errors.New("slack API error"),
		}
		caseUC := usecase.NewCaseUseCase(repo, nil, mock, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, mock, "https://example.com")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		// Action creation should still succeed
		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Desc", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()
		gt.Value(t, created.Title).Equal("Test Action")
		gt.Value(t, created.SlackMessageTS).Equal("") // No TS because posting failed
	})
}

func TestActionUseCase_PostSlackMessageToAction(t *testing.T) {
	t.Run("posts message and persists timestamp on unposted action", func(t *testing.T) {
		repo := memory.New()
		mock := &actionTestSlackMock{
			mockSlackService: mockSlackService{
				createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
					return fmt.Sprintf("C%d", caseID), nil
				},
			},
			postMessageTS: "9999.123456",
		}
		caseUC := usecase.NewCaseUseCase(repo, nil, mock, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, mock, "https://example.com")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()
		gt.Value(t, c.SlackChannelID).NotEqual("")

		// Simulate an action created via the legacy tool path: directly via
		// the repository, bypassing CreateAction. SlackMessageTS stays empty.
		raw, err := repo.Action().Create(ctx, testWorkspaceID, &model.Action{
			CaseID: c.ID,
			Title:  "Unsent Action",
			Status: types.ActionStatusTodo,
		})
		gt.NoError(t, err).Required()
		gt.Value(t, raw.SlackMessageTS).Equal("")

		// Reset mock so we observe only the PostSlackMessageToAction call.
		mock.postMessageCalled = false

		updated, err := actionUC.PostSlackMessageToAction(ctx, testWorkspaceID, raw.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, mock.postMessageCalled).Equal(true)
		gt.Value(t, mock.postMessageChannel).Equal(c.SlackChannelID)
		gt.Value(t, updated.SlackMessageTS).Equal("9999.123456")

		retrieved, err := actionUC.GetAction(ctx, testWorkspaceID, raw.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, retrieved.SlackMessageTS).Equal("9999.123456")
	})

	t.Run("returns ErrSlackMessageAlreadyPosted for action with existing TS", func(t *testing.T) {
		repo := memory.New()
		mock := &actionTestSlackMock{
			mockSlackService: mockSlackService{
				createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
					return fmt.Sprintf("C%d", caseID), nil
				},
			},
			postMessageTS: "1.1",
		}
		caseUC := usecase.NewCaseUseCase(repo, nil, mock, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, mock, "https://example.com")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Posted Action", "Desc", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()
		gt.Value(t, created.SlackMessageTS).NotEqual("")

		mock.postMessageCalled = false
		_, err = actionUC.PostSlackMessageToAction(ctx, testWorkspaceID, created.ID)
		gt.Error(t, err).Is(usecase.ErrSlackMessageAlreadyPosted)
		gt.Value(t, mock.postMessageCalled).Equal(false)
	})

	t.Run("returns ErrCaseHasNoSlackChannel when parent has no channel", func(t *testing.T) {
		repo := memory.New()
		// Build a Slack mock that records calls but returns a TS — to confirm
		// that PostMessage was NEVER invoked when channel is missing.
		mock := &actionTestSlackMock{postMessageTS: "should-not-be-used"}
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, mock, "https://example.com")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()
		gt.Value(t, c.SlackChannelID).Equal("")

		raw, err := repo.Action().Create(ctx, testWorkspaceID, &model.Action{
			CaseID: c.ID,
			Title:  "Unsent Action",
			Status: types.ActionStatusTodo,
		})
		gt.NoError(t, err).Required()

		_, err = actionUC.PostSlackMessageToAction(ctx, testWorkspaceID, raw.ID)
		gt.Error(t, err).Is(usecase.ErrCaseHasNoSlackChannel)
		gt.Value(t, mock.postMessageCalled).Equal(false)
	})

	t.Run("returns ErrActionNotFound for unknown action", func(t *testing.T) {
		repo := memory.New()
		mock := &actionTestSlackMock{}
		actionUC := usecase.NewActionUseCase(repo, nil, mock, "https://example.com")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := actionUC.PostSlackMessageToAction(ctx, testWorkspaceID, 99999)
		gt.Error(t, err).Is(usecase.ErrActionNotFound)
	})

	t.Run("denies access to non-member of private case", func(t *testing.T) {
		repo := memory.New()
		mock := &actionTestSlackMock{
			mockSlackService: mockSlackService{
				createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
					return fmt.Sprintf("C%d", caseID), nil
				},
			},
			postMessageTS: "should-not-post",
		}
		caseUC := usecase.NewCaseUseCase(repo, nil, mock, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, mock, "https://example.com")
		memberCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMEMBER"})

		c, err := caseUC.CreateCase(memberCtx, testWorkspaceID, "Private Case", "Desc",
			[]string{"UMEMBER"}, nil, true, "", "")
		gt.NoError(t, err).Required()

		raw, err := repo.Action().Create(memberCtx, testWorkspaceID, &model.Action{
			CaseID: c.ID,
			Title:  "Private Action",
			Status: types.ActionStatusTodo,
		})
		gt.NoError(t, err).Required()

		outsiderCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UOUTSIDER"})
		_, err = actionUC.PostSlackMessageToAction(outsiderCtx, testWorkspaceID, raw.ID)
		gt.Error(t, err).Is(usecase.ErrAccessDenied)
		gt.Value(t, mock.postMessageCalled).Equal(false)
	})

	t.Run("returns wrapped error when Slack PostMessage fails", func(t *testing.T) {
		repo := memory.New()
		mock := &actionTestSlackMock{
			mockSlackService: mockSlackService{
				createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
					return fmt.Sprintf("C%d", caseID), nil
				},
			},
			postMessageErr: errors.New("slack outage"),
		}
		caseUC := usecase.NewCaseUseCase(repo, nil, mock, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, mock, "https://example.com")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		raw, err := repo.Action().Create(ctx, testWorkspaceID, &model.Action{
			CaseID: c.ID,
			Title:  "Unsent",
			Status: types.ActionStatusTodo,
		})
		gt.NoError(t, err).Required()

		_, err = actionUC.PostSlackMessageToAction(ctx, testWorkspaceID, raw.ID)
		gt.Value(t, err).NotNil()
		// Slack failure must NOT be silently swallowed for the strict path.
		gt.String(t, err.Error()).Contains("slack outage")

		retrieved, err := actionUC.GetAction(ctx, testWorkspaceID, raw.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, retrieved.SlackMessageTS).Equal("")
	})
}

// actionTestSlackMockExt extends actionTestSlackMock to also capture
// PostThreadMessage calls used by change-notification tests.
type actionTestSlackMockExt struct {
	actionTestSlackMock
	postThreadCalled  bool
	postThreadChannel string
	postThreadTS      string
	postThreadBlocks  []goslack.Block
	postThreadText    string
}

func (m *actionTestSlackMockExt) PostThreadMessage(ctx context.Context, channelID string, threadTS string, blocks []goslack.Block, text string) (string, error) {
	m.postThreadCalled = true
	m.postThreadChannel = channelID
	m.postThreadTS = threadTS
	m.postThreadBlocks = blocks
	m.postThreadText = text
	return "thread-reply-ts", nil
}

func TestActionUseCase_UpdateAction_SlackSync(t *testing.T) {
	i18n.Init(i18n.LangEN)
	setup := func(t *testing.T) (*memory.Repository, *usecase.ActionUseCase, *actionTestSlackMockExt, *model.Action) {
		t.Helper()
		repo := memory.New()
		mock := &actionTestSlackMockExt{
			actionTestSlackMock: actionTestSlackMock{
				mockSlackService: mockSlackService{
					createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
						return fmt.Sprintf("C%d", caseID), nil
					},
				},
				postMessageTS: "1234567890.123456",
			},
		}
		caseUC := usecase.NewCaseUseCase(repo, nil, mock, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, mock, "https://example.com")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		// Register the human users that the bot-rejection guard will look up.
		gt.NoError(t, repo.SlackUser().SaveMany(ctx, []*model.SlackUser{
			{ID: "U001", Name: "alice", RealName: "Alice"},
			{ID: "U999", Name: "carol", RealName: "Carol"},
		})).Required()

		_, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()
		c, err := repo.Case().List(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()

		action, err := actionUC.CreateAction(ctx, testWorkspaceID, c[0].ID, "Test Action", "Desc", "U001", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		// Reset call tracking after creation
		mock.updateMessageCalled = false
		mock.postThreadCalled = false
		return repo, actionUC, mock, action
	}

	// listChangeEvents pulls the ActionEvent stream for the action and drops the
	// CREATED record so each test only sees diffs produced by UpdateAction.
	listChangeEvents := func(t *testing.T, repo *memory.Repository, ctx context.Context, actionID int64) []*model.ActionEvent {
		t.Helper()
		events, _, err := repo.ActionEvent().List(ctx, testWorkspaceID, actionID, 100, "")
		gt.NoError(t, err).Required()
		var diffs []*model.ActionEvent
		for _, e := range events {
			if e.Kind == types.ActionEventCreated {
				continue
			}
			diffs = append(diffs, e)
		}
		return diffs
	}

	t.Run("status change with SlackSyncFull updates message and records status event", func(t *testing.T) {
		repo, actionUC, mock, action := setup(t)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		newStatus := types.ActionStatusInProgress
		_, err := actionUC.UpdateAction(ctx, testWorkspaceID, usecase.UpdateActionInput{
			ID:        action.ID,
			Status:    &newStatus,
			SlackSync: usecase.SlackSyncFull,
			Actor:     usecase.ActorRef{Kind: usecase.ActorKindSlackUser, ID: "UDOER"},
		})
		gt.NoError(t, err).Required()

		updated, err := repo.Action().Get(ctx, testWorkspaceID, action.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, updated.Status).Equal(types.ActionStatusInProgress)
		gt.Bool(t, mock.updateMessageCalled).True()
		// SlackSyncFull both refreshes the message AND posts a thread
		// summary so channel watchers can see the change without opening
		// the WebUI. The ingest path drops these context-block posts so
		// they don't double-count in the ActionEvent feed.
		gt.Bool(t, mock.postThreadCalled).True()
		gt.String(t, mock.postThreadText).Contains("<@UDOER>")

		diffs := listChangeEvents(t, repo, ctx, action.ID)
		gt.Array(t, diffs).Length(1).Required()
		gt.Value(t, diffs[0].Kind).Equal(types.ActionEventStatusChanged)
		gt.Value(t, diffs[0].ActorID).Equal("UDOER")
		gt.Value(t, diffs[0].OldValue).Equal(types.ActionStatusTodo.String())
		gt.Value(t, diffs[0].NewValue).Equal(types.ActionStatusInProgress.String())
	})

	t.Run("assignee replacement records assignee event", func(t *testing.T) {
		repo, actionUC, _, action := setup(t)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		newAssignee := "U999"
		_, err := actionUC.UpdateAction(ctx, testWorkspaceID, usecase.UpdateActionInput{
			ID:         action.ID,
			AssigneeID: &newAssignee,
			SlackSync:  usecase.SlackSyncFull,
			Actor:      usecase.ActorRef{Kind: usecase.ActorKindSlackUser, ID: "UDOER"},
		})
		gt.NoError(t, err).Required()

		diffs := listChangeEvents(t, repo, ctx, action.ID)
		gt.Array(t, diffs).Length(1).Required()
		gt.Value(t, diffs[0].Kind).Equal(types.ActionEventAssigneeChanged)
		gt.Value(t, diffs[0].OldValue).Equal("U001")
		gt.Value(t, diffs[0].NewValue).Equal("U999")
	})

	t.Run("assignee clear records empty newValue", func(t *testing.T) {
		repo, actionUC, _, action := setup(t)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := actionUC.UpdateAction(ctx, testWorkspaceID, usecase.UpdateActionInput{
			ID:            action.ID,
			ClearAssignee: true,
			SlackSync:     usecase.SlackSyncFull,
			Actor:         usecase.ActorRef{Kind: usecase.ActorKindSlackUser, ID: "UDOER"},
		})
		gt.NoError(t, err).Required()

		diffs := listChangeEvents(t, repo, ctx, action.ID)
		gt.Array(t, diffs).Length(1).Required()
		gt.Value(t, diffs[0].Kind).Equal(types.ActionEventAssigneeChanged)
		gt.Value(t, diffs[0].OldValue).Equal("U001")
		gt.Value(t, diffs[0].NewValue).Equal("")
	})

	t.Run("SlackSyncMessageOnly refreshes message and still records event", func(t *testing.T) {
		repo, actionUC, mock, action := setup(t)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		newStatus := types.ActionStatusInProgress
		_, err := actionUC.UpdateAction(ctx, testWorkspaceID, usecase.UpdateActionInput{
			ID:        action.ID,
			Status:    &newStatus,
			SlackSync: usecase.SlackSyncMessageOnly,
		})
		gt.NoError(t, err).Required()
		gt.Bool(t, mock.updateMessageCalled).True()
		gt.Bool(t, mock.postThreadCalled).False()

		diffs := listChangeEvents(t, repo, ctx, action.ID)
		gt.Array(t, diffs).Length(1).Required()
	})

	t.Run("SlackSyncSkip skips Slack but still records event", func(t *testing.T) {
		repo, actionUC, mock, action := setup(t)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		newStatus := types.ActionStatusInProgress
		_, err := actionUC.UpdateAction(ctx, testWorkspaceID, usecase.UpdateActionInput{
			ID:        action.ID,
			Status:    &newStatus,
			SlackSync: usecase.SlackSyncSkip,
		})
		gt.NoError(t, err).Required()
		gt.Bool(t, mock.updateMessageCalled).False()
		gt.Bool(t, mock.postThreadCalled).False()

		diffs := listChangeEvents(t, repo, ctx, action.ID)
		gt.Array(t, diffs).Length(1).Required()
	})

	t.Run("no observable change records no event", func(t *testing.T) {
		repo, actionUC, mock, action := setup(t)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		// Set status to its current value: no diff in title/status/assignee.
		current := action.Status
		_, err := actionUC.UpdateAction(ctx, testWorkspaceID, usecase.UpdateActionInput{
			ID:        action.ID,
			Status:    &current,
			SlackSync: usecase.SlackSyncFull,
			Actor:     usecase.ActorRef{Kind: usecase.ActorKindSlackUser, ID: "UDOER"},
		})
		gt.NoError(t, err).Required()
		gt.Bool(t, mock.postThreadCalled).False()

		diffs := listChangeEvents(t, repo, ctx, action.ID)
		gt.Array(t, diffs).Length(0)
	})

	t.Run("system actor records event with empty actorID", func(t *testing.T) {
		repo, actionUC, _, action := setup(t)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		newTitle := "Renamed"
		_, err := actionUC.UpdateAction(ctx, testWorkspaceID, usecase.UpdateActionInput{
			ID:        action.ID,
			Title:     &newTitle,
			SlackSync: usecase.SlackSyncFull,
			Actor:     usecase.ActorRef{Kind: usecase.ActorKindSystem},
		})
		gt.NoError(t, err).Required()

		diffs := listChangeEvents(t, repo, ctx, action.ID)
		gt.Array(t, diffs).Length(1).Required()
		gt.Value(t, diffs[0].Kind).Equal(types.ActionEventTitleChanged)
		// System-driven changes leave ActorID empty; the WebUI renders a
		// neutral "system" label in that case.
		gt.Value(t, diffs[0].ActorID).Equal("")
		gt.Value(t, diffs[0].NewValue).Equal("Renamed")
	})

	t.Run("multiple field changes share a single CreatedAt", func(t *testing.T) {
		repo, actionUC, _, action := setup(t)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		newTitle := "Renamed"
		newStatus := types.ActionStatusInProgress
		newAssignee := "UASSIGNEE"
		_, err := actionUC.UpdateAction(ctx, testWorkspaceID, usecase.UpdateActionInput{
			ID:         action.ID,
			Title:      &newTitle,
			Status:     &newStatus,
			AssigneeID: &newAssignee,
			SlackSync:  usecase.SlackSyncSkip,
			Actor:      usecase.ActorRef{Kind: usecase.ActorKindSlackUser, ID: "UDOER"},
		})
		gt.NoError(t, err).Required()

		diffs := listChangeEvents(t, repo, ctx, action.ID)
		gt.Array(t, diffs).Length(3).Required()
		// All sibling diffs from one UpdateAction call must share the same
		// CreatedAt so the activity feed groups them deterministically; any
		// drift would let the sort flip per call.
		ts := diffs[0].CreatedAt
		for _, d := range diffs[1:] {
			gt.Bool(t, d.CreatedAt.Equal(ts)).True()
		}
	})
}

func TestParseSlackStatusSelectValue(t *testing.T) {
	t.Run("valid value", func(t *testing.T) {
		ws, id, status, err := usecase.ParseSlackStatusSelectValue("test-ws:42:IN_PROGRESS")
		gt.NoError(t, err).Required()
		gt.Value(t, ws).Equal("test-ws")
		gt.Value(t, id).Equal(int64(42))
		gt.Value(t, status).Equal(types.ActionStatusInProgress)
	})

	t.Run("workspace with hyphen", func(t *testing.T) {
		ws, id, status, err := usecase.ParseSlackStatusSelectValue("my-workspace:123:COMPLETED")
		gt.NoError(t, err).Required()
		gt.Value(t, ws).Equal("my-workspace")
		gt.Value(t, id).Equal(int64(123))
		gt.Value(t, status).Equal(types.ActionStatusCompleted)
	})

	t.Run("unknown-but-non-empty status passes through", func(t *testing.T) {
		// Status validity is decided by the workspace's ActionStatusSet at the
		// callsite, not by this parser. Any non-empty value is parsed.
		ws, id, status, err := usecase.ParseSlackStatusSelectValue("ws:1:CUSTOM_STATUS")
		gt.NoError(t, err).Required()
		gt.Value(t, ws).Equal("ws")
		gt.Value(t, id).Equal(int64(1))
		gt.Value(t, status).Equal(types.ActionStatus("CUSTOM_STATUS"))
	})

	t.Run("empty status is rejected", func(t *testing.T) {
		_, _, _, err := usecase.ParseSlackStatusSelectValue("ws:1:")
		gt.Value(t, err).NotNil()
	})

	t.Run("invalid action id", func(t *testing.T) {
		_, _, _, err := usecase.ParseSlackStatusSelectValue("ws:notnum:TODO")
		gt.Value(t, err).NotNil()
	})

	t.Run("missing fields", func(t *testing.T) {
		_, _, _, err := usecase.ParseSlackStatusSelectValue("only-one-field")
		gt.Value(t, err).NotNil()
	})
}

func TestParseSlackAssigneeBlockID(t *testing.T) {
	t.Run("round-trip", func(t *testing.T) {
		blockID := usecase.SlackActionAssigneeBlockID("test-ws", 42)
		ws, id, err := usecase.ParseSlackAssigneeBlockID(blockID)
		gt.NoError(t, err).Required()
		gt.Value(t, ws).Equal("test-ws")
		gt.Value(t, id).Equal(int64(42))
	})

	t.Run("invalid prefix", func(t *testing.T) {
		_, _, err := usecase.ParseSlackAssigneeBlockID("not_our_block_id")
		gt.Value(t, err).NotNil()
	})

	t.Run("missing action id", func(t *testing.T) {
		_, _, err := usecase.ParseSlackAssigneeBlockID("hc_action_assignee_block:onlyws")
		gt.Value(t, err).NotNil()
	})
}

func TestActionUseCase_DueDate(t *testing.T) {
	t.Run("create action with due date", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		dueDate := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Action with due date", "Description", "", "", types.ActionStatusTodo, &dueDate)
		gt.NoError(t, err).Required()

		gt.Value(t, created.DueDate).NotNil()
		gt.Value(t, created.DueDate.Year()).Equal(2026)
		gt.Value(t, created.DueDate.Month()).Equal(time.March)
		gt.Value(t, created.DueDate.Day()).Equal(15)

		// Verify persistence
		retrieved, err := actionUC.GetAction(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, retrieved.DueDate).NotNil()
		gt.Value(t, retrieved.DueDate.Equal(dueDate)).Equal(true)
	})

	t.Run("create action without due date", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Action without due date", "Description", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		gt.Value(t, created.DueDate == nil).Equal(true)
	})

	t.Run("update action to set due date", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Action", "Description", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()
		gt.Value(t, created.DueDate == nil).Equal(true)

		dueDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
		updated, err := actionUC.UpdateAction(ctx, testWorkspaceID, usecase.UpdateActionInput{
			ID: created.ID, DueDate: &dueDate, SlackSync: usecase.SlackSyncSkip,
		})
		gt.NoError(t, err).Required()

		gt.Value(t, updated.DueDate).NotNil()
		gt.Value(t, updated.DueDate.Equal(dueDate)).Equal(true)
	})

	t.Run("update action to clear due date", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		dueDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Action", "Description", "", "", types.ActionStatusTodo, &dueDate)
		gt.NoError(t, err).Required()
		gt.Value(t, created.DueDate).NotNil()

		// Clear due date using clearDueDate=true
		updated, err := actionUC.UpdateAction(ctx, testWorkspaceID, usecase.UpdateActionInput{
			ID: created.ID, ClearDueDate: true, SlackSync: usecase.SlackSyncSkip,
		})
		gt.NoError(t, err).Required()

		gt.Value(t, updated.DueDate == nil).Equal(true)
	})

	t.Run("update action without changing due date preserves it", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		dueDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Action", "Description", "", "", types.ActionStatusTodo, &dueDate)
		gt.NoError(t, err).Required()

		// Update only title, leave dueDate unchanged (nil, false)
		newTitle := "Updated Title"
		updated, err := actionUC.UpdateAction(ctx, testWorkspaceID, usecase.UpdateActionInput{
			ID: created.ID, Title: &newTitle, SlackSync: usecase.SlackSyncSkip,
		})
		gt.NoError(t, err).Required()

		gt.Value(t, updated.Title).Equal("Updated Title")
		gt.Value(t, updated.DueDate).NotNil()
		gt.Value(t, updated.DueDate.Equal(dueDate)).Equal(true)
	})
}

func TestActionUseCase_PrivateCaseAccessControl(t *testing.T) {
	t.Run("create action in private case as non-member returns access denied", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		memberCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMEMBER"})
		nonMemberCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UOTHER"})

		// Create private case directly via repo with specific members
		privateCase := &model.Case{
			Title:          "Private Case",
			Description:    "Secret",
			IsPrivate:      true,
			ChannelUserIDs: []string{"UMEMBER"},
			AssigneeIDs:    []string{},
		}
		created, err := repo.Case().Create(memberCtx, testWorkspaceID, privateCase)
		gt.NoError(t, err).Required()

		// Member can create action
		action, err := actionUC.CreateAction(memberCtx, testWorkspaceID, created.ID, "Member Action", "Desc", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()
		gt.Value(t, action.Title).Equal("Member Action")

		// Non-member cannot create action
		_, err = actionUC.CreateAction(nonMemberCtx, testWorkspaceID, created.ID, "Non-member Action", "Desc", "", "", types.ActionStatusTodo, nil)
		gt.Error(t, err).Is(usecase.ErrAccessDenied)
	})

	t.Run("update action in private case as non-member returns access denied", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		memberCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMEMBER"})
		nonMemberCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UOTHER"})

		privateCase := &model.Case{
			Title:          "Private Case",
			Description:    "Secret",
			IsPrivate:      true,
			ChannelUserIDs: []string{"UMEMBER"},
			AssigneeIDs:    []string{},
		}
		created, err := repo.Case().Create(memberCtx, testWorkspaceID, privateCase)
		gt.NoError(t, err).Required()

		// Member creates action
		action, err := actionUC.CreateAction(memberCtx, testWorkspaceID, created.ID, "Action", "Desc", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		// Non-member cannot update action
		newTitle := "Updated"
		_, err = actionUC.UpdateAction(nonMemberCtx, testWorkspaceID, usecase.UpdateActionInput{
			ID: action.ID, Title: &newTitle, SlackSync: usecase.SlackSyncSkip,
		})
		gt.Error(t, err).Is(usecase.ErrAccessDenied)
	})

	// Slack interactivity routes UpdateAction through async.Dispatch, which
	// hands the usecase a fresh background context with no auth token. The
	// access check must therefore fall back to the supplied Actor.ID when
	// the Slack actor is not a channel member.
	t.Run("update action in private case as Slack non-member is denied without token", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		memberCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMEMBER"})
		// Simulate the post-async.Dispatch context: no auth token attached.
		bgCtx := context.Background()

		privateCase := &model.Case{
			Title:          "Private Case",
			Description:    "Secret",
			IsPrivate:      true,
			ChannelUserIDs: []string{"UMEMBER"},
			AssigneeIDs:    []string{},
		}
		created, err := repo.Case().Create(memberCtx, testWorkspaceID, privateCase)
		gt.NoError(t, err).Required()

		action, err := actionUC.CreateAction(memberCtx, testWorkspaceID, created.ID, "Action", "Desc", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		newStatus := types.ActionStatusInProgress

		// Slack non-member: should be rejected via Actor.ID fallback.
		_, err = actionUC.UpdateAction(bgCtx, testWorkspaceID, usecase.UpdateActionInput{
			ID:        action.ID,
			Status:    &newStatus,
			SlackSync: usecase.SlackSyncSkip,
			Actor:     usecase.ActorRef{Kind: usecase.ActorKindSlackUser, ID: "UOTHER"},
		})
		gt.Error(t, err).Is(usecase.ErrAccessDenied)

		// Slack member: should succeed even though the context has no token.
		updated, err := actionUC.UpdateAction(bgCtx, testWorkspaceID, usecase.UpdateActionInput{
			ID:        action.ID,
			Status:    &newStatus,
			SlackSync: usecase.SlackSyncSkip,
			Actor:     usecase.ActorRef{Kind: usecase.ActorKindSlackUser, ID: "UMEMBER"},
		})
		gt.NoError(t, err).Required()
		gt.Value(t, updated.Status).Equal(types.ActionStatusInProgress)

		// System actor with no token: backward-compat path stays open.
		newStatus2 := types.ActionStatusCompleted
		updated, err = actionUC.UpdateAction(bgCtx, testWorkspaceID, usecase.UpdateActionInput{
			ID:        action.ID,
			Status:    &newStatus2,
			SlackSync: usecase.SlackSyncSkip,
			Actor:     usecase.ActorRef{Kind: usecase.ActorKindSystem},
		})
		gt.NoError(t, err).Required()
		gt.Value(t, updated.Status).Equal(types.ActionStatusCompleted)

		// Slack actor with empty ID (malformed callback): must be denied
		// rather than silently bypassing access control.
		newStatus3 := types.ActionStatusBlocked
		_, err = actionUC.UpdateAction(bgCtx, testWorkspaceID, usecase.UpdateActionInput{
			ID:        action.ID,
			Status:    &newStatus3,
			SlackSync: usecase.SlackSyncSkip,
			Actor:     usecase.ActorRef{Kind: usecase.ActorKindSlackUser, ID: ""},
		})
		gt.Error(t, err).Is(usecase.ErrAccessDenied)
	})

	t.Run("delete action in private case as non-member returns access denied", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		memberCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMEMBER"})
		nonMemberCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UOTHER"})

		privateCase := &model.Case{
			Title:          "Private Case",
			Description:    "Secret",
			IsPrivate:      true,
			ChannelUserIDs: []string{"UMEMBER"},
			AssigneeIDs:    []string{},
		}
		created, err := repo.Case().Create(memberCtx, testWorkspaceID, privateCase)
		gt.NoError(t, err).Required()

		// Member creates action
		action, err := actionUC.CreateAction(memberCtx, testWorkspaceID, created.ID, "Action", "Desc", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		// Non-member cannot archive action
		_, err = actionUC.ArchiveAction(nonMemberCtx, testWorkspaceID, action.ID, usecase.ActorRef{Kind: usecase.ActorKindSystem})
		gt.Error(t, err).Is(usecase.ErrAccessDenied)
	})

	t.Run("list actions filters out private case actions for non-members", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		memberCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMEMBER"})
		nonMemberCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UOTHER"})

		// Create public case with action
		pubCase, err := caseUC.CreateCase(memberCtx, testWorkspaceID, "Public Case", "Desc", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()
		_, err = actionUC.CreateAction(memberCtx, testWorkspaceID, pubCase.ID, "Public Action", "Desc", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		// Create private case with action (directly via repo)
		privateCase := &model.Case{
			Title:          "Private Case",
			Description:    "Secret",
			IsPrivate:      true,
			ChannelUserIDs: []string{"UMEMBER"},
			AssigneeIDs:    []string{},
		}
		privCreated, err := repo.Case().Create(memberCtx, testWorkspaceID, privateCase)
		gt.NoError(t, err).Required()
		_, err = actionUC.CreateAction(memberCtx, testWorkspaceID, privCreated.ID, "Private Action", "Desc", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		// Member sees both actions
		memberActions, err := actionUC.ListActions(memberCtx, testWorkspaceID, interfaces.ActionListOptions{})
		gt.NoError(t, err).Required()
		gt.Array(t, memberActions).Length(2)

		// Non-member sees only public action
		nonMemberActions, err := actionUC.ListActions(nonMemberCtx, testWorkspaceID, interfaces.ActionListOptions{})
		gt.NoError(t, err).Required()
		gt.Array(t, nonMemberActions).Length(1)
		gt.Value(t, nonMemberActions[0].Title).Equal("Public Action")
	})

	t.Run("get actions by private case as non-member returns empty", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		memberCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMEMBER"})
		nonMemberCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UOTHER"})

		privateCase := &model.Case{
			Title:          "Private Case",
			Description:    "Secret",
			IsPrivate:      true,
			ChannelUserIDs: []string{"UMEMBER"},
			AssigneeIDs:    []string{},
		}
		created, err := repo.Case().Create(memberCtx, testWorkspaceID, privateCase)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(memberCtx, testWorkspaceID, created.ID, "Action", "Desc", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		// Member sees actions
		memberActions, err := actionUC.GetActionsByCase(memberCtx, testWorkspaceID, created.ID, interfaces.ActionListOptions{})
		gt.NoError(t, err).Required()
		gt.Array(t, memberActions).Length(1)

		// Non-member gets empty list
		nonMemberActions, err := actionUC.GetActionsByCase(nonMemberCtx, testWorkspaceID, created.ID, interfaces.ActionListOptions{})
		gt.NoError(t, err).Required()
		gt.Array(t, nonMemberActions).Length(0)
	})
}
