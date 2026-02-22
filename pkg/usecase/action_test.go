package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	goslack "github.com/slack-go/slack"
)

func TestActionUseCase_CreateAction(t *testing.T) {
	t.Run("create action with valid case", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		// Create case first
		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		// Create action
		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Action Description", []string{"U001"}, "msg-123", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		gt.Number(t, created.ID).NotEqual(0)
		gt.Value(t, created.CaseID).Equal(c.ID)
		gt.Value(t, created.Title).Equal("Test Action")
		gt.Value(t, created.SlackMessageTS).Equal("msg-123")
		gt.Value(t, created.Status).Equal(types.ActionStatusTodo)
	})

	t.Run("create action without title fails", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "", "Description", []string{}, "", types.ActionStatusTodo, nil)
		gt.Value(t, err).NotNil()
	})

	t.Run("create action with non-existent case fails", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		_, err := actionUC.CreateAction(ctx, testWorkspaceID, 999, "Test Action", "Description", []string{}, "", types.ActionStatusTodo, nil)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})

	t.Run("create action with invalid status fails", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Description", []string{}, "", "invalid-status", nil)
		gt.Value(t, err).NotNil()
	})

	t.Run("create action with default status", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Description", []string{}, "", "", nil)
		gt.NoError(t, err).Required()

		gt.Value(t, created.Status).Equal(types.ActionStatusTodo)
	})
}

func TestActionUseCase_UpdateAction(t *testing.T) {
	t.Run("update action title and status", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Original Title", "Original Description", []string{"U001"}, "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		newTitle := "Updated Title"
		newStatus := types.ActionStatusInProgress
		updated, err := actionUC.UpdateAction(ctx, testWorkspaceID, created.ID, nil, &newTitle, nil, nil, nil, &newStatus, nil, false)
		gt.NoError(t, err).Required()

		gt.Value(t, updated.Title).Equal("Updated Title")
		gt.Value(t, updated.Status).Equal(types.ActionStatusInProgress)
		gt.Value(t, updated.CaseID).Equal(c.ID)
	})

	t.Run("update action caseID", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		c1, err := caseUC.CreateCase(ctx, testWorkspaceID, "Case 1", "Description 1", []string{}, nil)
		gt.NoError(t, err).Required()

		c2, err := caseUC.CreateCase(ctx, testWorkspaceID, "Case 2", "Description 2", []string{}, nil)
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c1.ID, "Test Action", "Description", []string{}, "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		updated, err := actionUC.UpdateAction(ctx, testWorkspaceID, created.ID, &c2.ID, nil, nil, nil, nil, nil, nil, false)
		gt.NoError(t, err).Required()

		gt.Value(t, updated.CaseID).Equal(c2.ID)
	})

	t.Run("update action with non-existent case fails", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Description", []string{}, "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		newCaseID := int64(999)
		_, err = actionUC.UpdateAction(ctx, testWorkspaceID, created.ID, &newCaseID, nil, nil, nil, nil, nil, nil, false)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})

	t.Run("update non-existent action fails", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		_, err := actionUC.UpdateAction(ctx, testWorkspaceID, 999, nil, nil, nil, nil, nil, nil, nil, false)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrActionNotFound)
	})
}

func TestActionUseCase_DeleteAction(t *testing.T) {
	t.Run("delete action", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Description", []string{}, "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		gt.NoError(t, actionUC.DeleteAction(ctx, testWorkspaceID, created.ID)).Required()

		_, err = actionUC.GetAction(ctx, testWorkspaceID, created.ID)
		gt.Value(t, err).NotNil()
	})

	t.Run("delete non-existent action fails", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		err := actionUC.DeleteAction(ctx, testWorkspaceID, 999)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrActionNotFound)
	})
}

func TestActionUseCase_GetAction(t *testing.T) {
	t.Run("get existing action", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Description", []string{}, "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		retrieved, err := actionUC.GetAction(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, retrieved.ID).Equal(created.ID)
		gt.Value(t, retrieved.Title).Equal(created.Title)
	})

	t.Run("get non-existent action fails", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		_, err := actionUC.GetAction(ctx, testWorkspaceID, 999)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrActionNotFound)
	})
}

func TestActionUseCase_ListActions(t *testing.T) {
	t.Run("list actions", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Action 1", "Description 1", []string{}, "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Action 2", "Description 2", []string{}, "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		actions, err := actionUC.ListActions(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()

		gt.Array(t, actions).Length(2)
	})
}

func TestActionUseCase_GetActionsByCase(t *testing.T) {
	t.Run("get actions by case", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		c1, err := caseUC.CreateCase(ctx, testWorkspaceID, "Case 1", "Description 1", []string{}, nil)
		gt.NoError(t, err).Required()

		c2, err := caseUC.CreateCase(ctx, testWorkspaceID, "Case 2", "Description 2", []string{}, nil)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c1.ID, "Action 1-1", "Description", []string{}, "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c1.ID, "Action 1-2", "Description", []string{}, "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c2.ID, "Action 2-1", "Description", []string{}, "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		actions, err := actionUC.GetActionsByCase(ctx, testWorkspaceID, c1.ID)
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
		caseUC := usecase.NewCaseUseCase(repo, nil, mock, "")
		actionUC := usecase.NewActionUseCase(repo, mock, "https://example.com")
		ctx := context.Background()

		// Create case with Slack channel
		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()
		gt.Value(t, c.SlackChannelID).NotEqual("")

		// Create action
		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Action Desc", []string{"U001"}, "", types.ActionStatusTodo, nil)
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
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, mock, "https://example.com")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()
		gt.Value(t, c.SlackChannelID).Equal("")

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Desc", []string{}, "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		// Slack message should NOT have been posted
		gt.Value(t, mock.postMessageCalled).Equal(false)
	})

	t.Run("does not post when slack service is nil", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "https://example.com")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Desc", []string{}, "", types.ActionStatusTodo, nil)
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
		caseUC := usecase.NewCaseUseCase(repo, nil, mock, "")
		actionUC := usecase.NewActionUseCase(repo, mock, "https://example.com")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		// Action creation should still succeed
		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Desc", []string{}, "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()
		gt.Value(t, created.Title).Equal("Test Action")
		gt.Value(t, created.SlackMessageTS).Equal("") // No TS because posting failed
	})
}

func TestActionUseCase_HandleSlackInteraction(t *testing.T) {
	setup := func(t *testing.T) (*memory.Repository, *usecase.ActionUseCase, *actionTestSlackMock, *model.Case, *model.Action) {
		t.Helper()
		repo := memory.New()
		mock := &actionTestSlackMock{
			mockSlackService: mockSlackService{
				createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
					return fmt.Sprintf("C%d", caseID), nil
				},
			},
			postMessageTS: "1234567890.123456",
		}
		caseUC := usecase.NewCaseUseCase(repo, nil, mock, "")
		actionUC := usecase.NewActionUseCase(repo, mock, "https://example.com")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		action, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Desc", []string{"U001"}, "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		// Reset mock tracking after creation
		mock.updateMessageCalled = false

		return repo, actionUC, mock, c, action
	}

	t.Run("assign adds user to assignees", func(t *testing.T) {
		repo, actionUC, mock, _, action := setup(t)
		ctx := context.Background()

		err := actionUC.HandleSlackInteraction(ctx, testWorkspaceID, action.ID, "UNEW", usecase.SlackActionIDAssign)
		gt.NoError(t, err).Required()

		// Verify assignee was added
		updated, err := repo.Action().Get(ctx, testWorkspaceID, action.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, updated.AssigneeIDs).Length(2)
		gt.Value(t, updated.AssigneeIDs[0]).Equal("U001")
		gt.Value(t, updated.AssigneeIDs[1]).Equal("UNEW")

		// Verify Slack message was updated
		gt.Value(t, mock.updateMessageCalled).Equal(true)
	})

	t.Run("assign does not duplicate existing user", func(t *testing.T) {
		repo, actionUC, _, _, action := setup(t)
		ctx := context.Background()

		err := actionUC.HandleSlackInteraction(ctx, testWorkspaceID, action.ID, "U001", usecase.SlackActionIDAssign)
		gt.NoError(t, err).Required()

		updated, err := repo.Action().Get(ctx, testWorkspaceID, action.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, updated.AssigneeIDs).Length(1)
		gt.Value(t, updated.AssigneeIDs[0]).Equal("U001")
	})

	t.Run("in_progress changes status", func(t *testing.T) {
		repo, actionUC, _, _, action := setup(t)
		ctx := context.Background()

		err := actionUC.HandleSlackInteraction(ctx, testWorkspaceID, action.ID, "U001", usecase.SlackActionIDInProgress)
		gt.NoError(t, err).Required()

		updated, err := repo.Action().Get(ctx, testWorkspaceID, action.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, updated.Status).Equal(types.ActionStatusInProgress)
	})

	t.Run("complete changes status", func(t *testing.T) {
		repo, actionUC, _, _, action := setup(t)
		ctx := context.Background()

		err := actionUC.HandleSlackInteraction(ctx, testWorkspaceID, action.ID, "U001", usecase.SlackActionIDComplete)
		gt.NoError(t, err).Required()

		updated, err := repo.Action().Get(ctx, testWorkspaceID, action.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, updated.Status).Equal(types.ActionStatusCompleted)
	})

	t.Run("unknown action type is ignored", func(t *testing.T) {
		repo, actionUC, _, _, action := setup(t)
		ctx := context.Background()

		err := actionUC.HandleSlackInteraction(ctx, testWorkspaceID, action.ID, "U001", "unknown_action")
		gt.NoError(t, err).Required()

		// Action should be unchanged
		unchanged, err := repo.Action().Get(ctx, testWorkspaceID, action.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, unchanged.Status).Equal(types.ActionStatusTodo)
	})

	t.Run("non-existent action returns error", func(t *testing.T) {
		_, actionUC, _, _, _ := setup(t)
		ctx := context.Background()

		err := actionUC.HandleSlackInteraction(ctx, testWorkspaceID, 99999, "U001", usecase.SlackActionIDAssign)
		gt.Value(t, err).NotNil()
	})
}

func TestParseSlackActionValue(t *testing.T) {
	t.Run("valid value", func(t *testing.T) {
		ws, id, err := usecase.ParseSlackActionValue("test-ws:42")
		gt.NoError(t, err).Required()
		gt.Value(t, ws).Equal("test-ws")
		gt.Value(t, id).Equal(int64(42))
	})

	t.Run("workspace with hyphen", func(t *testing.T) {
		ws, id, err := usecase.ParseSlackActionValue("my-workspace:123")
		gt.NoError(t, err).Required()
		gt.Value(t, ws).Equal("my-workspace")
		gt.Value(t, id).Equal(int64(123))
	})

	t.Run("invalid format without colon", func(t *testing.T) {
		_, _, err := usecase.ParseSlackActionValue("invalid")
		gt.Value(t, err).NotNil()
	})

	t.Run("invalid action ID", func(t *testing.T) {
		_, _, err := usecase.ParseSlackActionValue("ws:notanumber")
		gt.Value(t, err).NotNil()
	})
}

func TestActionUseCase_DueDate(t *testing.T) {
	t.Run("create action with due date", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		dueDate := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Action with due date", "Description", []string{}, "", types.ActionStatusTodo, &dueDate)
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
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Action without due date", "Description", []string{}, "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		gt.Value(t, created.DueDate == nil).Equal(true)
	})

	t.Run("update action to set due date", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Action", "Description", []string{}, "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()
		gt.Value(t, created.DueDate == nil).Equal(true)

		dueDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
		updated, err := actionUC.UpdateAction(ctx, testWorkspaceID, created.ID, nil, nil, nil, nil, nil, nil, &dueDate, false)
		gt.NoError(t, err).Required()

		gt.Value(t, updated.DueDate).NotNil()
		gt.Value(t, updated.DueDate.Equal(dueDate)).Equal(true)
	})

	t.Run("update action to clear due date", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		dueDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Action", "Description", []string{}, "", types.ActionStatusTodo, &dueDate)
		gt.NoError(t, err).Required()
		gt.Value(t, created.DueDate).NotNil()

		// Clear due date using clearDueDate=true
		updated, err := actionUC.UpdateAction(ctx, testWorkspaceID, created.ID, nil, nil, nil, nil, nil, nil, nil, true)
		gt.NoError(t, err).Required()

		gt.Value(t, updated.DueDate == nil).Equal(true)
	})

	t.Run("update action without changing due date preserves it", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		dueDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Action", "Description", []string{}, "", types.ActionStatusTodo, &dueDate)
		gt.NoError(t, err).Required()

		// Update only title, leave dueDate unchanged (nil, false)
		newTitle := "Updated Title"
		updated, err := actionUC.UpdateAction(ctx, testWorkspaceID, created.ID, nil, &newTitle, nil, nil, nil, nil, nil, false)
		gt.NoError(t, err).Required()

		gt.Value(t, updated.Title).Equal("Updated Title")
		gt.Value(t, updated.DueDate).NotNil()
		gt.Value(t, updated.DueDate.Equal(dueDate)).Equal(true)
	})
}
