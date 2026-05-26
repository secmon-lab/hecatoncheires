package model_test

import (
	"errors"
	"testing"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// validImportSession returns a minimally valid ImportSession used as the
// base for each Validate scenario.
func validImportSession() *model.ImportSession {
	executedAt := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	caseID := int64(142)
	actionID := int64(1421)
	return &model.ImportSession{
		ID:            model.NewImportSessionID(),
		WorkspaceID:   "ws-acme",
		CreatorUserID: "U12345678",
		Status:        model.ImportSessionApplied,
		Source: model.ImportSource{
			OriginalFileName: "incidents.yaml",
			ContentDigest:    "abcdef0123456789",
			SizeBytes:        128,
		},
		Snapshot: model.ImportSnapshot{
			Version: 1,
			Cases: []model.ImportSnapshotCase{
				{
					Index:       0,
					Title:       "Suspicious login",
					Description: "",
					IsPrivate:   false,
					AssigneeIDs: []string{"U87654321"},
					FieldValues: map[string]model.FieldValue{},
					Actions: []model.ImportSnapshotAction{
						{
							Index:  0,
							Title:  "Block IP",
							Result: model.ImportActionResult{Status: model.ImportItemCreated, CreatedActionID: &actionID},
						},
					},
					Issues: nil,
					Result: model.ImportCaseResult{Status: model.ImportItemCreated, CreatedCaseID: &caseID},
				},
			},
		},
		Issues:          nil,
		FieldSchemaHash: "deadbeef",
		CreatedAt:       time.Date(2026, 5, 25, 11, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC),
		ExecutedAt:      &executedAt,
		CreatedCount:    1,
		FailedCount:     0,
		SkippedCount:    0,
	}
}

func TestImportSessionValidate_OK(t *testing.T) {
	s := validImportSession()
	if err := s.Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestImportSessionValidate_NilReceiver(t *testing.T) {
	var s *model.ImportSession
	if err := s.Validate(); err == nil {
		t.Fatalf("expected error for nil receiver")
	}
}

func TestImportSessionValidate_MissingID(t *testing.T) {
	s := validImportSession()
	s.ID = ""
	err := s.Validate()
	if !errors.Is(err, model.ErrImportSessionMissingID) {
		t.Fatalf("expected ErrImportSessionMissingID, got %v", err)
	}
}

func TestImportSessionValidate_MissingWorkspace(t *testing.T) {
	s := validImportSession()
	s.WorkspaceID = ""
	err := s.Validate()
	if !errors.Is(err, model.ErrImportSessionMissingWorkspace) {
		t.Fatalf("expected ErrImportSessionMissingWorkspace, got %v", err)
	}
}

func TestImportSessionValidate_MissingCreator(t *testing.T) {
	s := validImportSession()
	s.CreatorUserID = ""
	err := s.Validate()
	if !errors.Is(err, model.ErrImportSessionMissingCreator) {
		t.Fatalf("expected ErrImportSessionMissingCreator, got %v", err)
	}
}

func TestImportSessionValidate_InvalidStatus(t *testing.T) {
	s := validImportSession()
	s.Status = model.ImportSessionStatus("discarded") // intentionally unknown
	err := s.Validate()
	if !errors.Is(err, model.ErrImportSessionInvalidStatus) {
		t.Fatalf("expected ErrImportSessionInvalidStatus, got %v", err)
	}
}

func TestImportSessionValidate_InvalidSessionIssueSeverity(t *testing.T) {
	s := validImportSession()
	s.Issues = []model.ImportIssue{
		{Path: "version", Message: "noop", Severity: model.ImportIssueSeverity("notice")},
	}
	err := s.Validate()
	if !errors.Is(err, model.ErrImportSessionInvalidIssueSeverity) {
		t.Fatalf("expected ErrImportSessionInvalidIssueSeverity, got %v", err)
	}
}

func TestImportSessionValidate_InvalidCaseResultStatus(t *testing.T) {
	s := validImportSession()
	s.Snapshot.Cases[0].Result = model.ImportCaseResult{Status: model.ImportItemResultStatus("unknown")}
	err := s.Validate()
	if !errors.Is(err, model.ErrImportSessionInvalidItemStatus) {
		t.Fatalf("expected ErrImportSessionInvalidItemStatus, got %v", err)
	}
}

func TestImportSessionValidate_InvalidActionResultStatus(t *testing.T) {
	s := validImportSession()
	s.Snapshot.Cases[0].Actions[0].Result = model.ImportActionResult{Status: model.ImportItemResultStatus("done")}
	err := s.Validate()
	if !errors.Is(err, model.ErrImportSessionInvalidItemStatus) {
		t.Fatalf("expected ErrImportSessionInvalidItemStatus, got %v", err)
	}
}

func TestImportSessionValidate_InvalidCaseIssueSeverity(t *testing.T) {
	s := validImportSession()
	s.Snapshot.Cases[0].Issues = []model.ImportIssue{
		{Path: "cases[0].title", Message: "x", Severity: model.ImportIssueSeverity("info")},
	}
	err := s.Validate()
	if !errors.Is(err, model.ErrImportSessionInvalidIssueSeverity) {
		t.Fatalf("expected ErrImportSessionInvalidIssueSeverity, got %v", err)
	}
}

func TestImportSessionValidate_InvalidActionIssueSeverity(t *testing.T) {
	s := validImportSession()
	s.Snapshot.Cases[0].Actions[0].Issues = []model.ImportIssue{
		{Path: "cases[0].actions[0].title", Message: "x", Severity: model.ImportIssueSeverity("trace")},
	}
	err := s.Validate()
	if !errors.Is(err, model.ErrImportSessionInvalidIssueSeverity) {
		t.Fatalf("expected ErrImportSessionInvalidIssueSeverity, got %v", err)
	}
}

func TestImportSession_HasErrorIssues(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(s *model.ImportSession)
		wantErr bool
	}{
		{
			name:    "no issues",
			setup:   func(s *model.ImportSession) {},
			wantErr: false,
		},
		{
			name: "session-level error",
			setup: func(s *model.ImportSession) {
				s.Issues = []model.ImportIssue{{Path: "version", Message: "bad", Severity: model.ImportIssueError}}
			},
			wantErr: true,
		},
		{
			name: "session-level warning only",
			setup: func(s *model.ImportSession) {
				s.Issues = []model.ImportIssue{{Path: "version", Message: "soft", Severity: model.ImportIssueWarning}}
			},
			wantErr: false,
		},
		{
			name: "case-level error",
			setup: func(s *model.ImportSession) {
				s.Snapshot.Cases[0].Issues = []model.ImportIssue{
					{Path: "cases[0].title", Message: "required", Severity: model.ImportIssueError},
				}
			},
			wantErr: true,
		},
		{
			name: "action-level error",
			setup: func(s *model.ImportSession) {
				s.Snapshot.Cases[0].Actions[0].Issues = []model.ImportIssue{
					{Path: "cases[0].actions[0].title", Message: "required", Severity: model.ImportIssueError},
				}
			},
			wantErr: true,
		},
		{
			name: "case-level warning only",
			setup: func(s *model.ImportSession) {
				s.Snapshot.Cases[0].Issues = []model.ImportIssue{
					{Path: "cases[0].assigneeIDs[0]", Message: "unknown user", Severity: model.ImportIssueWarning},
				}
			},
			wantErr: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := validImportSession()
			tc.setup(s)
			got := s.HasErrorIssues()
			if got != tc.wantErr {
				t.Fatalf("HasErrorIssues = %v, want %v", got, tc.wantErr)
			}
			if s.Valid() == tc.wantErr {
				t.Fatalf("Valid = %v, want %v", s.Valid(), !tc.wantErr)
			}
		})
	}
}

func TestImportSession_RecomputeCounts(t *testing.T) {
	caseID := int64(100)
	s := &model.ImportSession{
		ID:            model.NewImportSessionID(),
		WorkspaceID:   "ws-acme",
		CreatorUserID: "U1",
		Status:        model.ImportSessionFailed,
		Snapshot: model.ImportSnapshot{
			Version: 1,
			Cases: []model.ImportSnapshotCase{
				// Case 1: created, all actions created
				{
					Index: 0, Title: "ok-1",
					Actions: []model.ImportSnapshotAction{
						{Index: 0, Title: "a", Result: model.ImportActionResult{Status: model.ImportItemCreated}},
						{Index: 1, Title: "b", Result: model.ImportActionResult{Status: model.ImportItemCreated}},
					},
					Result: model.ImportCaseResult{Status: model.ImportItemCreated, CreatedCaseID: &caseID},
				},
				// Case 2: created, but one action failed and one action skipped
				{
					Index: 1, Title: "partial-2",
					Actions: []model.ImportSnapshotAction{
						{Index: 0, Title: "a", Result: model.ImportActionResult{Status: model.ImportItemCreated}},
						{Index: 1, Title: "b", Result: model.ImportActionResult{Status: model.ImportItemFailed}},
						{Index: 2, Title: "c", Result: model.ImportActionResult{Status: model.ImportItemSkipped}},
					},
					Result: model.ImportCaseResult{Status: model.ImportItemCreated, CreatedCaseID: &caseID},
				},
				// Case 3: skipped
				{
					Index: 2, Title: "skip-3",
					Actions: []model.ImportSnapshotAction{
						{Index: 0, Title: "a", Result: model.ImportActionResult{Status: model.ImportItemSkipped}},
					},
					Result: model.ImportCaseResult{Status: model.ImportItemSkipped},
				},
				// Case 4: failed
				{
					Index: 3, Title: "fail-4",
					Actions: []model.ImportSnapshotAction{
						{Index: 0, Title: "a", Result: model.ImportActionResult{Status: model.ImportItemSkipped}},
					},
					Result: model.ImportCaseResult{Status: model.ImportItemFailed},
				},
			},
		},
	}
	s.RecomputeCounts()
	// created: cases 1 and 2 (case-level created) → 2
	if s.CreatedCount != 2 {
		t.Errorf("CreatedCount = %d, want 2", s.CreatedCount)
	}
	// failed: case 4 (case-level) + case 2 action 1 (action-level) → 2
	if s.FailedCount != 2 {
		t.Errorf("FailedCount = %d, want 2", s.FailedCount)
	}
	// skipped: only case-level skipped → case 3 → 1
	// (case 2 action 2 is action-level skipped and not counted, by design)
	if s.SkippedCount != 1 {
		t.Errorf("SkippedCount = %d, want 1", s.SkippedCount)
	}
}

func TestImportSessionStatus_IsTerminal(t *testing.T) {
	cases := []struct {
		s    model.ImportSessionStatus
		want bool
	}{
		{model.ImportSessionPending, false},
		{model.ImportSessionApplied, true},
		{model.ImportSessionFailed, true},
	}
	for _, tc := range cases {
		if got := tc.s.IsTerminal(); got != tc.want {
			t.Errorf("IsTerminal(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestNewImportSessionID_Unique(t *testing.T) {
	seen := map[model.ImportSessionID]bool{}
	for i := range 100 {
		id := model.NewImportSessionID()
		if id == "" {
			t.Fatalf("got empty ID")
		}
		if seen[id] {
			t.Fatalf("duplicate ID %q at iteration %d", id, i)
		}
		seen[id] = true
	}
}
