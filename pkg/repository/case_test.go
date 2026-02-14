package repository_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runCaseRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	const wsID = "test-ws"

	t.Run("Create creates case with auto-increment ID", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		case1 := &model.Case{
			Title:       "SQL Injection Risk",
			Description: "Database vulnerable to SQL injection",
			AssigneeIDs: []string{"U123", "U456"},
		}

		created1, err := repo.Case().Create(ctx, wsID, case1)
		gt.NoError(t, err).Required()

		gt.Value(t, created1.ID).NotEqual(int64(0))
		gt.Value(t, created1.Title).Equal(case1.Title)
		gt.Value(t, created1.Description).Equal(case1.Description)
		gt.Array(t, created1.AssigneeIDs).Length(len(case1.AssigneeIDs))
		gt.Bool(t, created1.CreatedAt.IsZero()).False()
		gt.Bool(t, created1.UpdatedAt.IsZero()).False()

		// Create second case to test auto-increment
		case2 := &model.Case{
			Title:       "XSS Risk",
			Description: "Cross-site scripting vulnerability",
		}

		created2, err := repo.Case().Create(ctx, wsID, case2)
		gt.NoError(t, err).Required()

		gt.Value(t, created2.ID).NotEqual(created1.ID)
	})

	t.Run("Get retrieves existing case", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title:       "CSRF Risk",
			Description: "Cross-site request forgery",
			AssigneeIDs: []string{"U789"},
		})
		gt.NoError(t, err).Required()

		retrieved, err := repo.Case().Get(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, retrieved.ID).Equal(created.ID)
		gt.Value(t, retrieved.Title).Equal(created.Title)
		gt.Value(t, retrieved.Description).Equal(created.Description)
		gt.Array(t, retrieved.AssigneeIDs).Length(len(created.AssigneeIDs))
		gt.Bool(t, retrieved.CreatedAt.Equal(created.CreatedAt)).True()
		gt.Bool(t, retrieved.UpdatedAt.Equal(created.UpdatedAt)).True()
	})

	t.Run("Get returns error for non-existent case", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		_, err := repo.Case().Get(ctx, wsID, time.Now().UnixNano())
		gt.Value(t, err).NotNil()
	})

	t.Run("Update updates existing case", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title:       "Original Title",
			Description: "Original Description",
		})
		gt.NoError(t, err).Required()

		// Update the case
		created.Title = "Updated Title"
		created.Description = "Updated Description"
		created.AssigneeIDs = []string{"U111", "U222"}

		updated, err := repo.Case().Update(ctx, wsID, created)
		gt.NoError(t, err).Required()

		gt.Value(t, updated.ID).Equal(created.ID)
		gt.Value(t, updated.Title).Equal("Updated Title")
		gt.Value(t, updated.Description).Equal("Updated Description")
		gt.Array(t, updated.AssigneeIDs).Length(2)
		gt.Bool(t, updated.UpdatedAt.Before(created.UpdatedAt) || updated.UpdatedAt.Equal(created.UpdatedAt)).False()
	})

	t.Run("Delete deletes existing case", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title: "To be deleted",
		})
		gt.NoError(t, err).Required()

		err = repo.Case().Delete(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()

		// Verify it's deleted
		_, err = repo.Case().Get(ctx, wsID, created.ID)
		gt.Value(t, err).NotNil()
	})

	t.Run("Create and Get with FieldValues", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		fieldValues := map[string]model.FieldValue{
			"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "critical"},
			"score":    {FieldID: "score", Type: types.FieldTypeNumber, Value: 4.5},
			"tags":     {FieldID: "tags", Type: types.FieldTypeMultiSelect, Value: []string{"data-breach", "compliance"}},
			"url":      {FieldID: "url", Type: types.FieldTypeURL, Value: "https://example.com"},
		}

		created, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title:       "Case with fields",
			Description: "Testing field values",
			FieldValues: fieldValues,
		})
		gt.NoError(t, err).Required()

		retrieved, err := repo.Case().Get(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()

		gt.Number(t, len(retrieved.FieldValues)).Equal(4)

		gt.Value(t, retrieved.FieldValues["severity"].FieldID).Equal("severity")
		gt.Value(t, retrieved.FieldValues["severity"].Type).Equal(types.FieldTypeSelect)
		gt.Value(t, retrieved.FieldValues["severity"].Value).Equal("critical")

		gt.Value(t, retrieved.FieldValues["score"].FieldID).Equal("score")
		gt.Value(t, retrieved.FieldValues["score"].Type).Equal(types.FieldTypeNumber)
		gt.Value(t, retrieved.FieldValues["score"].Value).Equal(4.5)

		gt.Value(t, retrieved.FieldValues["tags"].FieldID).Equal("tags")
		gt.Value(t, retrieved.FieldValues["tags"].Type).Equal(types.FieldTypeMultiSelect)

		gt.Value(t, retrieved.FieldValues["url"].FieldID).Equal("url")
		gt.Value(t, retrieved.FieldValues["url"].Type).Equal(types.FieldTypeURL)
		gt.Value(t, retrieved.FieldValues["url"].Value).Equal("https://example.com")
	})

	t.Run("Create with nil FieldValues", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title:       "Case without fields",
			FieldValues: nil,
		})
		gt.NoError(t, err).Required()

		retrieved, err := repo.Case().Get(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()

		// nil or empty map is acceptable
		gt.Number(t, len(retrieved.FieldValues)).Equal(0)
	})

	t.Run("Create with empty FieldValues", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title:       "Case with empty fields",
			FieldValues: map[string]model.FieldValue{},
		})
		gt.NoError(t, err).Required()

		retrieved, err := repo.Case().Get(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()

		gt.Number(t, len(retrieved.FieldValues)).Equal(0)
	})

	t.Run("Update preserves and modifies FieldValues", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title: "Case to update fields",
			FieldValues: map[string]model.FieldValue{
				"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "low"},
			},
		})
		gt.NoError(t, err).Required()

		// Update with new field values
		created.FieldValues = map[string]model.FieldValue{
			"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "high"},
			"notes":    {FieldID: "notes", Type: types.FieldTypeText, Value: "urgent"},
		}

		updated, err := repo.Case().Update(ctx, wsID, created)
		gt.NoError(t, err).Required()

		gt.Number(t, len(updated.FieldValues)).Equal(2)
		gt.Value(t, updated.FieldValues["severity"].Value).Equal("high")
		gt.Value(t, updated.FieldValues["notes"].Value).Equal("urgent")

		// Verify via Get as well
		retrieved, err := repo.Case().Get(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()
		gt.Number(t, len(retrieved.FieldValues)).Equal(2)
		gt.Value(t, retrieved.FieldValues["severity"].Value).Equal("high")
	})

	t.Run("FieldValues deep copy isolation", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		tags := []string{"tag1", "tag2"}
		created, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title: "Deep copy test",
			FieldValues: map[string]model.FieldValue{
				"tags": {FieldID: "tags", Type: types.FieldTypeMultiSelect, Value: tags},
			},
		})
		gt.NoError(t, err).Required()

		// Mutate the original slice
		tags[0] = "mutated"

		// Retrieve and verify the stored value is not affected
		retrieved, err := repo.Case().Get(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()

		storedTags, ok := retrieved.FieldValues["tags"].Value.([]string)
		gt.Bool(t, ok).True()
		gt.Value(t, storedTags[0]).Equal("tag1")

		// Also verify that mutating the retrieved value doesn't affect the store
		storedTags[0] = "also-mutated"

		retrieved2, err := repo.Case().Get(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()

		storedTags2, ok := retrieved2.FieldValues["tags"].Value.([]string)
		gt.Bool(t, ok).True()
		gt.Value(t, storedTags2[0]).Equal("tag1")
	})

	t.Run("Delete removes case with FieldValues", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title: "Case to delete",
			FieldValues: map[string]model.FieldValue{
				"priority": {FieldID: "priority", Type: types.FieldTypeSelect, Value: "high"},
			},
		})
		gt.NoError(t, err).Required()

		err = repo.Case().Delete(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()

		_, err = repo.Case().Get(ctx, wsID, created.ID)
		gt.Value(t, err).NotNil()
	})

	t.Run("GetBySlackChannelID returns matching case", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title:          "Case with channel",
			SlackChannelID: "C-TEST-CHANNEL",
		})
		gt.NoError(t, err).Required()

		// Update to set SlackChannelID (Create may not persist it directly)
		created.SlackChannelID = "C-TEST-CHANNEL"
		_, err = repo.Case().Update(ctx, wsID, created)
		gt.NoError(t, err).Required()

		found, err := repo.Case().GetBySlackChannelID(ctx, wsID, "C-TEST-CHANNEL")
		gt.NoError(t, err).Required()
		gt.Value(t, found).NotNil()
		gt.Value(t, found.ID).Equal(created.ID)
		gt.Value(t, found.Title).Equal("Case with channel")
		gt.Value(t, found.SlackChannelID).Equal("C-TEST-CHANNEL")
	})

	t.Run("GetBySlackChannelID returns nil for non-existent channel", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		found, err := repo.Case().GetBySlackChannelID(ctx, wsID, "C-NONEXISTENT")
		gt.NoError(t, err)
		gt.Value(t, found).Nil()
	})

	t.Run("GetBySlackChannelID returns nil for non-existent workspace", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		found, err := repo.Case().GetBySlackChannelID(ctx, "nonexistent-ws", "C-WHATEVER")
		gt.NoError(t, err)
		gt.Value(t, found).Nil()
	})

	t.Run("CountFieldValues counts total and valid select values", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create 3 cases with select field: 2 valid, 1 invalid
		for _, severity := range []string{"high", "medium", "invalid-opt"} {
			_, err := repo.Case().Create(ctx, wsID, &model.Case{
				Title: "Case " + severity,
				FieldValues: map[string]model.FieldValue{
					"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: severity},
				},
			})
			gt.NoError(t, err).Required()
		}

		total, valid, err := repo.Case().CountFieldValues(
			ctx, wsID, "severity", types.FieldTypeSelect, []string{"high", "medium", "low"},
		)
		gt.NoError(t, err).Required()
		gt.Value(t, total).Equal(int64(3))
		gt.Value(t, valid).Equal(int64(2))
	})

	t.Run("CountFieldValues returns zero for empty workspace", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		total, valid, err := repo.Case().CountFieldValues(
			ctx, wsID, "severity", types.FieldTypeSelect, []string{"high"},
		)
		gt.NoError(t, err).Required()
		gt.Value(t, total).Equal(int64(0))
		gt.Value(t, valid).Equal(int64(0))
	})

	t.Run("CountFieldValues ignores different field types", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create a case with text field (not select)
		_, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title: "Text case",
			FieldValues: map[string]model.FieldValue{
				"severity": {FieldID: "severity", Type: types.FieldTypeText, Value: "high"},
			},
		})
		gt.NoError(t, err).Required()

		total, valid, err := repo.Case().CountFieldValues(
			ctx, wsID, "severity", types.FieldTypeSelect, []string{"high"},
		)
		gt.NoError(t, err).Required()
		gt.Value(t, total).Equal(int64(0))
		gt.Value(t, valid).Equal(int64(0))
	})

	t.Run("CountFieldValues counts multi-select values", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		_, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title: "Valid tags",
			FieldValues: map[string]model.FieldValue{
				"tags": {FieldID: "tags", Type: types.FieldTypeMultiSelect, Value: []string{"network", "malware"}},
			},
		})
		gt.NoError(t, err).Required()

		_, err = repo.Case().Create(ctx, wsID, &model.Case{
			Title: "Invalid tags",
			FieldValues: map[string]model.FieldValue{
				"tags": {FieldID: "tags", Type: types.FieldTypeMultiSelect, Value: []string{"network", "bogus"}},
			},
		})
		gt.NoError(t, err).Required()

		total, valid, err := repo.Case().CountFieldValues(
			ctx, wsID, "tags", types.FieldTypeMultiSelect, []string{"network", "malware", "phishing"},
		)
		gt.NoError(t, err).Required()
		gt.Value(t, total).Equal(int64(2))
		gt.Value(t, valid).Equal(int64(1))
	})

	t.Run("FindCaseWithInvalidFieldValue returns invalid case", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		_, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title: "Valid case",
			FieldValues: map[string]model.FieldValue{
				"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "high"},
			},
		})
		gt.NoError(t, err).Required()

		_, err = repo.Case().Create(ctx, wsID, &model.Case{
			Title: "Invalid case",
			FieldValues: map[string]model.FieldValue{
				"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "deleted-option"},
			},
		})
		gt.NoError(t, err).Required()

		found, err := repo.Case().FindCaseWithInvalidFieldValue(
			ctx, wsID, "severity", types.FieldTypeSelect, []string{"high", "medium", "low"},
		)
		gt.NoError(t, err).Required()
		gt.Value(t, found).NotNil()
		gt.Value(t, found.Title).Equal("Invalid case")

		fv, ok := found.FieldValues["severity"]
		gt.Bool(t, ok).True()
		gt.Value(t, fv.Value).Equal("deleted-option")
	})

	t.Run("FindCaseWithInvalidFieldValue returns nil when all valid", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		_, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title: "Valid case",
			FieldValues: map[string]model.FieldValue{
				"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "high"},
			},
		})
		gt.NoError(t, err).Required()

		found, err := repo.Case().FindCaseWithInvalidFieldValue(
			ctx, wsID, "severity", types.FieldTypeSelect, []string{"high", "medium", "low"},
		)
		gt.NoError(t, err).Required()
		gt.Value(t, found).Nil()
	})

	t.Run("FindCaseWithInvalidFieldValue detects invalid multi-select", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		_, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title: "Bad multi-select",
			FieldValues: map[string]model.FieldValue{
				"tags": {FieldID: "tags", Type: types.FieldTypeMultiSelect, Value: []string{"network", "removed-tag"}},
			},
		})
		gt.NoError(t, err).Required()

		found, err := repo.Case().FindCaseWithInvalidFieldValue(
			ctx, wsID, "tags", types.FieldTypeMultiSelect, []string{"network", "malware", "phishing"},
		)
		gt.NoError(t, err).Required()
		gt.Value(t, found).NotNil()
		gt.Value(t, found.Title).Equal("Bad multi-select")
	})

	t.Run("FindCaseWithInvalidFieldValue returns nil for empty workspace", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		found, err := repo.Case().FindCaseWithInvalidFieldValue(
			ctx, wsID, "severity", types.FieldTypeSelect, []string{"high"},
		)
		gt.NoError(t, err).Required()
		gt.Value(t, found).Nil()
	})

	t.Run("List retrieves all cases", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create multiple cases
		for i := 0; i < 3; i++ {
			_, err := repo.Case().Create(ctx, wsID, &model.Case{
				Title:       "Case " + string(rune('A'+i)),
				Description: "Description " + string(rune('A'+i)),
			})
			gt.NoError(t, err).Required()
		}

		cases, err := repo.Case().List(ctx, wsID)
		gt.NoError(t, err).Required()

		gt.Number(t, len(cases)).GreaterOrEqual(3)
	})

	t.Run("List with status filter returns only matching cases", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create open cases
		_, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title:  "Open Case 1",
			Status: types.CaseStatusOpen,
		})
		gt.NoError(t, err).Required()

		_, err = repo.Case().Create(ctx, wsID, &model.Case{
			Title:  "Open Case 2",
			Status: types.CaseStatusOpen,
		})
		gt.NoError(t, err).Required()

		// Create closed case
		_, err = repo.Case().Create(ctx, wsID, &model.Case{
			Title:  "Closed Case 1",
			Status: types.CaseStatusClosed,
		})
		gt.NoError(t, err).Required()

		// Filter by OPEN
		openCases, err := repo.Case().List(ctx, wsID, interfaces.WithStatus(types.CaseStatusOpen))
		gt.NoError(t, err).Required()
		gt.Number(t, len(openCases)).Equal(2)
		for _, c := range openCases {
			gt.Value(t, c.Status).Equal(types.CaseStatusOpen)
		}

		// Filter by CLOSED
		closedCases, err := repo.Case().List(ctx, wsID, interfaces.WithStatus(types.CaseStatusClosed))
		gt.NoError(t, err).Required()
		gt.Number(t, len(closedCases)).Equal(1)
		gt.Value(t, closedCases[0].Status).Equal(types.CaseStatusClosed)
		gt.Value(t, closedCases[0].Title).Equal("Closed Case 1")

		// No filter returns all
		allCases, err := repo.Case().List(ctx, wsID)
		gt.NoError(t, err).Required()
		gt.Number(t, len(allCases)).Equal(3)
	})

	t.Run("Create and Get preserves status", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title:  "Status Test",
			Status: types.CaseStatusClosed,
		})
		gt.NoError(t, err).Required()

		retrieved, err := repo.Case().Get(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, retrieved.Status).Equal(types.CaseStatusClosed)
	})

	t.Run("Update preserves status change", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title:  "Status Update Test",
			Status: types.CaseStatusOpen,
		})
		gt.NoError(t, err).Required()

		created.Status = types.CaseStatusClosed
		updated, err := repo.Case().Update(ctx, wsID, created)
		gt.NoError(t, err).Required()
		gt.Value(t, updated.Status).Equal(types.CaseStatusClosed)

		retrieved, err := repo.Case().Get(ctx, wsID, updated.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, retrieved.Status).Equal(types.CaseStatusClosed)
	})
}

func TestCaseRepository_Memory(t *testing.T) {
	runCaseRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestCaseRepository_Firestore(t *testing.T) {
	projectID := os.Getenv("FIRESTORE_PROJECT_ID")
	if projectID == "" {
		t.Skip("FIRESTORE_PROJECT_ID not set")
	}

	runCaseRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		repo, err := firestore.New(context.Background(), projectID, "")
		gt.NoError(t, err).Required()
		return repo
	})
}
