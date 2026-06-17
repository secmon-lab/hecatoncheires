package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
)

// MemoID is a UUID-based identifier for a Memo.
//
// A string UUID (v7) is used rather than the int64 counter scheme of Case /
// Action: memos are created frequently by agents and live in a per-Case
// subcollection, so a counter document would add transaction contention and
// per-Case counter bookkeeping. UUID v7 is generated in-process (no counter
// read, multi-instance safe) and is time-ordered, which keeps CreatedAt-order
// listing and BigQuery export naturally sortable. This matches the existing
// string-UUID identifiers used by Source / ActionStep / ActionEvent / Session.
type MemoID string

// NewMemoID generates a new time-ordered UUID v7 MemoID.
func NewMemoID() MemoID {
	return MemoID(uuid.Must(uuid.NewV7()).String())
}

// String returns the raw string form of the MemoID.
func (id MemoID) String() string { return string(id) }

// ErrMemoValidation is returned when a Memo fails its structural invariants.
var ErrMemoValidation = goerr.New("memo validation failed")

// Memo is a Case-scoped "memory": a note attached to a single Case that records
// facts / observations / hypotheses / decisions accumulated while working the
// Case. Like a Case it carries workspace-defined custom field values; only ID
// and Title are fixed, everything else is a custom field.
//
// All identifiers (ID / WorkspaceID / CaseID) are flat top-level fields even
// though the Firestore path already encodes WorkspaceID and CaseID. This mirrors
// the JobRunLog convention so a document inspected in isolation answers "which
// workspace / case is this?" and a Firestore -> BigQuery export yields rows that
// JOIN directly on WorkspaceID / CaseID.
type Memo struct {
	ID          MemoID
	WorkspaceID string
	CaseID      int64
	Title       string
	// FieldValues holds the custom field values keyed by field id, using the
	// same representation as Case (model.FieldValue, validated by the shared
	// FieldValidator against the workspace's memo field schema).
	FieldValues map[string]FieldValue
	// CreatorID is the Slack user id of the human author. It is empty when the
	// memo was authored by an agent (system actor).
	CreatorID string
	// ArchivedAt is nil for an active memo and non-nil once archived (soft
	// delete). Archive state is expressed by this single field; there is no
	// derived boolean.
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Validate enforces the structural invariants required before any persistence
// write. Repositories MUST call it before every write so a usecase / handler
// bug that forgot to inject an identity field fails loudly at the first write.
func (m *Memo) Validate() error {
	if m == nil {
		return goerr.Wrap(ErrMemoValidation, "memo is nil")
	}
	if m.ID == "" {
		return goerr.Wrap(ErrMemoValidation, "memo ID is required")
	}
	if m.WorkspaceID == "" {
		return goerr.Wrap(ErrMemoValidation, "workspace ID is required")
	}
	if m.CaseID <= 0 {
		return goerr.Wrap(ErrMemoValidation, "case ID is required",
			goerr.V("case_id", m.CaseID))
	}
	if m.Title == "" {
		return goerr.Wrap(ErrMemoValidation, "title is required")
	}
	return nil
}

// IsArchived reports whether the memo is currently archived.
func (m *Memo) IsArchived() bool {
	return m != nil && m.ArchivedAt != nil
}
