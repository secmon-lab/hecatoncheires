package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	goslack "github.com/slack-go/slack"
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
		uc := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		fieldValues := map[string]model.FieldValue{
			"priority": {FieldID: "priority", Value: "high"},
		}

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{"U001"}, fieldValues, false, "", "")
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
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := uc.CreateCase(ctx, testWorkspaceID, "", "Description", []string{}, nil, false, "", "")
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
		uc := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		fieldValues := map[string]model.FieldValue{
			"priority": {FieldID: "priority", Value: "invalid"},
		}

		_, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, fieldValues, false, "", "")
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
		uc := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(model.ErrMissingRequired)
	})
}

func TestCaseUseCase_UpdateCase_Drafts(t *testing.T) {
	t.Run("teammate can edit a public draft", func(t *testing.T) {
		// Public drafts are workspace-shared, so any teammate can iterate
		// on them via UpdateCase — including clearing the title back to
		// empty (the in-progress state is the whole point of DRAFT).
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")

		ownerCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMINE"})
		draft, err := uc.CreateDraft(ownerCtx, testWorkspaceID, "Half written", "old body", nil, nil, false)
		gt.NoError(t, err).Required()

		peerCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UPEER"})
		var patch usecase.CaseUpdate
		empty := ""
		newBody := "rewritten by teammate"
		patch.Title = &empty
		patch.Description = &newBody

		updated, err := uc.UpdateCase(peerCtx, testWorkspaceID, draft.ID, patch)
		gt.NoError(t, err).Required()
		gt.Value(t, updated.Title).Equal("")
		gt.Value(t, updated.Description).Equal("rewritten by teammate")
		gt.Value(t, updated.Status).Equal(types.CaseStatusDraft)
	})

	t.Run("non-reporter cannot edit a private draft", func(t *testing.T) {
		// Private drafts stay reporter-only — the ChannelUserIDs check
		// can't help yet (no channel), so UpdateCase falls back to the
		// reporter ID directly.
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")

		ownerCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMINE"})
		draft, err := uc.CreateDraft(ownerCtx, testWorkspaceID, "Hidden", "", nil, nil, true)
		gt.NoError(t, err).Required()

		peerCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UPEER"})
		var patch usecase.CaseUpdate
		newTitle := "stolen"
		patch.Title = &newTitle
		_, err = uc.UpdateCase(peerCtx, testWorkspaceID, draft.ID, patch)
		gt.Error(t, err).Is(usecase.TestErrAccessDenied)
	})

	t.Run("OPEN case still rejects empty title", func(t *testing.T) {
		// The empty-title gate only relaxes for drafts; once promoted to
		// OPEN, blanking the title would break Slack channel naming and
		// the listing UI.
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")

		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMINE"})
		open, err := uc.CreateCase(ctx, testWorkspaceID, "Live", "", nil, nil, false, "", "")
		gt.NoError(t, err).Required()

		var patch usecase.CaseUpdate
		empty := ""
		patch.Title = &empty
		_, err = uc.UpdateCase(ctx, testWorkspaceID, open.ID, patch)
		gt.Error(t, err)
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
		uc := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		// Create case first
		fieldValues := map[string]model.FieldValue{
			"priority": {FieldID: "priority", Value: "high"},
		}
		created, err := uc.CreateCase(ctx, testWorkspaceID, "Original Title", "Original Description", []string{"U001"}, fieldValues, false, "", "")
		gt.NoError(t, err).Required()

		// Update case
		updatedFieldValues := map[string]model.FieldValue{
			"priority": {FieldID: "priority", Value: "low"},
		}
		updatedTitle := "Updated Title"
		updatedDesc := "Updated Description"
		patch := usecase.CaseUpdate{Title: &updatedTitle, Description: &updatedDesc, Fields: updatedFieldValues}
		patch.SetAssignees([]string{"U002"})
		updated, err := uc.UpdateCase(ctx, testWorkspaceID, created.ID, patch)
		gt.NoError(t, err).Required()

		gt.Value(t, updated.Title).Equal("Updated Title")
		gt.Value(t, updated.Description).Equal("Updated Description")

		// Verify field values were updated
		retrieved, err := uc.GetCase(ctx, testWorkspaceID, updated.ID)
		gt.NoError(t, err).Required()
		gt.Number(t, len(retrieved.FieldValues)).Equal(1)
		gt.Value(t, retrieved.FieldValues["priority"].Value).Equal("low")
	})

	t.Run("partial update preserves untouched title, description, assignees and merges fields", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UORIG"})

		original := map[string]model.FieldValue{
			"stage":    {FieldID: "stage", Value: "screen"},
			"priority": {FieldID: "priority", Value: "low"},
		}
		created, err := uc.CreateCase(ctx, testWorkspaceID, "Original", "OrigDesc", []string{"U001"}, original, false, "", "")
		gt.NoError(t, err).Required()

		// Empty patch: nothing should change.
		untouched, err := uc.UpdateCase(ctx, testWorkspaceID, created.ID, usecase.CaseUpdate{})
		gt.NoError(t, err).Required()
		gt.String(t, untouched.Title).Equal("Original")
		gt.String(t, untouched.Description).Equal("OrigDesc")
		gt.Array(t, untouched.AssigneeIDs).Length(1)
		gt.Value(t, untouched.FieldValues["stage"].Value).Equal("screen")
		gt.Value(t, untouched.FieldValues["priority"].Value).Equal("low")

		// Update only one field — others (including required ones, if any)
		// must not be touched and must not be re-validated.
		updated, err := uc.UpdateCase(ctx, testWorkspaceID, created.ID, usecase.CaseUpdate{
			Fields: map[string]model.FieldValue{
				"priority": {FieldID: "priority", Value: "high"},
			},
		})
		gt.NoError(t, err).Required()
		gt.String(t, updated.Title).Equal("Original")
		gt.String(t, updated.Description).Equal("OrigDesc")
		gt.Array(t, updated.AssigneeIDs).Length(1)
		gt.Value(t, updated.FieldValues["stage"].Value).Equal("screen")
		gt.Value(t, updated.FieldValues["priority"].Value).Equal("high")

		// Update assignees only — title/description/fields preserved.
		patch := usecase.CaseUpdate{}
		patch.SetAssignees([]string{"U002", "U003"})
		assignUpd, err := uc.UpdateCase(ctx, testWorkspaceID, created.ID, patch)
		gt.NoError(t, err).Required()
		gt.String(t, assignUpd.Title).Equal("Original")
		gt.Array(t, assignUpd.AssigneeIDs).Length(2)
		gt.Value(t, assignUpd.FieldValues["stage"].Value).Equal("screen")

		// Empty title is rejected.
		empty := ""
		_, err = uc.UpdateCase(ctx, testWorkspaceID, created.ID, usecase.CaseUpdate{Title: &empty})
		gt.Value(t, err).NotNil()
	})

	t.Run("update non-existent case fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		title := "Title"
		desc := "Description"
		_, err := uc.UpdateCase(ctx, testWorkspaceID, 999, usecase.CaseUpdate{Title: &title, Description: &desc})
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})
}

func TestCaseUseCase_DeleteCase(t *testing.T) {
	t.Run("delete case with actions", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "", nil)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		// Create case
		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		// Create action for the case
		_, err = actionUC.CreateAction(ctx, testWorkspaceID, created.ID, "Test Action", "Action Description", "", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		// Delete case
		gt.NoError(t, uc.DeleteCase(ctx, testWorkspaceID, created.ID)).Required()

		// Verify case is deleted
		_, err = uc.GetCase(ctx, testWorkspaceID, created.ID)
		gt.Value(t, err).NotNil()

		// Verify actions are deleted
		actions, err := actionUC.GetActionsByCase(ctx, testWorkspaceID, created.ID, interfaces.ActionListOptions{ArchiveScope: interfaces.ActionArchiveScopeAll})
		gt.NoError(t, err).Required()
		gt.Array(t, actions).Length(0)
	})

	t.Run("delete non-existent case fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		err := uc.DeleteCase(ctx, testWorkspaceID, 999)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})
}

func TestCaseUseCase_GetCase(t *testing.T) {
	t.Run("get existing case", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		retrieved, err := uc.GetCase(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, retrieved.ID).Equal(created.ID)
		gt.Value(t, retrieved.Title).Equal(created.Title)
	})

	t.Run("get non-existent case fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := uc.GetCase(ctx, testWorkspaceID, 999)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})
}

func TestCaseUseCase_ListCases(t *testing.T) {
	t.Run("list cases", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		// Create multiple cases
		_, err := uc.CreateCase(ctx, testWorkspaceID, "Case 1", "Description 1", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		_, err = uc.CreateCase(ctx, testWorkspaceID, "Case 2", "Description 2", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		cases, err := uc.ListCases(ctx, testWorkspaceID, nil)
		gt.NoError(t, err).Required()

		gt.Array(t, cases).Length(2)
	})

	t.Run("list cases with status filter", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		// Create cases (all default to OPEN)
		case1, err := uc.CreateCase(ctx, testWorkspaceID, "Open Case 1", "desc", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		_, err = uc.CreateCase(ctx, testWorkspaceID, "Open Case 2", "desc", []string{}, nil, false, "", "")
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
	uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
	ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

	created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
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
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
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
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		_, err = uc.CloseCase(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()

		_, err = uc.CloseCase(ctx, testWorkspaceID, created.ID)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseAlreadyClosed)
	})

	t.Run("close non-existent case fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := uc.CloseCase(ctx, testWorkspaceID, 999)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})

	t.Run("close a draft case fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		created, err := repo.Case().Create(ctx, testWorkspaceID, &model.Case{
			Title:      "Some draft",
			Status:     types.CaseStatusDraft,
			ReporterID: "UTESTUSER",
		})
		gt.NoError(t, err).Required()

		_, err = uc.CloseCase(ctx, testWorkspaceID, created.ID)
		gt.Error(t, err).Is(usecase.ErrCaseIsDraft)

		// Status must remain DRAFT.
		got, err := repo.Case().Get(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.Status).Equal(types.CaseStatusDraft)
	})
}

func TestCaseUseCase_ReopenCase(t *testing.T) {
	t.Run("reopen a closed case", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
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
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		_, err = uc.ReopenCase(ctx, testWorkspaceID, created.ID)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseAlreadyOpen)
	})

	t.Run("reopen non-existent case fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := uc.ReopenCase(ctx, testWorkspaceID, 999)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})

	t.Run("reopen a draft case fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		created, err := repo.Case().Create(ctx, testWorkspaceID, &model.Case{
			Title:      "Some draft",
			Status:     types.CaseStatusDraft,
			ReporterID: "UTESTUSER",
		})
		gt.NoError(t, err).Required()

		_, err = uc.ReopenCase(ctx, testWorkspaceID, created.ID)
		gt.Error(t, err).Is(usecase.ErrCaseIsDraft)

		// Status must remain DRAFT.
		got, err := repo.Case().Get(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.Status).Equal(types.CaseStatusDraft)
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
		uc := usecase.NewCaseUseCase(repo, registry, nil, nil, "")

		cfg := uc.GetFieldConfiguration(testWorkspaceID)
		gt.Array(t, cfg.Fields).Length(1)
		gt.Value(t, cfg.Fields[0].ID).Equal("priority")
	})

	t.Run("get field configuration without schema", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")

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
		uc := usecase.NewCaseUseCase(repo, nil, mock, nil, "")

		token := auth.NewToken("UCREATOR", "creator@example.com", "Creator")
		ctx := auth.ContextWithToken(context.Background(), token)

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{"UASSIGNEE1", "UASSIGNEE2"}, nil, false, "", "")
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
		uc := usecase.NewCaseUseCase(repo, nil, mock, nil, "")

		token := auth.NewToken("UCREATOR", "creator@example.com", "Creator")
		ctx := auth.ContextWithToken(context.Background(), token)

		_, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{"UCREATOR", "UOTHER"}, nil, false, "", "")
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
		uc := usecase.NewCaseUseCase(repo, nil, mock, nil, "")

		token := auth.NewToken("UCREATOR", "creator@example.com", "Creator")
		ctx := auth.ContextWithToken(context.Background(), token)

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{"UASSIGNEE"}, nil, false, "", "")
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
		uc := usecase.NewCaseUseCase(repo, nil, mock, nil, "")

		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{"UASSIGNEE"}, nil, false, "", "")
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
		uc := usecase.NewCaseUseCase(repo, nil, mock, nil, "")

		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
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
		i18n.Init(i18n.LangEN)
		uc := usecase.NewCaseUseCase(repo, nil, mock, nil, "https://example.com")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
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
		uc := usecase.NewCaseUseCase(repo, nil, mock, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
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
		uc := usecase.NewCaseUseCase(repo, nil, mock, nil, "https://example.com")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()
		gt.Value(t, created.SlackChannelID).NotEqual("")
	})

}

func TestCaseUseCase_CreateCase_WelcomeMessages(t *testing.T) {
	newRegistry := func(messages []string) *model.WorkspaceRegistry {
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:            model.Workspace{ID: testWorkspaceID, Name: "Test"},
			SlackWelcomeMessages: messages,
		})
		return registry
	}

	t.Run("posts rendered messages in declared order", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		registry := newRegistry([]string{
			"Hello {{.Case.Title}}",
			"Reporter: <@{{.Case.ReporterID}}>",
		})
		uc := usecase.NewCaseUseCase(repo, registry, mock, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UCREATOR"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Phishing", "desc", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		gt.Array(t, mock.postedTexts).Length(2).Required()
		gt.Value(t, mock.postedTexts[0]).Equal("Hello Phishing")
		gt.Value(t, mock.postedTexts[1]).Equal("Reporter: <@UCREATOR>")
		gt.Value(t, mock.postedChannelIDs[0]).Equal(created.SlackChannelID)
		gt.Value(t, mock.postedChannelIDs[1]).Equal(created.SlackChannelID)
	})

	t.Run("template can reference custom Fields by ID and Name", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: testWorkspaceID, Name: "Test"},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{
						ID:   "severity",
						Name: "Severity",
						Type: types.FieldTypeSelect,
						Options: []config.FieldOption{
							{ID: "high", Name: "High"},
							{ID: "low", Name: "Low"},
						},
					},
				},
			},
			SlackWelcomeMessages: []string{
				"Severity: {{.Fields.severity.name}} ({{.Fields.severity.id}})",
			},
		})
		uc := usecase.NewCaseUseCase(repo, registry, mock, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UCREATOR"})

		fieldValues := map[string]model.FieldValue{
			"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "high"},
		}
		_, err := uc.CreateCase(ctx, testWorkspaceID, "Title", "desc", []string{}, fieldValues, false, "", "")
		gt.NoError(t, err).Required()

		gt.Array(t, mock.postedTexts).Length(1).Required()
		gt.Value(t, mock.postedTexts[0]).Equal("Severity: High (high)")
	})

	t.Run("URL is exposed when baseURL is set", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		registry := newRegistry([]string{
			"Detail: {{.URL}}",
		})
		i18n.Init(i18n.LangEN)
		uc := usecase.NewCaseUseCase(repo, registry, mock, nil, "https://example.com")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UCREATOR"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Title", "desc", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		expectedURL := fmt.Sprintf("https://example.com/ws/%s/cases/%d", testWorkspaceID, created.ID)
		gt.Array(t, mock.postedTexts).Length(1).Required()
		gt.Value(t, mock.postedTexts[0]).Equal("Detail: " + expectedURL)
	})

	t.Run("send failure does not abort case creation", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
			postMessageFn: func(_ context.Context, _ string, _ []goslack.Block, _ string) (string, error) {
				return "", errors.New("post failed")
			},
		}
		registry := newRegistry([]string{
			"Hello",
			"World",
		})
		uc := usecase.NewCaseUseCase(repo, registry, mock, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UCREATOR"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Title", "desc", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()
		gt.Value(t, created.SlackChannelID).NotEqual("")
		// Both messages were attempted even though the first failed.
		gt.Array(t, mock.postedTexts).Length(2)
	})

	t.Run("workspace without messages posts nothing", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UCREATOR"})

		_, err := uc.CreateCase(ctx, testWorkspaceID, "Title", "desc", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()
		gt.Array(t, mock.postedTexts).Length(0)
	})

	t.Run("private case still receives welcome messages", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		registry := newRegistry([]string{
			"private welcome",
		})
		uc := usecase.NewCaseUseCase(repo, registry, mock, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UCREATOR"})

		_, err := uc.CreateCase(ctx, testWorkspaceID, "Title", "desc", []string{}, nil, true, "", "")
		gt.NoError(t, err).Required()
		gt.Array(t, mock.postedTexts).Length(1)
		gt.Value(t, mock.postedTexts[0]).Equal("private welcome")
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
		uc := usecase.NewCaseUseCase(repo, nil, mock, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Private Case", "Secret", []string{}, nil, true, "", "")
		gt.NoError(t, err).Required()
		gt.Value(t, created.IsPrivate).Equal(true)
	})

	t.Run("get private case as member returns full case", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMEMBER"})

		// Create case directly in repo with IsPrivate and ChannelUserIDs
		caseModel := &model.Case{
			ReporterID: "U-TEST-DEFAULT",
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
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")

		// Create case as a different user
		adminCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UADMIN"})
		caseModel := &model.Case{
			ReporterID: "U-TEST-DEFAULT",
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
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMEMBER"})

		// Create public case
		_, err := uc.CreateCase(ctx, testWorkspaceID, "Public Case", "Visible", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		// Create private case with UMEMBER as member
		privCase := &model.Case{
			ReporterID: "U-TEST-DEFAULT",
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
			ReporterID: "U-TEST-DEFAULT",
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
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")

		adminCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UADMIN"})
		caseModel := &model.Case{
			ReporterID: "U-TEST-DEFAULT",
			Title:          "Private Case",
			Description:    "Secret",
			Status:         types.CaseStatusOpen,
			IsPrivate:      true,
			ChannelUserIDs: []string{"UADMIN"},
		}
		created, err := repo.Case().Create(adminCtx, testWorkspaceID, caseModel)
		gt.NoError(t, err).Required()

		nonMemberCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "USTRANGER"})
		hackTitle := "Hacked"
		hackDesc := "Hacked desc"
		hackPatch := usecase.CaseUpdate{Title: &hackTitle, Description: &hackDesc}
		hackPatch.SetAssignees([]string{})
		_, err = uc.UpdateCase(nonMemberCtx, testWorkspaceID, created.ID, hackPatch)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.TestErrAccessDenied)
	})

	t.Run("delete private case as non-member returns access denied", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")

		adminCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UADMIN"})
		caseModel := &model.Case{
			ReporterID: "U-TEST-DEFAULT",
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
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")

		adminCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UADMIN"})
		caseModel := &model.Case{
			ReporterID: "U-TEST-DEFAULT",
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
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UANYONE"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Public Case", "Visible", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		retrieved, err := uc.GetCase(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, retrieved.AccessDenied).Equal(false)
		gt.Value(t, retrieved.Title).Equal("Public Case")
	})

	t.Run("backward compatibility: existing case with nil ChannelUserIDs is public", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UANYONE"})

		// Create a case that simulates existing data (IsPrivate=false, ChannelUserIDs=nil)
		caseModel := &model.Case{
			ReporterID: "U-TEST-DEFAULT",
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
		uc := usecase.NewCaseUseCase(repo, nil, mock, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		// Seed SlackUser cache so filterHumanUsers can identify real users
		err := repo.SlackUser().SaveMany(ctx, []*model.SlackUser{
			{ID: "U001", Name: "user1", RealName: "User One"},
			{ID: "U002", Name: "user2", RealName: "User Two"},
			{ID: "U003", Name: "user3", RealName: "User Three"},
		})
		gt.NoError(t, err).Required()

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Desc", []string{}, nil, false, "", "")
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
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		// Create case without slack service (no channel)
		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Desc", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		_, err = uc.SyncCaseChannelUsers(ctx, testWorkspaceID, created.ID)
		gt.Value(t, err).NotNil()
	})

	t.Run("sync fails for non-existent case", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		_, err := uc.SyncCaseChannelUsers(ctx, testWorkspaceID, 999)
		gt.Value(t, err).NotNil()
	})
}

func TestCaseUseCase_CreateCase_AutoInvite(t *testing.T) {
	t.Run("auto-invite users from workspace config", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:        model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			SlackInviteUsers: []string{"UAUTO1", "UAUTO2"},
		})
		uc := usecase.NewCaseUseCase(repo, registry, mock, nil, "")

		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UCREATOR"})
		_, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		// Creator + auto-invite users
		gt.Array(t, mock.invitedUserIDs).Length(3)
		gt.Value(t, mock.invitedUserIDs[0]).Equal("UCREATOR")
		gt.Value(t, mock.invitedUserIDs[1]).Equal("UAUTO1")
		gt.Value(t, mock.invitedUserIDs[2]).Equal("UAUTO2")
	})

	t.Run("auto-invite deduplicates with creator and assignees", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:        model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			SlackInviteUsers: []string{"UCREATOR", "UASSIGNEE", "UAUTO1"},
		})
		uc := usecase.NewCaseUseCase(repo, registry, mock, nil, "")

		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UCREATOR"})
		_, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{"UASSIGNEE"}, nil, false, "", "")
		gt.NoError(t, err).Required()

		// UCREATOR and UASSIGNEE should appear only once each
		gt.Array(t, mock.invitedUserIDs).Length(3)
		gt.Value(t, mock.invitedUserIDs[0]).Equal("UCREATOR")
		gt.Value(t, mock.invitedUserIDs[1]).Equal("UASSIGNEE")
		gt.Value(t, mock.invitedUserIDs[2]).Equal("UAUTO1")
	})

	t.Run("auto-invite resolves group by ID", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
			getUserGroupMembersFn: func(_ context.Context, groupID string) ([]string, error) {
				if groupID == "S0001" {
					return []string{"UGROUP1", "UGROUP2"}, nil
				}
				return nil, nil
			},
		}

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:         model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			SlackInviteGroups: []string{"S0001"},
		})
		uc := usecase.NewCaseUseCase(repo, registry, mock, nil, "")

		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UCREATOR"})
		_, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		// Creator + group members
		gt.Array(t, mock.invitedUserIDs).Length(3)
		gt.Value(t, mock.invitedUserIDs[0]).Equal("UCREATOR")
		gt.Value(t, mock.invitedUserIDs[1]).Equal("UGROUP1")
		gt.Value(t, mock.invitedUserIDs[2]).Equal("UGROUP2")
	})

	t.Run("auto-invite resolves group by handle name with @ prefix", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
			listUserGroupsFn: func(_ context.Context) ([]slack.UserGroup, error) {
				return []slack.UserGroup{
					{ID: "S1234", Handle: "security-team", Name: "Security Team"},
					{ID: "S5678", Handle: "dev-team", Name: "Dev Team"},
				}, nil
			},
			getUserGroupMembersFn: func(_ context.Context, groupID string) ([]string, error) {
				if groupID == "S1234" {
					return []string{"USEC1", "USEC2"}, nil
				}
				return nil, nil
			},
		}

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:         model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			SlackInviteGroups: []string{"@security-team"},
		})
		uc := usecase.NewCaseUseCase(repo, registry, mock, nil, "")

		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UCREATOR"})
		_, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		// Creator + resolved group members
		gt.Array(t, mock.invitedUserIDs).Length(3)
		gt.Value(t, mock.invitedUserIDs[0]).Equal("UCREATOR")
		gt.Value(t, mock.invitedUserIDs[1]).Equal("USEC1")
		gt.Value(t, mock.invitedUserIDs[2]).Equal("USEC2")
	})

	t.Run("group resolution failure does not block case creation", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
			getUserGroupMembersFn: func(_ context.Context, _ string) ([]string, error) {
				return nil, errors.New("group API error")
			},
		}

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:         model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			SlackInviteGroups: []string{"S0001"},
		})
		uc := usecase.NewCaseUseCase(repo, registry, mock, nil, "")

		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UCREATOR"})
		created, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()
		gt.Value(t, created.SlackChannelID).NotEqual("")

		// Only creator should be invited (group members failed)
		gt.Array(t, mock.invitedUserIDs).Length(1)
		gt.Value(t, mock.invitedUserIDs[0]).Equal("UCREATOR")
	})

	t.Run("group ID without @ prefix does not call ListUserGroups", func(t *testing.T) {
		repo := memory.New()
		listUserGroupsCalled := false
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
			listUserGroupsFn: func(_ context.Context) ([]slack.UserGroup, error) {
				listUserGroupsCalled = true
				return nil, nil
			},
			getUserGroupMembersFn: func(_ context.Context, groupID string) ([]string, error) {
				return []string{"UMEMBER"}, nil
			},
		}

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:         model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			SlackInviteGroups: []string{"S0001"},
		})
		uc := usecase.NewCaseUseCase(repo, registry, mock, nil, "")

		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UCREATOR"})
		_, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		gt.Bool(t, listUserGroupsCalled).False()
	})

	t.Run("empty auto-invite config does not affect behavior", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
		})
		uc := usecase.NewCaseUseCase(repo, registry, mock, nil, "")

		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UCREATOR"})
		_, err := uc.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{"UASSIGNEE"}, nil, false, "", "")
		gt.NoError(t, err).Required()

		gt.Array(t, mock.invitedUserIDs).Length(2)
		gt.Value(t, mock.invitedUserIDs[0]).Equal("UCREATOR")
		gt.Value(t, mock.invitedUserIDs[1]).Equal("UASSIGNEE")
	})
}

func TestCaseUseCase_ReporterID(t *testing.T) {
	t.Run("create case sets reporter from auth token", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UREPORTER"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Reporter Test", "desc", []string{"UASSIGNEE"}, nil, false, "", "")
		gt.NoError(t, err).Required()
		gt.String(t, created.ReporterID).Equal("UREPORTER")
	})

	t.Run("create case without auth token is rejected", func(t *testing.T) {
		// Before the persistence-boundary validation was added, the
		// usecase silently persisted an unattributable case when the
		// caller had no auth.Token in context (Slack interactivity
		// handler forgot to inject, NoAuthn mode misconfigured, etc).
		// Now Case.Validate refuses the write — callers must arrange
		// for an auth-context Token to exist before reaching CreateCase.
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := context.Background()

		_, err := uc.CreateCase(ctx, testWorkspaceID, "No Reporter", "desc", []string{}, nil, false, "", "")
		gt.Error(t, err).Is(model.ErrCaseMissingReporter)
	})

	t.Run("update case preserves reporter", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UREPORTER"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Reporter Preserved", "desc", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()
		gt.String(t, created.ReporterID).Equal("UREPORTER")

		// Update with a different user context
		ctxOther := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UOTHER"})
		ut := "Updated Title"
		ud := "new desc"
		repPatch := usecase.CaseUpdate{Title: &ut, Description: &ud}
		repPatch.SetAssignees([]string{"UOTHER"})
		updated, err := uc.UpdateCase(ctxOther, testWorkspaceID, created.ID, repPatch)
		gt.NoError(t, err).Required()
		gt.String(t, updated.ReporterID).Equal("UREPORTER") // Reporter should NOT change
	})

	t.Run("reporter is persisted and retrievable", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UREPORTER"})

		created, err := uc.CreateCase(ctx, testWorkspaceID, "Persisted Reporter", "desc", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		retrieved, err := uc.GetCase(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()
		gt.String(t, retrieved.ReporterID).Equal("UREPORTER")
	})
}

// mockAdminService is a mock implementation of slack.AdminService for testing
type mockAdminService struct {
	connectFn          func(ctx context.Context, channelID string, targetTeamIDs []string) error
	connectedChannelID string
	connectedTeamIDs   []string
}

func (m *mockAdminService) ConnectChannelToWorkspace(ctx context.Context, channelID string, targetTeamIDs []string) error {
	m.connectedChannelID = channelID
	m.connectedTeamIDs = targetTeamIDs
	if m.connectFn != nil {
		return m.connectFn(ctx, channelID, targetTeamIDs)
	}
	return nil
}

func TestCaseUseCase_CreateCase_CrossWorkspaceConnect(t *testing.T) {
	t.Run("connects channel to source workspace when admin service available", func(t *testing.T) {
		repo := memory.New()
		slackMock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		adminMock := &mockAdminService{}

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:   model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			SlackTeamID: "TCONFIG",
		})
		uc := usecase.NewCaseUseCase(repo, registry, slackMock, adminMock, "")

		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UCREATOR"})
		created, err := uc.CreateCase(ctx, testWorkspaceID, "Cross WS Case", "Description", []string{}, nil, false, "TSOURCE", "")
		gt.NoError(t, err).Required()
		gt.Value(t, created.SlackChannelID).NotEqual("")

		// Verify ConnectChannelToWorkspace was called with both team IDs
		gt.String(t, adminMock.connectedChannelID).Equal(created.SlackChannelID)
		gt.Array(t, adminMock.connectedTeamIDs).Length(2)
		gt.Value(t, adminMock.connectedTeamIDs[0]).Equal("TCONFIG")
		gt.Value(t, adminMock.connectedTeamIDs[1]).Equal("TSOURCE")
	})

	t.Run("skips connect when source team ID matches configured team ID", func(t *testing.T) {
		repo := memory.New()
		slackMock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		adminMock := &mockAdminService{}

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:   model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			SlackTeamID: "TSAME",
		})
		uc := usecase.NewCaseUseCase(repo, registry, slackMock, adminMock, "")

		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UCREATOR"})
		_, err := uc.CreateCase(ctx, testWorkspaceID, "Same WS Case", "Description", []string{}, nil, false, "TSAME", "")
		gt.NoError(t, err).Required()

		// ConnectChannelToWorkspace should NOT have been called
		gt.String(t, adminMock.connectedChannelID).Equal("")
	})

	t.Run("skips connect when source team ID is empty", func(t *testing.T) {
		repo := memory.New()
		slackMock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		adminMock := &mockAdminService{}

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:   model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			SlackTeamID: "TCONFIG",
		})
		uc := usecase.NewCaseUseCase(repo, registry, slackMock, adminMock, "")

		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UCREATOR"})
		_, err := uc.CreateCase(ctx, testWorkspaceID, "No Source Team", "Description", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		gt.String(t, adminMock.connectedChannelID).Equal("")
	})

	t.Run("connect failure does not fail case creation", func(t *testing.T) {
		repo := memory.New()
		slackMock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		adminMock := &mockAdminService{
			connectFn: func(_ context.Context, _ string, _ []string) error {
				return errors.New("admin.conversations.setTeams failed")
			},
		}

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:   model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			SlackTeamID: "TCONFIG",
		})
		uc := usecase.NewCaseUseCase(repo, registry, slackMock, adminMock, "")

		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UCREATOR"})
		created, err := uc.CreateCase(ctx, testWorkspaceID, "Connect Fail Case", "Description", []string{}, nil, false, "TSOURCE", "")
		gt.NoError(t, err).Required()
		gt.Value(t, created.SlackChannelID).NotEqual("")
	})

	t.Run("skips connect when admin service is nil", func(t *testing.T) {
		repo := memory.New()
		slackMock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:   model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			SlackTeamID: "TCONFIG",
		})
		// No admin service (nil)
		uc := usecase.NewCaseUseCase(repo, registry, slackMock, nil, "")

		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UCREATOR"})
		created, err := uc.CreateCase(ctx, testWorkspaceID, "No Admin Svc", "Description", []string{}, nil, false, "TSOURCE", "")
		gt.NoError(t, err).Required()
		gt.Value(t, created.SlackChannelID).NotEqual("")
	})
}

func TestCaseUseCase_CreateCase_DuplicateRequestKey(t *testing.T) {
	t.Run("duplicate request key returns existing case without creating new one", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
		})
		uc := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		requestKey := "test-idempotency-key-1"

		// First call creates the case
		created, err := uc.CreateCase(ctx, testWorkspaceID, "First Case", "Description", []string{}, nil, false, "", requestKey)
		gt.NoError(t, err).Required()
		gt.Value(t, created.Title).Equal("First Case")
		gt.String(t, created.RequestKey).Equal(requestKey)

		// Second call with same key returns existing case
		duplicate, err := uc.CreateCase(ctx, testWorkspaceID, "Different Title", "Different Desc", []string{}, nil, false, "", requestKey)
		gt.NoError(t, err).Required()
		gt.Value(t, duplicate.ID).Equal(created.ID)
		gt.Value(t, duplicate.Title).Equal("First Case")

		// Verify only one case exists
		cases, err := uc.ListCases(ctx, testWorkspaceID, nil)
		gt.NoError(t, err).Required()
		gt.Array(t, cases).Length(1)
	})

	t.Run("empty request key skips dedup check", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
		})
		uc := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		// Create two cases with empty key — both should succeed
		case1, err := uc.CreateCase(ctx, testWorkspaceID, "Case 1", "Desc", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		case2, err := uc.CreateCase(ctx, testWorkspaceID, "Case 2", "Desc", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		gt.Value(t, case1.ID).NotEqual(case2.ID)
	})

	t.Run("different request keys create separate cases", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
		})
		uc := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UTESTUSER"})

		case1, err := uc.CreateCase(ctx, testWorkspaceID, "Case A", "Desc", []string{}, nil, false, "", "key-a")
		gt.NoError(t, err).Required()

		case2, err := uc.CreateCase(ctx, testWorkspaceID, "Case B", "Desc", []string{}, nil, false, "", "key-b")
		gt.NoError(t, err).Required()

		gt.Value(t, case1.ID).NotEqual(case2.ID)
		gt.Value(t, case1.Title).Equal("Case A")
		gt.Value(t, case2.Title).Equal("Case B")
	})
}

func TestCaseUseCase_CreateDraft(t *testing.T) {
	t.Run("create draft without side effects", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UAUTHOR"})

		got, err := uc.CreateDraft(ctx, testWorkspaceID, "Half-written", "Desc", []string{"UASSIGN"}, nil, true)
		gt.NoError(t, err).Required()
		gt.Value(t, got.Status).Equal(types.CaseStatusDraft)
		gt.Value(t, got.Title).Equal("Half-written")
		gt.Value(t, got.Description).Equal("Desc")
		gt.Value(t, got.ReporterID).Equal("UAUTHOR")
		gt.Value(t, got.IsPrivate).Equal(true)
		gt.Number(t, len(got.AssigneeIDs)).Equal(1)
		gt.Value(t, got.AssigneeIDs[0]).Equal("UASSIGN")
		gt.Value(t, got.SlackChannelID).Equal("")

		// Slack channel creation MUST NOT have fired.
		gt.String(t, mock.invitedChannelID).Equal("")
		gt.Array(t, mock.invitedUserIDs).Length(0)

		// Persisted state matches what was returned.
		stored, err := repo.Case().Get(context.Background(), testWorkspaceID, got.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, stored.Status).Equal(types.CaseStatusDraft)
		gt.Value(t, stored.Title).Equal("Half-written")
		gt.Value(t, stored.ReporterID).Equal("UAUTHOR")
	})

	t.Run("create draft with empty title is allowed", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UAUTHOR"})

		got, err := uc.CreateDraft(ctx, testWorkspaceID, "", "Just a body", nil, nil, false)
		gt.NoError(t, err).Required()
		gt.Value(t, got.Status).Equal(types.CaseStatusDraft)
		gt.Value(t, got.Title).Equal("")
	})

	t.Run("create draft skips required-field check", func(t *testing.T) {
		// Draft is by definition half-finished, so required fields that the
		// regular CreateCase path would reject must NOT block Save as Draft.
		// SubmitDraft is what enforces the full schema later.
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: testWorkspaceID, Name: "Test"},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{
						ID:       "category",
						Name:     "Category",
						Type:     types.FieldTypeSelect,
						Required: true,
						Options: []config.FieldOption{
							{ID: "a", Name: "A"},
							{ID: "b", Name: "B"},
						},
					},
				},
			},
		})
		uc := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UAUTHOR"})

		// No "category" supplied — required field missing.
		got, err := uc.CreateDraft(ctx, testWorkspaceID, "Half", "", nil, nil, false)
		gt.NoError(t, err).Required()
		gt.Value(t, got.Status).Equal(types.CaseStatusDraft)
		gt.Number(t, len(got.FieldValues)).Equal(0)
	})
}

func TestCaseUseCase_ListDrafts(t *testing.T) {
	t.Run("returns every draft in the workspace regardless of reporter", func(t *testing.T) {
		// Drafts are surfaced workspace-wide so anyone on the team can pick
		// up an in-progress entry — reporter scoping was intentionally
		// removed once the feature graduated past the original Slack-only
		// design.
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")

		mineCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMINE"})
		mineA, err := uc.CreateDraft(mineCtx, testWorkspaceID, "Mine A", "", nil, nil, false)
		gt.NoError(t, err).Required()
		mineB, err := uc.CreateDraft(mineCtx, testWorkspaceID, "Mine B", "", nil, nil, false)
		gt.NoError(t, err).Required()

		otherCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UOTHER"})
		theirs, err := uc.CreateDraft(otherCtx, testWorkspaceID, "Theirs", "", nil, nil, false)
		gt.NoError(t, err).Required()

		// Open case by UMINE — must NOT appear in drafts.
		_, err = uc.CreateCase(mineCtx, testWorkspaceID, "Submitted", "", nil, nil, false, "", "")
		gt.NoError(t, err).Required()

		got, err := uc.ListDrafts(mineCtx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Number(t, len(got)).Equal(3)
		ids := map[int64]bool{}
		for _, c := range got {
			ids[c.ID] = true
			gt.Value(t, c.Status).Equal(types.CaseStatusDraft)
		}
		gt.Bool(t, ids[mineA.ID]).True()
		gt.Bool(t, ids[mineB.ID]).True()
		gt.Bool(t, ids[theirs.ID]).True()
	})

	t.Run("private drafts stay reporter-only", func(t *testing.T) {
		// A draft has no Slack channel yet, so the usual ChannelUserIDs-based
		// access control locks everyone out of private drafts. To keep the
		// reporter from losing access to their own private entry while still
		// hiding it from teammates, ListDrafts filters private drafts by
		// reporter ID directly.
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")

		ownerCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UOWNER"})
		privateDraft, err := uc.CreateDraft(ownerCtx, testWorkspaceID, "Secret", "", nil, nil, true)
		gt.NoError(t, err).Required()
		publicDraft, err := uc.CreateDraft(ownerCtx, testWorkspaceID, "Public", "", nil, nil, false)
		gt.NoError(t, err).Required()

		// The owner sees both their public and their private draft.
		mine, err := uc.ListDrafts(ownerCtx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Number(t, len(mine)).Equal(2)

		// A different teammate sees the public one but NOT the private one.
		otherCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UPEER"})
		theirs, err := uc.ListDrafts(otherCtx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Number(t, len(theirs)).Equal(1)
		gt.Value(t, theirs[0].ID).Equal(publicDraft.ID)

		// Sanity: the private draft still exists in the repo — it's just
		// hidden from the peer at the usecase layer.
		stored, err := repo.Case().Get(context.Background(), testWorkspaceID, privateDraft.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, stored.IsPrivate).Equal(true)
	})

	t.Run("no auth token surfaces public drafts only", func(t *testing.T) {
		// Bot / system contexts still get the workspace's public drafts —
		// hiding everything would mask legitimate background work — but
		// private drafts are filtered out (no reporter to match).
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")

		ownerCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UOWNER"})
		_, err := uc.CreateDraft(ownerCtx, testWorkspaceID, "Public draft", "", nil, nil, false)
		gt.NoError(t, err).Required()
		_, err = uc.CreateDraft(ownerCtx, testWorkspaceID, "Private draft", "", nil, nil, true)
		gt.NoError(t, err).Required()

		got, err := uc.ListDrafts(context.Background(), testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Number(t, len(got)).Equal(1)
		gt.Value(t, got[0].Title).Equal("Public draft")
	})
}

func TestCaseUseCase_GetDraft(t *testing.T) {
	t.Run("reporter retrieves their own draft", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMINE"})

		created, err := uc.CreateDraft(ctx, testWorkspaceID, "Mine", "Body", nil, nil, false)
		gt.NoError(t, err).Required()

		got, err := uc.GetDraft(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.ID).Equal(created.ID)
		gt.Value(t, got.Title).Equal("Mine")
	})

	t.Run("non-reporter can read a public draft", func(t *testing.T) {
		// Public drafts surface across the workspace so any teammate can
		// review an in-progress entry. The reporter check is only enforced
		// at the mutation layer (SubmitDraft / DiscardDraft).
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")

		ownerCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMINE"})
		created, err := uc.CreateDraft(ownerCtx, testWorkspaceID, "Shared", "", nil, nil, false)
		gt.NoError(t, err).Required()

		otherCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UOTHER"})
		got, err := uc.GetDraft(otherCtx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.ID).Equal(created.ID)
		gt.Value(t, got.Title).Equal("Shared")
	})

	t.Run("non-reporter cannot read a private draft", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")

		ownerCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMINE"})
		created, err := uc.CreateDraft(ownerCtx, testWorkspaceID, "Hidden", "", nil, nil, true)
		gt.NoError(t, err).Required()

		otherCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UOTHER"})
		_, err = uc.GetDraft(otherCtx, testWorkspaceID, created.ID)
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})

	t.Run("non-draft case returns ErrCaseNotDraft", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMINE"})

		open, err := uc.CreateCase(ctx, testWorkspaceID, "Open Case", "", nil, nil, false, "", "")
		gt.NoError(t, err).Required()

		_, err = uc.GetDraft(ctx, testWorkspaceID, open.ID)
		gt.Error(t, err).Is(usecase.ErrCaseNotDraft)
	})

	t.Run("unknown id returns ErrCaseNotFound", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMINE"})

		_, err := uc.GetDraft(ctx, testWorkspaceID, 999)
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})
}

func TestCaseUseCase_SubmitDraft(t *testing.T) {
	t.Run("promotes draft and runs activation side effects", func(t *testing.T) {
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UAUTHOR"})

		draft, err := uc.CreateDraft(ctx, testWorkspaceID, "Will Submit", "Body", []string{"UASSIGN"}, nil, false)
		gt.NoError(t, err).Required()
		gt.Value(t, draft.Status).Equal(types.CaseStatusDraft)

		submitted, err := uc.SubmitDraft(ctx, testWorkspaceID, draft.ID, nil)
		gt.NoError(t, err).Required()
		gt.Value(t, submitted.Status).Equal(types.CaseStatusOpen)
		gt.Value(t, submitted.ID).Equal(draft.ID)
		gt.Value(t, submitted.SlackChannelID).Equal(fmt.Sprintf("C%d", draft.ID))

		// Reporter + assignee both invited during activation.
		gt.Array(t, mock.invitedUserIDs).Length(2)
		gt.Value(t, mock.invitedUserIDs[0]).Equal("UAUTHOR")
		gt.Value(t, mock.invitedUserIDs[1]).Equal("UASSIGN")

		// Persisted state matches.
		stored, err := repo.Case().Get(context.Background(), testWorkspaceID, draft.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, stored.Status).Equal(types.CaseStatusOpen)
		gt.Value(t, stored.SlackChannelID).Equal(fmt.Sprintf("C%d", draft.ID))
	})

	t.Run("submit-with-patch applies edits and still invites reporter", func(t *testing.T) {
		// Architecturally important: "save final edits and submit" is one
		// business operation, so it runs through a single SubmitDraft call.
		// We patch the title, swap assignees, and verify the resulting
		// channel invite list still includes the original reporter (UAUTHOR)
		// even though the patch did not name them.
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UAUTHOR"})

		draft, err := uc.CreateDraft(ctx, testWorkspaceID, "Half-written", "old body", []string{"UOLD"}, nil, false)
		gt.NoError(t, err).Required()

		newTitle := "Final title"
		newBody := "polished body"
		var patch usecase.CaseUpdate
		patch.Title = &newTitle
		patch.Description = &newBody
		patch.SetAssignees([]string{"UNEW"})

		submitted, err := uc.SubmitDraft(ctx, testWorkspaceID, draft.ID, &patch)
		gt.NoError(t, err).Required()
		gt.Value(t, submitted.Status).Equal(types.CaseStatusOpen)
		gt.Value(t, submitted.Title).Equal("Final title")
		gt.Value(t, submitted.Description).Equal("polished body")
		gt.Value(t, submitted.ReporterID).Equal("UAUTHOR")

		// Channel invites: reporter (UAUTHOR) + new assignee (UNEW).
		// The pre-edit assignee (UOLD) has been replaced.
		gt.Array(t, mock.invitedUserIDs).Length(2)
		gt.Value(t, mock.invitedUserIDs[0]).Equal("UAUTHOR")
		gt.Value(t, mock.invitedUserIDs[1]).Equal("UNEW")

		// The persisted case reflects the patch — i.e. the same value the
		// channel was activated against, not a stale draft snapshot.
		stored, err := repo.Case().Get(context.Background(), testWorkspaceID, draft.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, stored.Title).Equal("Final title")
		gt.Value(t, stored.Description).Equal("polished body")
		gt.Number(t, len(stored.AssigneeIDs)).Equal(1)
		gt.Value(t, stored.AssigneeIDs[0]).Equal("UNEW")
	})

	t.Run("requires title to submit", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UAUTHOR"})

		draft, err := uc.CreateDraft(ctx, testWorkspaceID, "", "Body only", nil, nil, false)
		gt.NoError(t, err).Required()

		_, err = uc.SubmitDraft(ctx, testWorkspaceID, draft.ID, nil)
		gt.Error(t, err)

		stored, err := repo.Case().Get(context.Background(), testWorkspaceID, draft.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, stored.Status).Equal(types.CaseStatusDraft)
	})

	t.Run("submitter (actor) is invited even when different from reporter", func(t *testing.T) {
		// When Bob promotes Alice's draft, both end up in the case channel:
		// Alice via ReporterID and Bob via the auth-context user that
		// triggered activateCase. Without this guarantee, the user who
		// hits Submit on the Web side would find themselves locked out of
		// the freshly-OPEN case channel.
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock, nil, "")

		ownerCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UALICE"})
		draft, err := uc.CreateDraft(ownerCtx, testWorkspaceID, "Public draft", "", nil, nil, false)
		gt.NoError(t, err).Required()

		submitterCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UBOB"})
		submitted, err := uc.SubmitDraft(submitterCtx, testWorkspaceID, draft.ID, nil)
		gt.NoError(t, err).Required()
		gt.Value(t, submitted.Status).Equal(types.CaseStatusOpen)
		// Reporter is preserved — promotion doesn't reassign ownership.
		gt.Value(t, submitted.ReporterID).Equal("UALICE")

		// Channel invite list contains both the reporter (UALICE) and the
		// submitter (UBOB). Ordering: reporter, actor, assignees.
		gt.Array(t, mock.invitedUserIDs).Length(2)
		gt.Value(t, mock.invitedUserIDs[0]).Equal("UALICE")
		gt.Value(t, mock.invitedUserIDs[1]).Equal("UBOB")
	})

	t.Run("any teammate can submit a public draft", func(t *testing.T) {
		// Drafts are team-shared — once a teammate visits a public draft
		// they can promote it to OPEN. The original reporter stays
		// recorded on the case, and channel activation invites them as
		// usual. Private drafts stay reporter-only because non-reporters
		// can't even see them.
		repo := memory.New()
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, caseID int64, _ string, _ string) (string, error) {
				return fmt.Sprintf("C%d", caseID), nil
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock, nil, "")

		ownerCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMINE"})
		draft, err := uc.CreateDraft(ownerCtx, testWorkspaceID, "Shared", "", nil, nil, false)
		gt.NoError(t, err).Required()

		otherCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UOTHER"})
		submitted, err := uc.SubmitDraft(otherCtx, testWorkspaceID, draft.ID, nil)
		gt.NoError(t, err).Required()
		gt.Value(t, submitted.Status).Equal(types.CaseStatusOpen)
		gt.Value(t, submitted.ReporterID).Equal("UMINE")
	})

	t.Run("non-reporter cannot submit a private draft", func(t *testing.T) {
		// Private drafts are invisible to non-reporters, so SubmitDraft
		// reaches them as ErrCaseNotFound from GetDraft (same code path
		// as "draft does not exist").
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")

		ownerCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMINE"})
		draft, err := uc.CreateDraft(ownerCtx, testWorkspaceID, "Secret", "", nil, nil, true)
		gt.NoError(t, err).Required()

		otherCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UOTHER"})
		_, err = uc.SubmitDraft(otherCtx, testWorkspaceID, draft.ID, nil)
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})

	t.Run("rejects submit when required field is still missing", func(t *testing.T) {
		// CreateDraft skipped the required check, but SubmitDraft must enforce
		// it before flipping the case to OPEN — otherwise an incomplete draft
		// would slip past the workspace schema.
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: testWorkspaceID, Name: "Test"},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{
						ID:       "category",
						Name:     "Category",
						Type:     types.FieldTypeSelect,
						Required: true,
						Options: []config.FieldOption{
							{ID: "a", Name: "A"},
							{ID: "b", Name: "B"},
						},
					},
				},
			},
		})
		uc := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UAUTHOR"})

		draft, err := uc.CreateDraft(ctx, testWorkspaceID, "Has Title", "", nil, nil, false)
		gt.NoError(t, err).Required()

		_, err = uc.SubmitDraft(ctx, testWorkspaceID, draft.ID, nil)
		gt.Error(t, err).Is(usecase.ErrMissingRequiredOnSubmit)
		// The error message must name the missing field so the UI can
		// point the user at it directly (rather than the legacy opaque
		// "required field is missing").
		gt.String(t, err.Error()).Contains("Category")

		// The draft survives the failed submit so the user can fill in the
		// missing field and retry.
		stored, err := repo.Case().Get(context.Background(), testWorkspaceID, draft.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, stored.Status).Equal(types.CaseStatusDraft)
		gt.Value(t, stored.Title).Equal("Has Title")
	})

	t.Run("rolls back to DRAFT when activation fails", func(t *testing.T) {
		repo := memory.New()
		// CreateChannel returns error → activation fails. SubmitDraft's
		// rollback policy preserves the user's work: the case row stays,
		// status reverts to DRAFT, and the user can retry from the same
		// entry rather than starting over.
		mock := &mockSlackService{
			createChannelFn: func(_ context.Context, _ int64, _ string, _ string) (string, error) {
				return "", errors.New("channel creation rejected")
			},
		}
		uc := usecase.NewCaseUseCase(repo, nil, mock, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UAUTHOR"})

		draft, err := uc.CreateDraft(ctx, testWorkspaceID, "Will Fail", "Body", nil, nil, false)
		gt.NoError(t, err).Required()

		_, err = uc.SubmitDraft(ctx, testWorkspaceID, draft.ID, nil)
		gt.Error(t, err)

		// Persisted case row survives; status is flipped back to DRAFT.
		stored, getErr := repo.Case().Get(context.Background(), testWorkspaceID, draft.ID)
		gt.NoError(t, getErr).Required()
		gt.Value(t, stored.Status).Equal(types.CaseStatusDraft)
		gt.Value(t, stored.Title).Equal("Will Fail")
		gt.Value(t, stored.Description).Equal("Body")
	})

	t.Run("already-submitted case cannot be submitted again", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UAUTHOR"})

		open, err := uc.CreateCase(ctx, testWorkspaceID, "Already Open", "", nil, nil, false, "", "")
		gt.NoError(t, err).Required()

		_, err = uc.SubmitDraft(ctx, testWorkspaceID, open.ID, nil)
		gt.Error(t, err).Is(usecase.ErrCaseNotDraft)
	})
}

func TestCaseUseCase_DiscardDraft(t *testing.T) {
	t.Run("reporter discards own draft", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMINE"})

		draft, err := uc.CreateDraft(ctx, testWorkspaceID, "Discard Me", "", nil, nil, false)
		gt.NoError(t, err).Required()

		err = uc.DiscardDraft(ctx, testWorkspaceID, draft.ID)
		gt.NoError(t, err).Required()

		_, getErr := repo.Case().Get(context.Background(), testWorkspaceID, draft.ID)
		gt.Error(t, getErr)
	})

	t.Run("any teammate can discard a public draft", func(t *testing.T) {
		// Public drafts are workspace-shared so any teammate can clean one
		// up. Private drafts remain reporter-only because GetDraft hides
		// them from everyone else (covered in the next case).
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")

		ownerCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMINE"})
		draft, err := uc.CreateDraft(ownerCtx, testWorkspaceID, "Shared", "", nil, nil, false)
		gt.NoError(t, err).Required()

		otherCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UOTHER"})
		gt.NoError(t, uc.DiscardDraft(otherCtx, testWorkspaceID, draft.ID)).Required()

		_, getErr := repo.Case().Get(context.Background(), testWorkspaceID, draft.ID)
		gt.Error(t, getErr)
	})

	t.Run("non-reporter cannot discard a private draft", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")

		ownerCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMINE"})
		draft, err := uc.CreateDraft(ownerCtx, testWorkspaceID, "Hidden", "", nil, nil, true)
		gt.NoError(t, err).Required()

		otherCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UOTHER"})
		err = uc.DiscardDraft(otherCtx, testWorkspaceID, draft.ID)
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)

		// Draft must still exist after the failed discard.
		stored, getErr := repo.Case().Get(context.Background(), testWorkspaceID, draft.ID)
		gt.NoError(t, getErr).Required()
		gt.Value(t, stored.Status).Equal(types.CaseStatusDraft)
	})

	t.Run("non-draft case is rejected", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMINE"})

		open, err := uc.CreateCase(ctx, testWorkspaceID, "Open", "", nil, nil, false, "", "")
		gt.NoError(t, err).Required()

		err = uc.DiscardDraft(ctx, testWorkspaceID, open.ID)
		gt.Error(t, err).Is(usecase.ErrCaseNotDraft)
	})
}

func TestCaseUseCase_GetCase_DraftVisibility(t *testing.T) {
	t.Run("public draft is readable by anyone in the workspace", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")

		ownerCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMINE"})
		draft, err := uc.CreateDraft(ownerCtx, testWorkspaceID, "Public draft", "", nil, nil, false)
		gt.NoError(t, err).Required()

		// Reporter sees their draft.
		got, err := uc.GetCase(ownerCtx, testWorkspaceID, draft.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.ID).Equal(draft.ID)

		// Stranger also sees the public draft (workspace-wide visibility).
		otherCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UOTHER"})
		got, err = uc.GetCase(otherCtx, testWorkspaceID, draft.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.ID).Equal(draft.ID)
	})

	t.Run("private draft stays hidden from non-reporters", func(t *testing.T) {
		// Private drafts have no Slack channel yet, so the normal
		// ChannelUserIDs-based check would lock out the reporter too.
		// GetCase falls back to a reporter check for private drafts in
		// DRAFT status.
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")

		ownerCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMINE"})
		draft, err := uc.CreateDraft(ownerCtx, testWorkspaceID, "Secret", "", nil, nil, true)
		gt.NoError(t, err).Required()

		got, err := uc.GetCase(ownerCtx, testWorkspaceID, draft.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.ID).Equal(draft.ID)

		otherCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UOTHER"})
		_, err = uc.GetCase(otherCtx, testWorkspaceID, draft.ID)
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})
}
