package repository_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runFieldValueRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	t.Run("Save creates field value", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create a case first
		c, err := repo.Case().Create(ctx, &model.Case{
			Title: "Test Case",
		})
		gt.NoError(t, err).Required()

		fv := &model.FieldValue{
			CaseID:  c.ID,
			FieldID: "category",
			Value:   []string{"data-breach"},
		}

		err = repo.CaseField().Save(ctx, fv)
		gt.NoError(t, err).Required()

		// Retrieve field values to verify
		fieldValues, err := repo.CaseField().GetByCaseID(ctx, c.ID)
		gt.NoError(t, err).Required()

		gt.Array(t, fieldValues).Length(1).Required()

		gt.Value(t, fieldValues[0].FieldID).Equal("category")
	})

	t.Run("Save updates existing field value", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create a case
		c, err := repo.Case().Create(ctx, &model.Case{
			Title: "Test Case",
		})
		gt.NoError(t, err).Required()

		// Save initial field value
		fv := &model.FieldValue{
			CaseID:  c.ID,
			FieldID: "status",
			Value:   "draft",
		}

		err = repo.CaseField().Save(ctx, fv)
		gt.NoError(t, err).Required()

		// Update the field value
		fv.Value = "published"
		err = repo.CaseField().Save(ctx, fv)
		gt.NoError(t, err).Required()

		// Verify update
		fieldValues, err := repo.CaseField().GetByCaseID(ctx, c.ID)
		gt.NoError(t, err).Required()

		gt.Array(t, fieldValues).Length(1).Required()

		gt.Value(t, fieldValues[0].Value).Equal("published")
	})

	t.Run("GetByCaseID retrieves all field values for a case", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create a case
		c, err := repo.Case().Create(ctx, &model.Case{
			Title: "Test Case",
		})
		gt.NoError(t, err).Required()

		// Save multiple field values
		fieldValues := []model.FieldValue{
			{CaseID: c.ID, FieldID: "category", Value: []string{"data-breach"}},
			{CaseID: c.ID, FieldID: "priority", Value: "high"},
			{CaseID: c.ID, FieldID: "score", Value: float64(85)},
		}

		for _, fv := range fieldValues {
			err := repo.CaseField().Save(ctx, &fv)
			gt.NoError(t, err).Required()
		}

		// Retrieve all field values
		retrieved, err := repo.CaseField().GetByCaseID(ctx, c.ID)
		gt.NoError(t, err).Required()

		gt.Array(t, retrieved).Length(3)
	})

	t.Run("GetByCaseID returns empty for case with no field values", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create a case without field values
		c, err := repo.Case().Create(ctx, &model.Case{
			Title: "Empty Case",
		})
		gt.NoError(t, err).Required()

		fieldValues, err := repo.CaseField().GetByCaseID(ctx, c.ID)
		gt.NoError(t, err).Required()

		gt.Array(t, fieldValues).Length(0)
	})

	t.Run("GetByCaseIDs retrieves field values for multiple cases", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create cases
		case1, err := repo.Case().Create(ctx, &model.Case{
			Title: "Case 1",
		})
		gt.NoError(t, err).Required()

		case2, err := repo.Case().Create(ctx, &model.Case{
			Title: "Case 2",
		})
		gt.NoError(t, err).Required()

		// Save field values for case1
		for _, fieldID := range []string{"field1", "field2"} {
			err := repo.CaseField().Save(ctx, &model.FieldValue{
				CaseID:  case1.ID,
				FieldID: fieldID,
				Value:   "value",
			})
			gt.NoError(t, err).Required()
		}

		// Save field values for case2
		for _, fieldID := range []string{"field3", "field4", "field5"} {
			err := repo.CaseField().Save(ctx, &model.FieldValue{
				CaseID:  case2.ID,
				FieldID: fieldID,
				Value:   "value",
			})
			gt.NoError(t, err).Required()
		}

		// Retrieve field values for both cases
		fieldValuesMap, err := repo.CaseField().GetByCaseIDs(ctx, []int64{case1.ID, case2.ID})
		gt.NoError(t, err).Required()

		gt.Array(t, fieldValuesMap[case1.ID]).Length(2)
		gt.Array(t, fieldValuesMap[case2.ID]).Length(3)
	})

	t.Run("DeleteByCaseID deletes all field values for a case", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create a case
		c, err := repo.Case().Create(ctx, &model.Case{
			Title: "Test Case",
		})
		gt.NoError(t, err).Required()

		// Save field values
		for _, fieldID := range []string{"field1", "field2", "field3"} {
			err := repo.CaseField().Save(ctx, &model.FieldValue{
				CaseID:  c.ID,
				FieldID: fieldID,
				Value:   "value",
			})
			gt.NoError(t, err).Required()
		}

		// Delete all field values
		err = repo.CaseField().DeleteByCaseID(ctx, c.ID)
		gt.NoError(t, err).Required()

		// Verify deletion
		fieldValues, err := repo.CaseField().GetByCaseID(ctx, c.ID)
		gt.NoError(t, err).Required()

		gt.Array(t, fieldValues).Length(0)
	})

	t.Run("Save handles different value types", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create a case
		c, err := repo.Case().Create(ctx, &model.Case{
			Title: "Test Case",
		})
		gt.NoError(t, err).Required()

		// Test different value types
		testCases := []struct {
			fieldID string
			value   any
		}{
			{"text-field", "text value"},
			{"number-field", float64(42)},
			{"select-field", "option-a"},
			{"multi-select-field", []string{"option-a", "option-b"}},
			{"date-field", time.Now()},
		}

		for _, tc := range testCases {
			err := repo.CaseField().Save(ctx, &model.FieldValue{
				CaseID:  c.ID,
				FieldID: tc.fieldID,
				Value:   tc.value,
			})
			gt.NoError(t, err).Required()
		}

		// Retrieve and verify
		fieldValues, err := repo.CaseField().GetByCaseID(ctx, c.ID)
		gt.NoError(t, err).Required()

		gt.Array(t, fieldValues).Length(len(testCases))
	})
}

func TestFieldValueRepository_Memory(t *testing.T) {
	runFieldValueRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestFieldValueRepository_Firestore(t *testing.T) {
	projectID := os.Getenv("FIRESTORE_PROJECT_ID")
	if projectID == "" {
		t.Skip("FIRESTORE_PROJECT_ID not set")
	}

	runFieldValueRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		repo, err := firestore.New(context.Background(), projectID)
		gt.NoError(t, err).Required()
		return repo
	})
}
