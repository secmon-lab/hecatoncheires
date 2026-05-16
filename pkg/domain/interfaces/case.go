package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// CaseRepository defines the interface for Case data access
type CaseRepository interface {
	// Create creates a new case with auto-generated ID
	Create(ctx context.Context, workspaceID string, c *model.Case) (*model.Case, error)

	// Get retrieves a case by ID
	Get(ctx context.Context, workspaceID string, id int64) (*model.Case, error)

	// List retrieves cases with optional filtering.
	// Cases in DRAFT status are excluded by default; use ListDrafts to read
	// drafts. Passing WithStatus(CaseStatusDraft) honours the filter, but
	// callers should generally rely on ListDrafts for the draft-author view.
	List(ctx context.Context, workspaceID string, opts ...ListCaseOption) ([]*model.Case, error)

	// ListDrafts retrieves cases in DRAFT status authored by the given reporter
	// (Slack user ID). Drafts are author-scoped: each reporter sees only
	// their own drafts.
	ListDrafts(ctx context.Context, workspaceID, reporterID string) ([]*model.Case, error)

	// Update updates an existing case
	Update(ctx context.Context, workspaceID string, c *model.Case) (*model.Case, error)

	// Delete deletes a case by ID
	Delete(ctx context.Context, workspaceID string, id int64) error

	// GetBySlackChannelID retrieves a case by its Slack channel ID.
	// Returns nil, nil if no case is found with the given channel ID.
	GetBySlackChannelID(ctx context.Context, workspaceID string, channelID string) (*model.Case, error)

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
