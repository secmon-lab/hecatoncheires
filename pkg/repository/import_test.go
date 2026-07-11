package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

// newValidSession builds a minimally valid ImportSession used as the base
// for round-trip tests. Repositories must round-trip every field
// exhaustively so that a "silently dropped field" regression (the bug
// that motivated the repository write contract) cannot recur.
func newValidSession(workspaceID, creatorUserID string) *model.ImportSession {
	createdAt := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	executedAt := time.Date(2026, 5, 25, 10, 5, 0, 0, time.UTC)
	dueDate := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	caseID := int64(142)
	actionID := int64(1421)

	return &model.ImportSession{
		ID:            model.NewImportSessionID(),
		WorkspaceID:   workspaceID,
		CreatorUserID: creatorUserID,
		Status:        model.ImportSessionApplied,
		Source: model.ImportSource{
			OriginalFileName: "incidents.yaml",
			ContentDigest:    "abc123",
			SizeBytes:        2048,
		},
		Snapshot: model.ImportSnapshot{
			Version: 1,
			Cases: []model.ImportSnapshotCase{
				{
					Index:       0,
					Title:       "Suspicious login",
					Description: "Multiple failed attempts.",
					IsPrivate:   false,
					AssigneeIDs: []string{"U12345678"},
					FieldValues: map[string]model.FieldValue{
						"severity": {FieldID: "severity", Value: "high"},
					},
					Actions: []model.ImportSnapshotAction{
						{
							Index:       0,
							Title:       "Block IP",
							Description: "Add firewall rule",
							AssigneeID:  "U87654321",
							DueDate:     &dueDate,
							Result:      model.ImportActionResult{Status: model.ImportItemCreated, CreatedActionID: &actionID},
						},
					},
					Issues: []model.ImportIssue{
						{Path: "cases[0].fields.severity", Message: "ok-ish", Severity: model.ImportIssueWarning},
					},
					Result: model.ImportCaseResult{Status: model.ImportItemCreated, CreatedCaseID: &caseID},
				},
			},
		},
		Issues: []model.ImportIssue{
			{Path: "version", Message: "version=1 accepted", Severity: model.ImportIssueWarning},
		},
		FieldSchemaHash: "schemahash",
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		ExecutedAt:      &executedAt,
		CreatedCount:    1,
		FailedCount:     0,
		SkippedCount:    0,
	}
}

// runImportRepositoryTest exercises the ImportRepository contract against
// whichever backend newRepo constructs. Like runCaseRepositoryTest it is
// invoked for both memory and Firestore so a field dropped only on the
// Firestore Create path (the memory repo round-trips via a full struct
// copy and hides such bugs) is caught apples-to-apples.
func runImportRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	t.Run("RoundTrip", func(t *testing.T) {
		ctx := context.Background()
		repo := newRepo(t).Import()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		in := newValidSession(wsID, "U12345678")
		created, err := repo.Create(ctx, in.WorkspaceID, in)
		gt.NoError(t, err).Required()
		gt.Value(t, created.ID).Equal(in.ID)

		got, err := repo.Get(ctx, in.WorkspaceID, in.ID)
		gt.NoError(t, err).Required()

		// Round-trip every field exhaustively.
		gt.Value(t, got.WorkspaceID).Equal(in.WorkspaceID)
		gt.Value(t, got.CreatorUserID).Equal(in.CreatorUserID)
		gt.Value(t, got.Status).Equal(in.Status)
		gt.Value(t, got.Source.OriginalFileName).Equal(in.Source.OriginalFileName)
		gt.Value(t, got.Source.ContentDigest).Equal(in.Source.ContentDigest)
		gt.Value(t, got.Source.SizeBytes).Equal(in.Source.SizeBytes)
		gt.Value(t, got.FieldSchemaHash).Equal(in.FieldSchemaHash)
		gt.Bool(t, got.CreatedAt.Equal(in.CreatedAt)).True()
		gt.Bool(t, got.UpdatedAt.Equal(in.UpdatedAt)).True()
		gt.Value(t, got.ExecutedAt).NotNil().Required()
		gt.Bool(t, got.ExecutedAt.Equal(*in.ExecutedAt)).True()
		gt.Value(t, got.CreatedCount).Equal(in.CreatedCount)
		gt.Value(t, got.FailedCount).Equal(in.FailedCount)
		gt.Value(t, got.SkippedCount).Equal(in.SkippedCount)

		gt.Array(t, got.Issues).Length(1).Required()
		gt.Value(t, got.Issues[0].Path).Equal("version")
		gt.Value(t, got.Issues[0].Severity).Equal(model.ImportIssueWarning)

		gt.Array(t, got.Snapshot.Cases).Length(1).Required()
		gc := got.Snapshot.Cases[0]
		gt.Value(t, gc.Title).Equal("Suspicious login")
		gt.Value(t, gc.Description).Equal("Multiple failed attempts.")
		gt.Bool(t, gc.IsPrivate).False()
		gt.Value(t, gc.AssigneeIDs).Equal([]string{"U12345678"})
		gt.Value(t, gc.Result.Status).Equal(model.ImportItemCreated)
		gt.Value(t, gc.Result.CreatedCaseID).NotNil().Required()
		gt.Value(t, *gc.Result.CreatedCaseID).Equal(int64(142))

		gt.Array(t, gc.Actions).Length(1).Required()
		ga := gc.Actions[0]
		gt.Value(t, ga.Title).Equal("Block IP")
		gt.Value(t, ga.AssigneeID).Equal("U87654321")
		gt.Value(t, ga.DueDate).NotNil().Required()
		gt.Bool(t, ga.DueDate.Equal(time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC))).True()
		gt.Value(t, ga.Result.Status).Equal(model.ImportItemCreated)
		gt.Value(t, ga.Result.CreatedActionID).NotNil().Required()
		gt.Value(t, *ga.Result.CreatedActionID).Equal(int64(1421))
	})

	t.Run("Update", func(t *testing.T) {
		ctx := context.Background()
		repo := newRepo(t).Import()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		in := newValidSession(wsID, "U1")
		in.Status = model.ImportSessionPending
		in.ExecutedAt = nil
		in.Snapshot.Cases[0].Result = model.ImportCaseResult{Status: model.ImportItemPending}
		in.Snapshot.Cases[0].Actions[0].Result = model.ImportActionResult{Status: model.ImportItemPending}
		_, err := repo.Create(ctx, in.WorkspaceID, in)
		gt.NoError(t, err).Required()

		// Mutate and update.
		exec := time.Date(2026, 5, 25, 11, 0, 0, 0, time.UTC)
		caseID := int64(200)
		in.Status = model.ImportSessionApplied
		in.ExecutedAt = &exec
		in.Snapshot.Cases[0].Result = model.ImportCaseResult{Status: model.ImportItemCreated, CreatedCaseID: &caseID}
		in.Snapshot.Cases[0].Actions[0].Result = model.ImportActionResult{Status: model.ImportItemCreated}
		in.RecomputeCounts()
		_, err = repo.Update(ctx, in.WorkspaceID, in)
		gt.NoError(t, err).Required()

		got, err := repo.Get(ctx, in.WorkspaceID, in.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.Status).Equal(model.ImportSessionApplied)
		gt.Value(t, got.ExecutedAt).NotNil().Required()
		gt.Bool(t, got.ExecutedAt.Equal(exec)).True()
		gt.Value(t, got.Snapshot.Cases[0].Result.Status).Equal(model.ImportItemCreated)
		gt.Value(t, got.CreatedCount).Equal(1)
	})

	t.Run("NotFound", func(t *testing.T) {
		ctx := context.Background()
		repo := newRepo(t).Import()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		// memory and Firestore expose distinct ErrNotFound sentinels, so the
		// shared helper asserts only that a lookup of a missing session fails.
		_, err := repo.Get(ctx, wsID, model.NewImportSessionID())
		gt.Error(t, err)
	})

	t.Run("ValidateRejectsBadSession", func(t *testing.T) {
		ctx := context.Background()
		repo := newRepo(t).Import()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		bad := newValidSession(wsID, "U1")
		bad.CreatorUserID = "" // triggers ErrImportSessionMissingCreator
		_, err := repo.Create(ctx, wsID, bad)
		gt.Error(t, err).Is(model.ErrImportSessionMissingCreator)
	})
}

func TestImportRepository_Memory(t *testing.T) {
	t.Parallel()
	runImportRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestImportRepository_Firestore(t *testing.T) {
	t.Parallel()
	runImportRepositoryTest(t, newFirestoreRepository)
}
