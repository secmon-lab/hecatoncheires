package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// FieldValueRepository defines the interface for FieldValue data access
// Note: Custom fields are only for Cases, not Actions
type FieldValueRepository interface {
	// GetByCaseID retrieves all field values for a specific case
	GetByCaseID(ctx context.Context, caseID int64) ([]model.FieldValue, error)

	// GetByCaseIDs retrieves field values for multiple cases (for batch operations)
	// Returns a map of case ID to list of field values
	GetByCaseIDs(ctx context.Context, caseIDs []int64) (map[int64][]model.FieldValue, error)

	// Save creates or updates a field value
	// If a field value with the same CaseID and FieldID exists, it will be updated
	Save(ctx context.Context, fv *model.FieldValue) error

	// DeleteByCaseID deletes all field values associated with a specific case
	DeleteByCaseID(ctx context.Context, caseID int64) error
}
