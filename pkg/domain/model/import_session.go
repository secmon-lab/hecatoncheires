package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
)

// ImportSessionID is a UUID-based identifier for ImportSession.
type ImportSessionID string

// NewImportSessionID generates a new UUID v4 ImportSessionID.
func NewImportSessionID() ImportSessionID {
	return ImportSessionID(uuid.New().String())
}

// String returns the string representation.
func (id ImportSessionID) String() string {
	return string(id)
}

// ImportSessionStatus is the lifecycle status of an ImportSession.
//
// Allowed transitions: pending → applied | failed. Once an ImportSession
// has reached applied or failed, it is terminal; retrying requires
// creating a new ImportSession via createCaseImport.
type ImportSessionStatus string

const (
	ImportSessionPending ImportSessionStatus = "pending"
	ImportSessionApplied ImportSessionStatus = "applied"
	ImportSessionFailed  ImportSessionStatus = "failed"
)

// IsTerminal reports whether the session is in a state that cannot be
// further mutated (applied or failed).
func (s ImportSessionStatus) IsTerminal() bool {
	return s == ImportSessionApplied || s == ImportSessionFailed
}

// IsValidImportSessionStatus reports whether s is a known status value.
func IsValidImportSessionStatus(s ImportSessionStatus) bool {
	switch s {
	case ImportSessionPending, ImportSessionApplied, ImportSessionFailed:
		return true
	}
	return false
}

// ImportItemResultStatus represents the per-Case / per-Action execution
// result. pending = not yet executed; created = persisted successfully;
// failed = persistence attempted and failed; skipped = not attempted because
// an earlier item in the same execute call failed.
type ImportItemResultStatus string

const (
	ImportItemPending ImportItemResultStatus = "pending"
	ImportItemCreated ImportItemResultStatus = "created"
	ImportItemFailed  ImportItemResultStatus = "failed"
	ImportItemSkipped ImportItemResultStatus = "skipped"
)

// IsValidImportItemResultStatus reports whether s is a known per-item
// result status.
func IsValidImportItemResultStatus(s ImportItemResultStatus) bool {
	switch s {
	case ImportItemPending, ImportItemCreated, ImportItemFailed, ImportItemSkipped:
		return true
	}
	return false
}

// ImportIssueSeverity classifies a single ImportIssue. error blocks
// execute; warning is informational only.
type ImportIssueSeverity string

const (
	ImportIssueError   ImportIssueSeverity = "error"
	ImportIssueWarning ImportIssueSeverity = "warning"
)

// IsValidImportIssueSeverity reports whether s is a known severity.
func IsValidImportIssueSeverity(s ImportIssueSeverity) bool {
	switch s {
	case ImportIssueError, ImportIssueWarning:
		return true
	}
	return false
}

// ImportSession is the persistent record of one "import YAML → Case/Action"
// workflow. Created by createCaseImport (status=pending), advanced once by
// executeCaseImport (→ applied or failed). Stored at
// tenants/{tenantID}/imports/{importID} in Firestore. The owning user
// (CreatorUserID) is the only principal allowed to read or execute it.
type ImportSession struct {
	ID            ImportSessionID
	WorkspaceID   string
	CreatorUserID string // Slack User ID of the creator (immutable)
	Status        ImportSessionStatus

	Source   ImportSource   // Original file metadata
	Snapshot ImportSnapshot // Normalized payload (per-Case result is stored here)
	Issues   []ImportIssue  // Session-level issues (parse failure, version, schema stale, etc.)

	// FieldSchemaHash is a digest of the workspace field schema captured at
	// createCaseImport time. executeCaseImport compares against the current
	// hash and refuses to run if it differs (callers must create a new import).
	FieldSchemaHash string

	CreatedAt time.Time
	UpdatedAt time.Time

	// ExecutedAt is set when status transitions to applied or failed.
	ExecutedAt *time.Time

	// Aggregate counts over Snapshot.Cases[*].Result.Status. Recomputed
	// by the usecase on every Update so they stay consistent.
	CreatedCount int
	FailedCount  int
	SkippedCount int
}

// ImportSource captures metadata about the uploaded YAML payload.
type ImportSource struct {
	OriginalFileName string
	ContentDigest    string // sha256 of the raw YAML, hex-encoded
	SizeBytes        int
}

// ImportSnapshot is the workspace-normalized interpretation of the
// uploaded YAML. Per-Case results live inside Cases[*].Result; the
// snapshot itself is mutated in place as execute progresses.
type ImportSnapshot struct {
	Version int
	Cases   []ImportSnapshotCase
}

// ImportSnapshotCase is one Case to be created. Issues holds per-Case
// validation findings detected at createCaseImport time (and at execute
// time when field schema has drifted). Result is the per-Case execution
// outcome.
type ImportSnapshotCase struct {
	Index       int
	Title       string
	Description string
	IsPrivate   bool
	AssigneeIDs []string
	FieldValues map[string]FieldValue
	Actions     []ImportSnapshotAction
	Issues      []ImportIssue
	Result      ImportCaseResult
}

// ImportSnapshotAction is one Action to be created under its parent
// Snapshot Case. AssigneeID is a single Slack User ID (matches the
// existing Action.AssigneeID semantics).
type ImportSnapshotAction struct {
	Index       int
	Title       string
	Description string
	AssigneeID  string
	DueDate     *time.Time
	Issues      []ImportIssue
	Result      ImportActionResult
}

// ImportCaseResult is the per-Case execution outcome. CreatedCaseID is
// set when Status == created; Error is set when Status == failed.
type ImportCaseResult struct {
	Status        ImportItemResultStatus
	CreatedCaseID *int64
	Error         *ImportIssue
}

// ImportActionResult is the per-Action execution outcome, parallel to
// ImportCaseResult.
type ImportActionResult struct {
	Status          ImportItemResultStatus
	CreatedActionID *int64
	Error           *ImportIssue
}

// ImportIssue is a single validation or execution-time finding. Path is
// a JSONPath-like locator (e.g. "cases[0].fields.severity") used by the
// frontend to highlight the offending value. Message is the human-
// readable text, already translated to the user's language (the i18n
// boundary lives in the usecase that produces the issue).
type ImportIssue struct {
	Path     string
	Message  string
	Severity ImportIssueSeverity
}

// Sentinel errors. Repositories MUST call Validate before every write so
// that an upstream bug (e.g. forgetting to populate CreatorUserID after
// auth.ContextWithToken) fails loudly at the persistence boundary
// instead of silently producing an unattributable record.
var (
	ErrImportSessionMissingID            = goerr.New("import session has no ID")
	ErrImportSessionMissingWorkspace     = goerr.New("import session has no workspace ID")
	ErrImportSessionMissingCreator       = goerr.New("import session has no creator user ID")
	ErrImportSessionInvalidStatus        = goerr.New("import session has invalid status")
	ErrImportSessionInvalidSnapshot      = goerr.New("import session snapshot is invalid")
	ErrImportSessionInvalidIssueSeverity = goerr.New("import session issue has invalid severity")
	ErrImportSessionInvalidItemStatus    = goerr.New("import session item has invalid status")
)

// Validate checks the invariants every persisted ImportSession must
// satisfy. Repositories MUST call this before every write.
func (s *ImportSession) Validate() error {
	if s == nil {
		return goerr.New("import session is nil")
	}
	if s.ID == "" {
		return goerr.Wrap(ErrImportSessionMissingID, "import session is missing ID")
	}
	if s.WorkspaceID == "" {
		return goerr.Wrap(ErrImportSessionMissingWorkspace,
			"import session is missing workspace ID",
			goerr.V("import_id", s.ID))
	}
	if s.CreatorUserID == "" {
		return goerr.Wrap(ErrImportSessionMissingCreator,
			"import session is missing creator user ID",
			goerr.V("import_id", s.ID))
	}
	if !IsValidImportSessionStatus(s.Status) {
		return goerr.Wrap(ErrImportSessionInvalidStatus,
			"import session has unknown status",
			goerr.V("import_id", s.ID),
			goerr.V("status", string(s.Status)))
	}
	for i, issue := range s.Issues {
		if !IsValidImportIssueSeverity(issue.Severity) {
			return goerr.Wrap(ErrImportSessionInvalidIssueSeverity,
				"session issue has unknown severity",
				goerr.V("import_id", s.ID),
				goerr.V("issue_index", i),
				goerr.V("severity", string(issue.Severity)))
		}
	}
	if err := s.Snapshot.validate(s.ID); err != nil {
		return err
	}
	return nil
}

func (sn *ImportSnapshot) validate(sessionID ImportSessionID) error {
	for ci, c := range sn.Cases {
		if !IsValidImportItemResultStatus(c.Result.Status) {
			return goerr.Wrap(ErrImportSessionInvalidItemStatus,
				"case result has unknown status",
				goerr.V("import_id", sessionID),
				goerr.V("case_index", ci),
				goerr.V("status", string(c.Result.Status)))
		}
		for ii, issue := range c.Issues {
			if !IsValidImportIssueSeverity(issue.Severity) {
				return goerr.Wrap(ErrImportSessionInvalidIssueSeverity,
					"case issue has unknown severity",
					goerr.V("import_id", sessionID),
					goerr.V("case_index", ci),
					goerr.V("issue_index", ii),
					goerr.V("severity", string(issue.Severity)))
			}
		}
		for ai, a := range c.Actions {
			if !IsValidImportItemResultStatus(a.Result.Status) {
				return goerr.Wrap(ErrImportSessionInvalidItemStatus,
					"action result has unknown status",
					goerr.V("import_id", sessionID),
					goerr.V("case_index", ci),
					goerr.V("action_index", ai),
					goerr.V("status", string(a.Result.Status)))
			}
			for ii, issue := range a.Issues {
				if !IsValidImportIssueSeverity(issue.Severity) {
					return goerr.Wrap(ErrImportSessionInvalidIssueSeverity,
						"action issue has unknown severity",
						goerr.V("import_id", sessionID),
						goerr.V("case_index", ci),
						goerr.V("action_index", ai),
						goerr.V("issue_index", ii),
						goerr.V("severity", string(issue.Severity)))
				}
			}
		}
	}
	return nil
}

// HasErrorIssues reports whether the session has any error-severity
// issue, including those nested under per-Case / per-Action issues.
// Used to compute the GraphQL `valid` field and to gate executeCaseImport.
func (s *ImportSession) HasErrorIssues() bool {
	for _, i := range s.Issues {
		if i.Severity == ImportIssueError {
			return true
		}
	}
	for _, c := range s.Snapshot.Cases {
		for _, i := range c.Issues {
			if i.Severity == ImportIssueError {
				return true
			}
		}
		for _, a := range c.Actions {
			for _, i := range a.Issues {
				if i.Severity == ImportIssueError {
					return true
				}
			}
		}
	}
	return false
}

// Valid is the logical opposite of HasErrorIssues. Provided as a method
// so converters can mirror the GraphQL `valid: Boolean!` field directly.
func (s *ImportSession) Valid() bool {
	return !s.HasErrorIssues()
}

// RecomputeCounts updates CreatedCount / FailedCount / SkippedCount from
// the current Snapshot. The usecase calls this whenever Snapshot.Cases
// is mutated so the cached counts stay consistent with reality.
//
// Counting rules (matches the spec / Design Canvas i05):
//   - "created" counts Cases with Result.Status == created.
//   - "failed" counts Cases with Result.Status == failed AND Actions
//     with Result.Status == failed (a Case can be created while one of
//     its Actions fails — that Action contributes to FailedCount).
//   - "skipped" counts Cases with Result.Status == skipped only.
//     Action-level skipped (under a created Case whose earlier Action
//     failed) is NOT counted, matching the summary line shown to the
//     user (Design Canvas treats the summary as Case-level except for
//     failed which sweeps in Action-level failures too).
func (s *ImportSession) RecomputeCounts() {
	created := 0
	failed := 0
	skipped := 0
	for _, c := range s.Snapshot.Cases {
		switch c.Result.Status {
		case ImportItemCreated:
			created++
		case ImportItemFailed:
			failed++
		case ImportItemSkipped:
			skipped++
		}
		for _, a := range c.Actions {
			if a.Result.Status == ImportItemFailed {
				failed++
			}
		}
	}
	s.CreatedCount = created
	s.FailedCount = failed
	s.SkippedCount = skipped
}
