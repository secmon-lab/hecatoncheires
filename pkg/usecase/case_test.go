package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

const testWorkspaceID = "test-ws"

func TestCaseUseCase_CreateCase(t *testing.T) {
	t.Run("create case with valid fields", func(t *testing.T) {
		repo := memory.New()
		fieldSchema := &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{
					ID:       "priority",
					Name:     "Priority",
					Type:     types.FieldTypeSelect,
					Required: true,
					Options: []config.FieldOption{
						{ID: "high", Name: "High"},
						{ID: "low", Name: "Low"},
					},
				},
			},
			Labels: config.EntityLabels{
				Case: "Case",
			},
		}

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:   model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			FieldSchema: fieldSchema,
		})
		uc := usecase.NewCaseUseCase(repo, registry, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		fieldValues := map[string]model.FieldValue{
			"priority": {FieldID: "priority", Value: "high"},
		}

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{"U001"}, fieldValues, false)
		gt.NoError(t, err).Required()

		gt.Number(t, created.ID).NotEqual(0)
		gt.Value(t, created.Title).Equal("Test Case")
		gt.Value(t, created.Description).Equal("Description")

		// Verify field values are embedded in the case
		retrieved, err := uc.GetCase(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()
		gt.Number(t, len(retrieved.FieldValues)).Equal(1)
		gt.Value(t, retrieved.FieldValues["priority"].Value).Equal("high")
		gt.Value(t, retrieved.FieldValues["priority"].Type).Equal(types.FieldTypeSelect)
	})

	t.Run("create case without title fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := uc.CreateCase(ctx, testWorkspaceID, "", "Description", []string{}, nil, false)
		gt.Value(t, err).NotNil()
	})

	t.Run("create case with invalid field fails", func(t *testing.T) {
		repo := memory.New()
		fieldSchema := &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{
					ID:       "priority",
					Name:     "Priority",
					Type:     types.FieldTypeSelect,
					Required: true,
					Options: []config.FieldOption{
						{ID: "high", Name: "High"},
					},
				},
			},
		}

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:   model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			FieldSchema: fieldSchema,
		})
		uc := usecase.NewCaseUseCase(repo, registry, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		fieldValues := map[string]model.FieldValue{
			"priority": {FieldID: "priority", Value: "invalid"},
		}

		_, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, fieldValues, false)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(model.ErrInvalidOptionID)
	})

	t.Run("create case with missing required field fails", func(t *testing.T) {
		repo := memory.New()
		fieldSchema := &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{
					ID:       "priority",
					Name:     "Priority",
					Type:     types.FieldTypeText,
					Required: true,
				},
			},
		}

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:   model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			FieldSchema: fieldSchema,
		})
		uc := usecase.NewCaseUseCase(repo, registry, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(model.ErrMissingRequired)
	})
}

func TestCaseUseCase_UpdateCase(t *testing.T) {
	t.Run("update case with valid fields", func(t *testing.T) {
		repo := memory.New()
		fieldSchema := &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{
					ID:       "priority",
					Name:     "Priority",
					Type:     types.FieldTypeSelect,
					Required: true,
					Options: []config.FieldOption{
						{ID: "high", Name: "High"},
						{ID: "low", Name: "Low"},
					},
				},
			},
		}

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:   model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			FieldSchema: fieldSchema,
		})
		uc := usecase.NewCaseUseCase(repo, registry, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		// Create case first
		fieldValues := map[string]model.FieldValue{
			"priority": {FieldID: "priority", Value: "high"},
		}
		created, err := uc.CreateCase(ctx, testWorkspaceID, "Original Title", "Original Description", []string{"U001"}, fieldValues, false)
		gt.NoError(t, err).Required()

		// Update case
		updatedFieldValues := map[string]model.FieldValue{
			"priority": {FieldID: "priority", Value: "low"},
		}
		updated, err := uc.UpdateCase(ctx, testWorkspaceID, created.ID, "Updated Title", "Updated Description", []string{"U002"}, updatedFieldValues)
		gt.NoError(t, err).Required()

		gt.Value(t, updated.Title).Equal("Updated Title")
		gt.Value(t, updated.Description).Equal("Updated Description")

		// Verify field values were updated
		retrieved, err := uc.GetCase(ctx, testWorkspaceID, updated.ID)
		gt.NoError(t, err).Required()
		gt.Number(t, len(retrieved.FieldValues)).Equal(1)
		gt.Value(t, retrieved.FieldValues["priority"].Value).Equal("low")
	})

	t.Run("update non-existent case fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := uc.UpdateCase(ctx, testWorkspaceID, 999, "Title", "Description", []string{}, nil)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})
}

func TestCaseUseCase_DeleteCase(t *testing.T) {
	t.Run("delete case with actions", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		// Create case
		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false)
		gt.NoError(t, err).Required()

		// Create action for the case
		_, err = actionUC.CreateAction(ctx, testWorkspaceID, created.ID, "Test Action", "Action Description", []string{}, "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		// Delete case
		gt.NoError(t, uc.DeleteCase(ctx, testWorkspaceID, created.ID)).Required()

		// Verify case is deleted
		_, err = uc.GetCase(ctx, testWorkspaceID, created.ID)
		gt.Value(t, err).NotNil()

		// Verify actions are deleted
		actions, err := actionUC.GetActionsByCase(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, actions).Length(0)
	})

	t.Run("delete non-existent case fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		err := uc.DeleteCase(ctx, testWorkspaceID, 999)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})
}

func TestCaseUseCase_GetCase(t *testing.T) {
	t.Run("get existing case", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false)
		gt.NoError(t, err).Required()

		retrieved, err := uc.GetCase(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, retrieved.ID).Equal(created.ID)
		gt.Value(t, retrieved.Title).Equal(created.Title)
	})

	t.Run("get non-existent case fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := uc.GetCase(ctx, testWorkspaceID, 999)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})
}

func TestCaseUseCase_ListCases(t *testing.T) {
	t.Run("list cases", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		// Create multiple cases
		_, err := uc.CreateCase(ctx, testWorkspaceID, "Case 1", "Description 1", []string{}, nil, false)
		gt.NoError(t, err).Required()

		_, err = uc.CreateCase(ctx, testWorkspaceID, "Case 2", "Description 2", []string{}, nil, false)
		gt.NoError(t, err).Required()

		cases, err := uc.ListCases(ctx, testWorkspaceID, nil)
		gt.NoError(t, err).Required()

		gt.Array(t, cases).Length(2)
	})

	t.Run("list cases with status filter", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		// Create cases (all default to OPEN)
		case1, err := uc.CreateCase(ctx, testWorkspaceID, "Open Case 1", "desc", []string{}, nil, false)
		gt.NoError(t, err).Required()

		_, err = uc.CreateCase(ctx, testWorkspaceID, "Open Case 2", "desc", []string{}, nil, false)
		gt.NoError(t, err).Required()

		// Close one case
		_, err = uc.CloseCase(ctx, testWorkspaceID, case1.ID)
		gt.NoError(t, err).Required()

		// Filter by OPEN status
		openStatus := types.CaseStatusOpen
		openCases, err := uc.ListCases(ctx, testWorkspaceID, &openStatus)
		gt.NoError(t, err).Required()
		gt.Array(t, openCases).Length(1)
		gt.Value(t, openCases[0].Title).Equal("Open Case 2")

		// Filter by CLOSED status
		closedStatus := types.CaseStatusClosed
		closedCases, err := uc.ListCases(ctx, testWorkspaceID, &closedStatus)
		gt.NoError(t, err).Required()
		gt.Array(t, closedCases).Length(1)
		gt.Value(t, closedCases[0].Title).Equal("Open Case 1")
	})
}

func TestCaseUseCase_CreateCase_DefaultStatus(t *testing.T) {
	repo := memory.New()
	uc := usecase.NewCaseUseCase(repo, nil, nil, "")
	ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

	created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false)
	gt.NoError(t, err).Required()
	gt.Value(t, created.Status).Equal(types.CaseStatusOpen)

	// Verify through GetCase as well
	retrieved, err := uc.GetCase(ctx, testWorkspaceID, created.ID)
	gt.NoError(t, err).Required()
	gt.Value(t, retrieved.Status).Equal(types.CaseStatusOpen)
}

func TestCaseUseCase_CloseCase(t *testing.T) {
	t.Run("close an open case", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false)
		gt.NoError(t, err).Required()
		gt.Value(t, created.Status).Equal(types.CaseStatusOpen)

		closed, err := uc.CloseCase(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, closed.Status).Equal(types.CaseStatusClosed)
		gt.Value(t, closed.ID).Equal(created.ID)
		gt.Value(t, closed.Title).Equal("Test Case")
	})

	t.Run("close an already closed case fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false)
		gt.NoError(t, err).Required()

		_, err = uc.CloseCase(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()

		_, err = uc.CloseCase(ctx, testWorkspaceID, created.ID)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseAlreadyClosed)
	})

	t.Run("close non-existent case fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := uc.CloseCase(ctx, testWorkspaceID, 999)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})
}

func TestCaseUseCase_ReopenCase(t *testing.T) {
	t.Run("reopen a closed case", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false)
		gt.NoError(t, err).Required()

		_, err = uc.CloseCase(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()

		reopened, err := uc.ReopenCase(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, reopened.Status).Equal(types.CaseStatusOpen)
		gt.Value(t, reopened.ID).Equal(created.ID)
		gt.Value(t, reopened.Title).Equal("Test Case")
	})

	t.Run("reopen an already open case fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false)
		gt.NoError(t, err).Required()

		_, err = uc.ReopenCase(ctx, testWorkspaceID, created.ID)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseAlreadyOpen)
	})

	t.Run("reopen non-existent case fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := uc.ReopenCase(ctx, testWorkspaceID, 999)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})
}

func TestCaseUseCase_GetFieldConfiguration(t *testing.T) {
	t.Run("get field configuration with schema", func(t *testing.T) {
		repo := memory.New()
		fieldSchema := &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "priority", Name: "Priority", Type: types.FieldTypeText},
			},
			Labels: config.EntityLabels{
				Case: "Case",
			},
		}

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:   model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			FieldSchema: fieldSchema,
		})
		uc := usecase.NewCaseUseCase(repo, registry, nil, "")

		cfg := uc.GetFieldConfiguration(testWorkspaceID)
		gt.Array(t, cfg.Fields).Length(1)
		gt.Value(t, cfg.Fields[0].ID).Equal("priority")
	})

	t.Run("get field configuration without schema", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")

		cfg := uc.GetFieldConfiguration(testWorkspaceID)
		gt.Array(t, cfg.Fields).Length(0)
		gt.Value(t, cfg.Labels.Case).Equal("Case")
	})
}

func TestCaseUseCase_CreateCase_SlackInvite(t *testing.T) {
	t.Run("invites creator and assignees to channel", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock, "")

		token := auth.NewToken("UCREATOR", "creator@example.com", "Creator")
		ctx := auth.ContextWithToken(context.Background(), token)

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{"UASSIGNEE1", "UASSIGNEE2"}, nil, false)
		gt.NoError(t, err).Required()
		gt.Value(t, created.SlackChannelID).NotEqual("")

		// Verify creator is first, followed by assignees
		gt.Array(t, mock.invitedUserIDs).Length(3)
		gt.Value(t, mock.invitedUserIDs[0]).Equal("UCREATOR")
		gt.Value(t, mock.invitedUserIDs[1]).Equal("UASSIGNEE1")
		gt.Value(t, mock.invitedUserIDs[2]).Equal("UASSIGNEE2")
		gt.Value(t, mock.invitedChannelID).Equal(created.SlackChannelID)
	})

	t.Run("deduplicates creator in assignees", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock, "")

		token := auth.NewToken("UCREATOR", "creator@example.com", "Creator")
		ctx := auth.ContextWithToken(context.Background(), token)

		_, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{"UCREATOR", "UOTHER"}, nil, false)
		gt.NoError(t, err).Required()

		// UCREATOR should appear only once
		gt.Array(t, mock.invitedUserIDs).Length(2)
		gt.Value(t, mock.invitedUserIDs[0]).Equal("UCREATOR")
		gt.Value(t, mock.invitedUserIDs[1]).Equal("UOTHER")
	})

	t.Run("invite failure does not fail case creation", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
			inviteUsersToChannelFn: func(_ context.Context, _ string, _ []string) error {
				return errors.New("slack invite error")
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock, "")

		token := auth.NewToken("UCREATOR", "creator@example.com", "Creator")
		ctx := auth.ContextWithToken(context.Background(), token)

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{"UASSIGNEE"}, nil, false)
		gt.NoError(t, err).Required()
		gt.Value(t, created.SlackChannelID).NotEqual("")
	})

	t.Run("invites creator and assignees with auth token", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock, "")

		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{"UASSIGNEE"}, nil, false)
		gt.NoError(t, err).Required()

		// Creator and assignee should be invited
		gt.Array(t, mock.invitedUserIDs).Length(2)
		gt.Value(t, mock.invitedUserIDs[0]).Equal("UTESTUSER")
		gt.Value(t, mock.invitedUserIDs[1]).Equal("UASSIGNEE")
	})

	t.Run("invites only creator when no assignees", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock, "")

		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false)
		gt.NoError(t, err).Required()

		// Only creator should be invited
		gt.Array(t, mock.invitedUserIDs).Length(1)
		gt.Value(t, mock.invitedUserIDs[0]).Equal("UTESTUSER")
	})
}

func TestCaseUseCase_CreateCase_BookmarkAndMapping(t *testing.T) {
	t.Run("adds bookmark and saves mapping when baseURL is set", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock, "https://example.com")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false)
		gt.NoError(t, err).Required()

		// Verify bookmark was added
		gt.Value(t, mock.bookmarkChannelID).Equal(created.SlackChannelID)
		gt.Value(t, mock.bookmarkTitle).Equal("Open Case")
		expectedURL := fmt.Sprintf("https://example.com/ws/%s/cases/%d", testWorkspaceID, created.ID)
		gt.Value(t, mock.bookmarkLink).Equal(expectedURL)
	})

	t.Run("skips bookmark when baseURL is empty", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false)
		gt.NoError(t, err).Required()

		// Bookmark should not have been added
		gt.Value(t, mock.bookmarkChannelID).Equal("")
	})

	t.Run("bookmark failure does not fail case creation", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
			addBookmarkFn: func(_ context.Context, _, _, _ string) error {
				return errors.New("bookmark error")
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock, "https://example.com")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false)
		gt.NoError(t, err).Required()
		gt.Value(t, created.SlackChannelID).NotEqual("")
	})

}

func TestCaseUseCase_PrivateCaseAccessControl(t *testing.T) {
	t.Run("create private case sets IsPrivate flag", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Private Case", "Secret", []string{}, nil, true)
		gt.NoError(t, err).Required()
		gt.Value(t, created.IsPrivate).Equal(true)
	})

	t.Run("get private case as member returns full case", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMEMBER"})

		// Create case directly in repo with IsPrivate and ChannelUserIDs
		caseModel := &model.Case{
			Title:          "Private Case",
			Description:    "Secret desc",
			Status:         types.CaseStatusOpen,
			IsPrivate:      true,
			ChannelUserIDs: []string{"UMEMBER", "UOTHER"},
		}
		created, err := repo.Case().Create(ctx, testWorkspaceID, caseModel)
		gt.NoError(t, err).Required()

		// Get as member
		retrieved, err := uc.GetCase(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, retrieved.Title).Equal("Private Case")
		gt.Value(t, retrieved.Description).Equal("Secret desc")
		gt.Value(t, retrieved.AccessDenied).Equal(false)
	})

	t.Run("get private case as non-member returns restricted case", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")

		// Create case as a different user
		adminCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UADMIN"})
		caseModel := &model.Case{
			Title:          "Private Case",
			Description:    "Secret desc",
			Status:         types.CaseStatusOpen,
			IsPrivate:      true,
			ChannelUserIDs: []string{"UADMIN"},
		}
		created, err := repo.Case().Create(adminCtx, testWorkspaceID, caseModel)
		gt.NoError(t, err).Required()

		// Get as non-member
		nonMemberCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "USTRANGER"})
		retrieved, err := uc.GetCase(nonMemberCtx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, retrieved.AccessDenied).Equal(true)
		gt.Value(t, retrieved.Title).Equal("")
		gt.Value(t, retrieved.Description).Equal("")
		gt.Value(t, retrieved.IsPrivate).Equal(true)
		gt.Value(t, retrieved.ID).Equal(created.ID)
	})

	t.Run("list cases restricts private cases for non-members", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMEMBER"})

		// Create public case
		_, err := uc.CreateCase(ctx, testWorkspaceID, "Public Case", "Visible", []string{}, nil, false)
		gt.NoError(t, err).Required()

		// Create private case with UMEMBER as member
		privCase := &model.Case{
			Title:          "Private Visible",
			Description:    "Visible to member",
			Status:         types.CaseStatusOpen,
			IsPrivate:      true,
			ChannelUserIDs: []string{"UMEMBER"},
		}
		_, err = repo.Case().Create(ctx, testWorkspaceID, privCase)
		gt.NoError(t, err).Required()

		// Create private case without UMEMBER
		privCase2 := &model.Case{
			Title:          "Private Hidden",
			Description:    "Not visible",
			Status:         types.CaseStatusOpen,
			IsPrivate:      true,
			ChannelUserIDs: []string{"UOTHER"},
		}
		_, err = repo.Case().Create(ctx, testWorkspaceID, privCase2)
		gt.NoError(t, err).Required()

		// List cases
		cases, err := uc.ListCases(ctx, testWorkspaceID, nil)
		gt.NoError(t, err).Required()
		gt.Array(t, cases).Length(3)

		// Check that private hidden case is restricted
		var restrictedCount int
		for _, c := range cases {
			if c.AccessDenied {
				restrictedCount++
				gt.Value(t, c.Title).Equal("")
			}
		}
		gt.Value(t, restrictedCount).Equal(1)
	})

	t.Run("update private case as non-member returns access denied", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")

		adminCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UADMIN"})
		caseModel := &model.Case{
			Title:          "Private Case",
			Description:    "Secret",
			Status:         types.CaseStatusOpen,
			IsPrivate:      true,
			ChannelUserIDs: []string{"UADMIN"},
		}
		created, err := repo.Case().Create(adminCtx, testWorkspaceID, caseModel)
		gt.NoError(t, err).Required()

		nonMemberCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "USTRANGER"})
		_, err = uc.UpdateCase(nonMemberCtx, testWorkspaceID, created.ID, "Hacked", "Hacked desc", []string{}, nil)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.TestErrAccessDenied)
	})

	t.Run("delete private case as non-member returns access denied", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")

		adminCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UADMIN"})
		caseModel := &model.Case{
			Title:          "Private Case",
			Description:    "Secret",
			Status:         types.CaseStatusOpen,
			IsPrivate:      true,
			ChannelUserIDs: []string{"UADMIN"},
		}
		created, err := repo.Case().Create(adminCtx, testWorkspaceID, caseModel)
		gt.NoError(t, err).Required()

		nonMemberCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "USTRANGER"})
		err = uc.DeleteCase(nonMemberCtx, testWorkspaceID, created.ID)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.TestErrAccessDenied)
	})

	t.Run("close private case as non-member returns access denied", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")

		adminCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UADMIN"})
		caseModel := &model.Case{
			Title:          "Private Case",
			Description:    "Secret",
			Status:         types.CaseStatusOpen,
			IsPrivate:      true,
			ChannelUserIDs: []string{"UADMIN"},
		}
		created, err := repo.Case().Create(adminCtx, testWorkspaceID, caseModel)
		gt.NoError(t, err).Required()

		nonMemberCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "USTRANGER"})
		_, err = uc.CloseCase(nonMemberCtx, testWorkspaceID, created.ID)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.TestErrAccessDenied)
	})

	t.Run("public case is accessible without restrictions", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UANYONE"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Public Case", "Visible", []string{}, nil, false)
		gt.NoError(t, err).Required()

		retrieved, err := uc.GetCase(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, retrieved.AccessDenied).Equal(false)
		gt.Value(t, retrieved.Title).Equal("Public Case")
	})

	t.Run("backward compatibility: existing case with nil ChannelUserIDs is public", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UANYONE"})

		// Create a case that simulates existing data (IsPrivate=false, ChannelUserIDs=nil)
		caseModel := &model.Case{
			Title:       "Legacy Case",
			Description: "Old case",
			Status:      types.CaseStatusOpen,
			// IsPrivate defaults to false, ChannelUserIDs defaults to nil
		}
		created, err := repo.Case().Create(ctx, testWorkspaceID, caseModel)
		gt.NoError(t, err).Required()

		retrieved, err := uc.GetCase(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, retrieved.AccessDenied).Equal(false)
		gt.Value(t, retrieved.Title).Equal("Legacy Case")
	})
}

func TestCaseUseCase_SyncCaseChannelUsers(t *testing.T) {
	t.Run("sync updates channel user IDs", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		// Seed SlackUser cache so filterHumanUsers can identify real users
		err := repo.SlackUser().SaveMany(ctx, []*model.SlackUser{
			{ID: "U001", Name: "user1", RealName: "User One"},
			{ID: "U002", Name: "user2", RealName: "User Two"},
			{ID: "U003", Name: "user3", RealName: "User Three"},
		})
		gt.NoError(t, err).Required()

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Desc", []string{}, nil, false)
		gt.NoError(t, err).Required()
		gt.Value(t, created.SlackChannelID).NotEqual("")

		// Override GetConversationMembers to return specific members
		mock.getConversationMembersFn = func(_ context.Context, _ string) ([]string, error) {
			return []string{"U001", "U002", "U003"}, nil
		}

		synced, err := uc.SyncCaseChannelUsers(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, synced.ChannelUserIDs).Length(3)
	})

	t.Run("sync fails when case has no slack channel", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		// Create case without slack service (no channel)
		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Desc", []string{}, nil, false)
		gt.NoError(t, err).Required()

		_, err = uc.SyncCaseChannelUsers(ctx, testWorkspaceID, created.ID)
		gt.Value(t, err).NotNil()
	})

	t.Run("sync fails for non-existent case", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := uc.SyncCaseChannelUsers(ctx, testWorkspaceID, 999)
		gt.Value(t, err).NotNil()
	})
}
