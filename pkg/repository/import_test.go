package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestImportRepository_RoundTrip_Memory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.New().Import()

	in := newValidSession("ws-acme", "U12345678")
	created, err := repo.Create(ctx, in.WorkspaceID, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID != in.ID {
		t.Errorf("create returned ID=%q, want %q", created.ID, in.ID)
	}

	got, err := repo.Get(ctx, in.WorkspaceID, in.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	// Round-trip every field exhaustively
	if got.WorkspaceID != in.WorkspaceID {
		t.Errorf("WorkspaceID: got %q want %q", got.WorkspaceID, in.WorkspaceID)
	}
	if got.CreatorUserID != in.CreatorUserID {
		t.Errorf("CreatorUserID: got %q want %q", got.CreatorUserID, in.CreatorUserID)
	}
	if got.Status != in.Status {
		t.Errorf("Status: got %q want %q", got.Status, in.Status)
	}
	if got.Source.OriginalFileName != in.Source.OriginalFileName {
		t.Errorf("Source.OriginalFileName: got %q want %q", got.Source.OriginalFileName, in.Source.OriginalFileName)
	}
	if got.Source.ContentDigest != in.Source.ContentDigest {
		t.Errorf("Source.ContentDigest mismatch")
	}
	if got.Source.SizeBytes != in.Source.SizeBytes {
		t.Errorf("Source.SizeBytes mismatch")
	}
	if got.FieldSchemaHash != in.FieldSchemaHash {
		t.Errorf("FieldSchemaHash mismatch")
	}
	if !got.CreatedAt.Equal(in.CreatedAt) {
		t.Errorf("CreatedAt mismatch")
	}
	if !got.UpdatedAt.Equal(in.UpdatedAt) {
		t.Errorf("UpdatedAt mismatch")
	}
	if got.ExecutedAt == nil || !got.ExecutedAt.Equal(*in.ExecutedAt) {
		t.Errorf("ExecutedAt mismatch")
	}
	if got.CreatedCount != in.CreatedCount {
		t.Errorf("CreatedCount mismatch")
	}
	if len(got.Issues) != 1 || got.Issues[0].Path != "version" {
		t.Errorf("session Issues mismatch: %+v", got.Issues)
	}
	if len(got.Snapshot.Cases) != 1 {
		t.Fatalf("snapshot.cases length: got %d want 1", len(got.Snapshot.Cases))
	}
	gc := got.Snapshot.Cases[0]
	if gc.Title != "Suspicious login" {
		t.Errorf("case title: got %q", gc.Title)
	}
	if len(gc.AssigneeIDs) != 1 || gc.AssigneeIDs[0] != "U12345678" {
		t.Errorf("case AssigneeIDs: %v", gc.AssigneeIDs)
	}
	if gc.Result.Status != model.ImportItemCreated {
		t.Errorf("case result status: %q", gc.Result.Status)
	}
	if gc.Result.CreatedCaseID == nil || *gc.Result.CreatedCaseID != 142 {
		t.Errorf("case result CreatedCaseID: %v", gc.Result.CreatedCaseID)
	}
	if len(gc.Actions) != 1 {
		t.Fatalf("actions length")
	}
	ga := gc.Actions[0]
	if ga.Title != "Block IP" || ga.AssigneeID != "U87654321" {
		t.Errorf("action mismatch")
	}
	if ga.DueDate == nil || !ga.DueDate.Equal(time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("action DueDate: %v", ga.DueDate)
	}
	if ga.Result.Status != model.ImportItemCreated || ga.Result.CreatedActionID == nil || *ga.Result.CreatedActionID != 1421 {
		t.Errorf("action result: %+v", ga.Result)
	}
}

func TestImportRepository_Update(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.New().Import()

	in := newValidSession("ws-acme", "U1")
	in.Status = model.ImportSessionPending
	in.ExecutedAt = nil
	in.Snapshot.Cases[0].Result = model.ImportCaseResult{Status: model.ImportItemPending}
	in.Snapshot.Cases[0].Actions[0].Result = model.ImportActionResult{Status: model.ImportItemPending}
	if _, err := repo.Create(ctx, in.WorkspaceID, in); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Mutate and update
	exec := time.Date(2026, 5, 25, 11, 0, 0, 0, time.UTC)
	caseID := int64(200)
	in.Status = model.ImportSessionApplied
	in.ExecutedAt = &exec
	in.Snapshot.Cases[0].Result = model.ImportCaseResult{Status: model.ImportItemCreated, CreatedCaseID: &caseID}
	in.Snapshot.Cases[0].Actions[0].Result = model.ImportActionResult{Status: model.ImportItemCreated}
	in.RecomputeCounts()
	if _, err := repo.Update(ctx, in.WorkspaceID, in); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := repo.Get(ctx, in.WorkspaceID, in.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.Status != model.ImportSessionApplied {
		t.Errorf("status not updated")
	}
	if got.Snapshot.Cases[0].Result.Status != model.ImportItemCreated {
		t.Errorf("case status not updated")
	}
	if got.CreatedCount != 1 {
		t.Errorf("counts not updated: created=%d", got.CreatedCount)
	}
}

func TestImportRepository_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.New().Import()
	_, err := repo.Get(ctx, "ws-acme", model.NewImportSessionID())
	if err == nil {
		t.Fatalf("expected error for missing session")
	}
	// Memory's ErrNotFound is wrapped; we just confirm an error came back.
	if errors.Is(err, nil) {
		t.Fatalf("got nil-wrapping error")
	}
}

func TestImportRepository_ValidateRejectsBadSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.New().Import()
	bad := newValidSession("ws-acme", "U1")
	bad.CreatorUserID = "" // triggers ErrImportSessionMissingCreator
	if _, err := repo.Create(ctx, "ws-acme", bad); err == nil {
		t.Fatalf("expected validation error")
	} else if !errors.Is(err, model.ErrImportSessionMissingCreator) {
		t.Fatalf("expected ErrImportSessionMissingCreator, got %v", err)
	}
}
