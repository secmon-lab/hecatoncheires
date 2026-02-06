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

		uc := usecase.NewCaseUseCase(repo, fieldSchema, nil)
		ctx := context.Background()

		fields := []model.FieldValue{
			{
				FieldID: "priority",
				Value:   "high",
			},
		}

		created, err := uc.CreateCase(ctx, "Test Case", "Description", []string{"U001"}, fields)
		gt.NoError(t, err).Required()

		gt.Number(t, created.ID).NotEqual(0)
		gt.Value(t, created.Title).Equal("Test Case")
		gt.Value(t, created.Description).Equal("Description")

		// Verify field values were saved
		savedFields, err := uc.GetCaseFieldValues(ctx, created.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, savedFields).Length(1)
	})

	t.Run("create case without title fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil)
		ctx := context.Background()

		_, err := uc.CreateCase(ctx, "", "Description", []string{}, nil)
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

		uc := usecase.NewCaseUseCase(repo, fieldSchema, nil)
		ctx := context.Background()

		fields := []model.FieldValue{
			{
				FieldID: "priority",
				Value:   "invalid",
			},
		}

		_, err := uc.CreateCase(ctx, "Test Case", "Description", []string{}, fields)
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

		uc := usecase.NewCaseUseCase(repo, fieldSchema, nil)
		ctx := context.Background()

		_, err := uc.CreateCase(ctx, "Test Case", "Description", []string{}, nil)
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

		uc := usecase.NewCaseUseCase(repo, fieldSchema, nil)
		ctx := context.Background()

		// Create case first
		fields := []model.FieldValue{
			{FieldID: "priority", Value: "high"},
		}
		created, err := uc.CreateCase(ctx, "Original Title", "Original Description", []string{"U001"}, fields)
		gt.NoError(t, err).Required()

		// Update case
		updatedFields := []model.FieldValue{
			{FieldID: "priority", Value: "low"},
		}
		updated, err := uc.UpdateCase(ctx, created.ID, "Updated Title", "Updated Description", []string{"U002"}, updatedFields)
		gt.NoError(t, err).Required()

		gt.Value(t, updated.Title).Equal("Updated Title")
		gt.Value(t, updated.Description).Equal("Updated Description")

		// Verify field values were updated
		savedFields, err := uc.GetCaseFieldValues(ctx, updated.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, savedFields).Length(1)
		gt.Value(t, savedFields[0].Value).Equal("low")
	})

	t.Run("update non-existent case fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil)
		ctx := context.Background()

		_, err := uc.UpdateCase(ctx, 999, "Title", "Description", []string{}, nil)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})
}

func TestCaseUseCase_DeleteCase(t *testing.T) {
	t.Run("delete case with actions", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil)
		actionUC := usecase.NewActionUseCase(repo)
		ctx := context.Background()

		// Create case
		created, err := uc.CreateCase(ctx, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		// Create action for the case
		_, err = actionUC.CreateAction(ctx, created.ID, "Test Action", "Action Description", []string{}, "", types.ActionStatusTodo)
		gt.NoError(t, err).Required()

		// Delete case
		gt.NoError(t, uc.DeleteCase(ctx, created.ID)).Required()

		// Verify case is deleted
		_, err = uc.GetCase(ctx, created.ID)
		gt.Value(t, err).NotNil()

		// Verify actions are deleted
		actions, err := actionUC.GetActionsByCase(ctx, created.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, actions).Length(0)
	})

	t.Run("delete non-existent case fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil)
		ctx := context.Background()

		err := uc.DeleteCase(ctx, 999)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})
}

func TestCaseUseCase_GetCase(t *testing.T) {
	t.Run("get existing case", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil)
		ctx := context.Background()

		created, err := uc.CreateCase(ctx, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		retrieved, err := uc.GetCase(ctx, created.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, retrieved.ID).Equal(created.ID)
		gt.Value(t, retrieved.Title).Equal(created.Title)
	})

	t.Run("get non-existent case fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil)
		ctx := context.Background()

		_, err := uc.GetCase(ctx, 999)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})
}

func TestCaseUseCase_ListCases(t *testing.T) {
	t.Run("list cases", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil)
		ctx := context.Background()

		// Create multiple cases
		_, err := uc.CreateCase(ctx, "Case 1", "Description 1", []string{}, nil)
		gt.NoError(t, err).Required()

		_, err = uc.CreateCase(ctx, "Case 2", "Description 2", []string{}, nil)
		gt.NoError(t, err).Required()

		cases, err := uc.ListCases(ctx)
		gt.NoError(t, err).Required()

		gt.Array(t, cases).Length(2)
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

		uc := usecase.NewCaseUseCase(repo, fieldSchema, nil)

		cfg := uc.GetFieldConfiguration()
		gt.Array(t, cfg.Fields).Length(1)
		gt.Value(t, cfg.Fields[0].ID).Equal("priority")
	})

	t.Run("get field configuration without schema", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil)

		cfg := uc.GetFieldConfiguration()
		gt.Array(t, cfg.Fields).Length(0)
		gt.Value(t, cfg.Labels.Case).Equal("Case")
	})
}

func TestCaseUseCase_CreateCase_SlackInvite(t *testing.T) {
	t.Run("invites creator and assignees to channel", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, riskID int64, _ string) (string, error) {
				return fmt.Sprintf("C%d", riskID), nil
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock)

		token := auth.NewToken("UCREATOR", "creator@example.com", "Creator")
		ctx := auth.ContextWithToken(context.Background(), token)

		created, err := uc.CreateCase(ctx, "Test Case", "Description", []string{"UASSIGNEE1", "UASSIGNEE2"}, nil)
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
			createChannelFn: func(_ context.Context, riskID int64, _ string) (string, error) {
				return fmt.Sprintf("C%d", riskID), nil
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock)

		token := auth.NewToken("UCREATOR", "creator@example.com", "Creator")
		ctx := auth.ContextWithToken(context.Background(), token)

		_, err := uc.CreateCase(ctx, "Test Case", "Description", []string{"UCREATOR", "UOTHER"}, nil)
		gt.NoError(t, err).Required()

		// UCREATOR should appear only once
		gt.Array(t, mock.invitedUserIDs).Length(2)
		gt.Value(t, mock.invitedUserIDs[0]).Equal("UCREATOR")
		gt.Value(t, mock.invitedUserIDs[1]).Equal("UOTHER")
	})

	t.Run("invite failure does not fail case creation", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, riskID int64, _ string) (string, error) {
				return fmt.Sprintf("C%d", riskID), nil
			},
			inviteUsersToChannelFn: func(_ context.Context, _ string, _ []string) error {
				return errors.New("slack invite error")
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock)

		token := auth.NewToken("UCREATOR", "creator@example.com", "Creator")
		ctx := auth.ContextWithToken(context.Background(), token)

		created, err := uc.CreateCase(ctx, "Test Case", "Description", []string{"UASSIGNEE"}, nil)
		gt.NoError(t, err).Required()
		gt.Value(t, created.SlackChannelID).NotEqual("")
	})

	t.Run("invites assignees without auth token", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, riskID int64, _ string) (string, error) {
				return fmt.Sprintf("C%d", riskID), nil
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock)

		// No auth token in context
		ctx := context.Background()

		_, err := uc.CreateCase(ctx, "Test Case", "Description", []string{"UASSIGNEE"}, nil)
		gt.NoError(t, err).Required()

		// Only assignees should be invited (no creator)
		gt.Array(t, mock.invitedUserIDs).Length(1)
		gt.Value(t, mock.invitedUserIDs[0]).Equal("UASSIGNEE")
	})

	t.Run("no invite when no users", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, riskID int64, _ string) (string, error) {
				return fmt.Sprintf("C%d", riskID), nil
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock)

		// No auth token, no assignees
		ctx := context.Background()

		_, err := uc.CreateCase(ctx, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		// No invite should have been called
		gt.Array(t, mock.invitedUserIDs).Length(0)
	})
}
