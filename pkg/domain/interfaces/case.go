package interfaces

import (
	"context"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// CaseRepository defines the interface for Case data access
type CaseRepository interface {
	// Create creates a new case with auto-generated ID
	Create(ctx context.Context, workspaceID string, c *model.Case) (*model.Case, error)

	// Get retrieves a case by ID
	Get(ctx context.Context, workspaceID string, id int64) (*model.Case, error)

	// GetByIDs retrieves multiple cases by IDs in a single batch.
	// Returns a map keyed by case ID containing only the cases that
	// were found; missing IDs are silently absent from the result map
	// (callers must distinguish "missing" from "found"). This is the
	// batch fetch hook used by the GraphQL DataLoader to collapse
	// per-row Reporter / Assignees lookups into one repository call
	// per request.
	GetByIDs(ctx context.Context, workspaceID string, ids []int64) (map[int64]*model.Case, error)

	// List retrieves cases with optional filtering.
	// Cases in DRAFT status are excluded by default; use ListDrafts to read
	// drafts. Passing WithStatus(CaseStatusDraft) honours the filter, but
	// callers should generally rely on ListDrafts for the draft-author view.
	List(ctx context.Context, workspaceID string, opts ...ListCaseOption) ([]*model.Case, error)

	// ListDrafts retrieves all cases in DRAFT status across the workspace.
	// Drafts are surfaced workspace-wide so any team member can pick up an
	// in-progress entry; the usecase layer applies private-draft access
	// control (private drafts are visible only to their reporter).
	ListDrafts(ctx context.Context, workspaceID string) ([]*model.Case, error)

	// Update updates an existing case
	Update(ctx context.Context, workspaceID string, c *model.Case) (*model.Case, error)

	// AddAssignees atomically unions the given Slack user IDs into the case's
	// assignee set and returns the updated case. IDs already present are
	// ignored. Unlike Update (which replaces the whole assignee list and so
	// races with concurrent edits), this reads and writes the case inside a
	// single transaction, so simultaneous "assign me" actions cannot clobber
	// one another. updatedAt is supplied by the caller — the repository never
	// reads the clock. The full case document is rewritten, so model
	// invariants are re-validated.
	AddAssignees(ctx context.Context, workspaceID string, id int64, userIDs []string, updatedAt time.Time) (*model.Case, error)

	// RemoveAssignees atomically removes the given Slack user IDs from the
	// case's assignee set and returns the updated case. IDs not present are
	// ignored. Concurrency and updatedAt semantics match AddAssignees.
	RemoveAssignees(ctx context.Context, workspaceID string, id int64, userIDs []string, updatedAt time.Time) (*model.Case, error)

	// Delete deletes a case by ID
	Delete(ctx context.Context, workspaceID string, id int64) error

	// GetBySlackChannelID retrieves a case by its Slack channel ID.
	// Returns nil, nil if no case is found with the given channel ID.
	GetBySlackChannelID(ctx context.Context, workspaceID string, channelID string) (*model.Case, error)

	// GetBySlackThread retrieves a thread-mode case by its monitored channel
	// and thread timestamp. Returns nil, nil if no matching case exists. Used
	// for thread-mode case lookup (idempotent creation and reply ingest).
	GetBySlackThread(ctx context.Context, workspaceID string, channelID string, threadTS string) (*model.Case, error)

	// GetByRequestKey retrieves a case by its request key.
	// Returns nil, nil if no case is found with the given key.
	GetByRequestKey(ctx context.Context, workspaceID string, key string) (*model.Case, error)

	// CountFieldValues counts the total number of cases with the specified field
	// and how many of those have a value matching one of validValues.
	// invalidCount = total - valid detects the existence of invalid values
	// without transferring document data (uses aggregation queries).
	CountFieldValues(ctx context.Context, workspaceID string, fieldID string, fieldType types.FieldType, validValues []string) (total int64, valid int64, err error)

	// FindCaseWithInvalidFieldValue returns one case where the specified field
	// has a value not in validValues. Returns nil if all values are valid.
	// Intended to be called after CountFieldValues confirms invalid values exist.
	FindCaseWithInvalidFieldValue(ctx context.Context, workspaceID string, fieldID string, fieldType types.FieldType, validValues []string) (*model.Case, error)
}
