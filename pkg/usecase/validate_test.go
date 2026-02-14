package usecase_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func buildValidateTestSchema() *config.FieldSchema {
	return &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{
				ID:   "title-text",
				Name: "Title Text",
				Type: types.FieldTypeText,
			},
			{
				ID:   "score",
				Name: "Score",
				Type: types.FieldTypeNumber,
			},
			{
				ID:   "severity",
				Name: "Severity",
				Type: types.FieldTypeSelect,
				Options: []config.FieldOption{
					{ID: "critical", Name: "Critical"},
					{ID: "high", Name: "High"},
					{ID: "medium", Name: "Medium"},
					{ID: "low", Name: "Low"},
				},
			},
			{
				ID:   "tags",
				Name: "Tags",
				Type: types.FieldTypeMultiSelect,
				Options: []config.FieldOption{
					{ID: "network", Name: "Network"},
					{ID: "malware", Name: "Malware"},
					{ID: "phishing", Name: "Phishing"},
				},
			},
			{
				ID:   "assignee",
				Name: "Assignee",
				Type: types.FieldTypeUser,
			},
			{
				ID:   "due-date",
				Name: "Due Date",
				Type: types.FieldTypeDate,
			},
			{
				ID:   "reference",
				Name: "Reference",
				Type: types.FieldTypeURL,
			},
		},
		Labels: config.EntityLabels{Case: "Case"},
	}
}

func setupValidateTest(t *testing.T, wsID string, schema *config.FieldSchema) (*memory.Memory, *usecase.UseCases) {
	t.Helper()
	repo := memory.New()
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace:   model.Workspace{ID: wsID, Name: "Test Workspace"},
		FieldSchema: schema,
	})
	uc := usecase.New(repo, registry)
	return repo, uc
}

func TestValidateDB_NoCases(t *testing.T) {
	wsID := "ws-empty"
	_, uc := setupValidateTest(t, wsID, buildValidateTestSchema())

	result, err := uc.ValidateDB(context.Background())
	gt.NoError(t, err).Required()
	gt.Bool(t, result.HasIssues()).False()
}

func TestValidateDB_AllFieldTypesValid(t *testing.T) {
	wsID := "ws-valid"
	repo, uc := setupValidateTest(t, wsID, buildValidateTestSchema())
	ctx := context.Background()

	_, err := repo.Case().Create(ctx, wsID, &model.Case{
		Title:       "Valid Case",
		Description: "All field types are valid",
		FieldValues: map[string]model.FieldValue{
			"title-text": {FieldID: "title-text", Type: types.FieldTypeText, Value: "hello"},
			"score":      {FieldID: "score", Type: types.FieldTypeNumber, Value: float64(42)},
			"severity":   {FieldID: "severity", Type: types.FieldTypeSelect, Value: "high"},
			"tags":       {FieldID: "tags", Type: types.FieldTypeMultiSelect, Value: []string{"network", "malware"}},
			"assignee":   {FieldID: "assignee", Type: types.FieldTypeUser, Value: "U001"},
			"due-date":   {FieldID: "due-date", Type: types.FieldTypeDate, Value: "2026-02-14T00:00:00Z"},
			"reference":  {FieldID: "reference", Type: types.FieldTypeURL, Value: "https://example.com"},
		},
	})
	gt.NoError(t, err).Required()

	result, err := uc.ValidateDB(ctx)
	gt.NoError(t, err).Required()
	gt.Bool(t, result.HasIssues()).False()
}

func TestValidateDB_SelectInvalidOptionID(t *testing.T) {
	wsID := "ws-select-invalid"
	repo, uc := setupValidateTest(t, wsID, buildValidateTestSchema())
	ctx := context.Background()

	_, err := repo.Case().Create(ctx, wsID, &model.Case{
		Title: "Bad Select",
		FieldValues: map[string]model.FieldValue{
			"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "unknown-severity"},
		},
	})
	gt.NoError(t, err).Required()

	result, err := uc.ValidateDB(ctx)
	gt.NoError(t, err).Required()
	gt.Bool(t, result.HasIssues()).True()
	gt.Array(t, result.Issues).Length(1)
	gt.Value(t, result.Issues[0].FieldID).Equal("severity")
	gt.Value(t, result.Issues[0].WorkspaceID).Equal(wsID)
}

func TestValidateDB_SelectWrongType(t *testing.T) {
	wsID := "ws-select-wrong-type"
	repo, uc := setupValidateTest(t, wsID, buildValidateTestSchema())
	ctx := context.Background()

	// Value is int instead of string — detected as invalid because Value won't match any option
	_, err := repo.Case().Create(ctx, wsID, &model.Case{
		Title: "Select Wrong Type",
		FieldValues: map[string]model.FieldValue{
			"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: 42},
		},
	})
	gt.NoError(t, err).Required()

	result, err := uc.ValidateDB(ctx)
	gt.NoError(t, err).Required()
	gt.Bool(t, result.HasIssues()).True()
	gt.Array(t, result.Issues).Length(1)
	gt.Value(t, result.Issues[0].FieldID).Equal("severity")
}

func TestValidateDB_MultiSelectInvalidOptionID(t *testing.T) {
	wsID := "ws-multiselect-invalid"
	repo, uc := setupValidateTest(t, wsID, buildValidateTestSchema())
	ctx := context.Background()

	_, err := repo.Case().Create(ctx, wsID, &model.Case{
		Title: "Bad MultiSelect",
		FieldValues: map[string]model.FieldValue{
			"tags": {FieldID: "tags", Type: types.FieldTypeMultiSelect, Value: []string{"network", "nonexistent"}},
		},
	})
	gt.NoError(t, err).Required()

	result, err := uc.ValidateDB(ctx)
	gt.NoError(t, err).Required()
	gt.Bool(t, result.HasIssues()).True()
	gt.Array(t, result.Issues).Length(1)
	gt.Value(t, result.Issues[0].FieldID).Equal("tags")
}

func TestValidateDB_MultiSelectWrongType(t *testing.T) {
	wsID := "ws-multiselect-type"
	repo, uc := setupValidateTest(t, wsID, buildValidateTestSchema())
	ctx := context.Background()

	// Value is string instead of []string — invalid for multi-select
	_, err := repo.Case().Create(ctx, wsID, &model.Case{
		Title: "Bad MultiSelect Type",
		FieldValues: map[string]model.FieldValue{
			"tags": {FieldID: "tags", Type: types.FieldTypeMultiSelect, Value: "should-be-array"},
		},
	})
	gt.NoError(t, err).Required()

	result, err := uc.ValidateDB(ctx)
	gt.NoError(t, err).Required()
	gt.Bool(t, result.HasIssues()).True()
	gt.Array(t, result.Issues).Length(1)
	gt.Value(t, result.Issues[0].FieldID).Equal("tags")
}

func TestValidateDB_NonSelectFieldsNotChecked(t *testing.T) {
	wsID := "ws-non-select"
	repo, uc := setupValidateTest(t, wsID, buildValidateTestSchema())
	ctx := context.Background()

	// Even with wrong types for non-select fields, ValidateDB should not report issues
	_, err := repo.Case().Create(ctx, wsID, &model.Case{
		Title: "Non-select fields with wrong types",
		FieldValues: map[string]model.FieldValue{
			"title-text": {FieldID: "title-text", Type: types.FieldTypeText, Value: 12345},
			"score":      {FieldID: "score", Type: types.FieldTypeNumber, Value: "not-a-number"},
			"assignee":   {FieldID: "assignee", Type: types.FieldTypeUser, Value: 999},
			"due-date":   {FieldID: "due-date", Type: types.FieldTypeDate, Value: 12345},
			"reference":  {FieldID: "reference", Type: types.FieldTypeURL, Value: 12345},
		},
	})
	gt.NoError(t, err).Required()

	result, err := uc.ValidateDB(ctx)
	gt.NoError(t, err).Required()
	gt.Bool(t, result.HasIssues()).False()
}

func TestValidateDB_UnknownFieldSkipped(t *testing.T) {
	wsID := "ws-unknown-field"
	repo, uc := setupValidateTest(t, wsID, buildValidateTestSchema())
	ctx := context.Background()

	_, err := repo.Case().Create(ctx, wsID, &model.Case{
		Title: "Unknown Field",
		FieldValues: map[string]model.FieldValue{
			"unknown-field": {FieldID: "unknown-field", Type: types.FieldTypeSelect, Value: "anything"},
		},
	})
	gt.NoError(t, err).Required()

	result, err := uc.ValidateDB(ctx)
	gt.NoError(t, err).Required()
	gt.Bool(t, result.HasIssues()).False()
}

func TestValidateDB_MultipleIssuesAcrossFields(t *testing.T) {
	wsID := "ws-multi-issues"
	repo, uc := setupValidateTest(t, wsID, buildValidateTestSchema())
	ctx := context.Background()

	// Case with invalid values in both select and multi-select fields
	_, err := repo.Case().Create(ctx, wsID, &model.Case{
		Title: "Multiple Issues",
		FieldValues: map[string]model.FieldValue{
			"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "nonexistent"},
			"tags":     {FieldID: "tags", Type: types.FieldTypeMultiSelect, Value: []string{"bogus"}},
		},
	})
	gt.NoError(t, err).Required()

	result, err := uc.ValidateDB(ctx)
	gt.NoError(t, err).Required()
	gt.Bool(t, result.HasIssues()).True()
	gt.Number(t, len(result.Issues)).Equal(2)

	fieldIDs := make(map[string]bool)
	for _, issue := range result.Issues {
		fieldIDs[issue.FieldID] = true
	}
	gt.Bool(t, fieldIDs["severity"]).True()
	gt.Bool(t, fieldIDs["tags"]).True()
}

func TestValidateDB_MultipleWorkspaces(t *testing.T) {
	repo := memory.New()
	registry := model.NewWorkspaceRegistry()
	ctx := context.Background()

	wsID1 := "ws-one"
	wsID2 := "ws-two"

	schema1 := &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{
				ID: "status", Name: "Status", Type: types.FieldTypeSelect,
				Options: []config.FieldOption{
					{ID: "open", Name: "Open"},
					{ID: "closed", Name: "Closed"},
				},
			},
		},
		Labels: config.EntityLabels{Case: "Case"},
	}
	schema2 := &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{
				ID: "priority", Name: "Priority", Type: types.FieldTypeSelect,
				Options: []config.FieldOption{
					{ID: "p0", Name: "P0"},
					{ID: "p1", Name: "P1"},
				},
			},
		},
		Labels: config.EntityLabels{Case: "Case"},
	}

	registry.Register(&model.WorkspaceEntry{
		Workspace:   model.Workspace{ID: wsID1, Name: "Workspace 1"},
		FieldSchema: schema1,
	})
	registry.Register(&model.WorkspaceEntry{
		Workspace:   model.Workspace{ID: wsID2, Name: "Workspace 2"},
		FieldSchema: schema2,
	})

	uc := usecase.New(repo, registry)

	// Valid case in ws1
	_, err := repo.Case().Create(ctx, wsID1, &model.Case{
		Title: "Valid in ws1",
		FieldValues: map[string]model.FieldValue{
			"status": {FieldID: "status", Type: types.FieldTypeSelect, Value: "open"},
		},
	})
	gt.NoError(t, err).Required()

	// Invalid case in ws2
	_, err = repo.Case().Create(ctx, wsID2, &model.Case{
		Title: "Invalid in ws2",
		FieldValues: map[string]model.FieldValue{
			"priority": {FieldID: "priority", Type: types.FieldTypeSelect, Value: "p999"},
		},
	})
	gt.NoError(t, err).Required()

	result, err := uc.ValidateDB(ctx)
	gt.NoError(t, err).Required()
	gt.Bool(t, result.HasIssues()).True()
	gt.Array(t, result.Issues).Length(1)
	gt.Value(t, result.Issues[0].WorkspaceID).Equal(wsID2)
	gt.Value(t, result.Issues[0].FieldID).Equal("priority")
}

func TestValidateDB_InterfaceSliceMultiSelect(t *testing.T) {
	wsID := "ws-iface-slice"
	repo, uc := setupValidateTest(t, wsID, buildValidateTestSchema())
	ctx := context.Background()

	// []interface{} with valid strings — can happen from JSON/Firestore deserialization
	_, err := repo.Case().Create(ctx, wsID, &model.Case{
		Title: "Interface Slice MultiSelect",
		FieldValues: map[string]model.FieldValue{
			"tags": {FieldID: "tags", Type: types.FieldTypeMultiSelect, Value: []interface{}{"network", "malware"}},
		},
	})
	gt.NoError(t, err).Required()

	result, err := uc.ValidateDB(ctx)
	gt.NoError(t, err).Required()
	gt.Bool(t, result.HasIssues()).False()
}

func TestValidateDB_InterfaceSliceMultiSelectInvalid(t *testing.T) {
	wsID := "ws-iface-slice-invalid"
	repo, uc := setupValidateTest(t, wsID, buildValidateTestSchema())
	ctx := context.Background()

	// []interface{} with non-string elements
	_, err := repo.Case().Create(ctx, wsID, &model.Case{
		Title: "Interface Slice Invalid",
		FieldValues: map[string]model.FieldValue{
			"tags": {FieldID: "tags", Type: types.FieldTypeMultiSelect, Value: []interface{}{123, 456}},
		},
	})
	gt.NoError(t, err).Required()

	result, err := uc.ValidateDB(ctx)
	gt.NoError(t, err).Required()
	gt.Bool(t, result.HasIssues()).True()
	gt.Array(t, result.Issues).Length(1)
	gt.Value(t, result.Issues[0].FieldID).Equal("tags")
}

func TestValidateDB_IssueContainsSampleInfo(t *testing.T) {
	wsID := "ws-sample-info"
	repo, uc := setupValidateTest(t, wsID, buildValidateTestSchema())
	ctx := context.Background()

	created, err := repo.Case().Create(ctx, wsID, &model.Case{
		Title: "Sample Case",
		FieldValues: map[string]model.FieldValue{
			"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "deleted-option"},
		},
	})
	gt.NoError(t, err).Required()

	result, err := uc.ValidateDB(ctx)
	gt.NoError(t, err).Required()
	gt.Bool(t, result.HasIssues()).True()
	gt.Array(t, result.Issues).Length(1)

	issue := result.Issues[0]
	gt.Value(t, issue.CaseID).Equal(created.ID)
	gt.Value(t, issue.FieldID).Equal("severity")
	gt.Value(t, issue.Actual).Equal("deleted-option")
	gt.Value(t, issue.WorkspaceID).Equal(wsID)
}

func TestValidateDB_MultipleCasesOnlyCountsOne(t *testing.T) {
	wsID := "ws-multi-cases"
	repo, uc := setupValidateTest(t, wsID, buildValidateTestSchema())
	ctx := context.Background()

	// Create 3 cases with the same invalid select value
	for i := 0; i < 3; i++ {
		_, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title: "Bad Case",
			FieldValues: map[string]model.FieldValue{
				"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "removed"},
			},
		})
		gt.NoError(t, err).Required()
	}

	result, err := uc.ValidateDB(ctx)
	gt.NoError(t, err).Required()
	gt.Bool(t, result.HasIssues()).True()
	// Only one issue per field, not one per case
	gt.Array(t, result.Issues).Length(1)
	gt.Value(t, result.Issues[0].FieldID).Equal("severity")
}
