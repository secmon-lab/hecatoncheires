package memory

import (
	"context"
	"sync"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type fieldValueKey struct {
	CaseID  int64
	FieldID string
}

type fieldValueRepository struct {
	mu     sync.RWMutex
	values map[fieldValueKey]*model.FieldValue
}

func newFieldValueRepository() *fieldValueRepository {
	return &fieldValueRepository{
		values: make(map[fieldValueKey]*model.FieldValue),
	}
}

// copyFieldValue creates a deep copy of a field value
// Note: Value field might contain complex types, so we need to handle them carefully
func copyFieldValue(fv *model.FieldValue) *model.FieldValue {
	copied := &model.FieldValue{
		CaseID:    fv.CaseID,
		FieldID:   fv.FieldID,
		UpdatedAt: fv.UpdatedAt,
	}

	// Copy value based on its type
	switch v := fv.Value.(type) {
	case []string:
		slice := make([]string, len(v))
		copy(slice, v)
		copied.Value = slice
	case []interface{}:
		slice := make([]interface{}, len(v))
		copy(slice, v)
		copied.Value = slice
	default:
		// For simple types (string, int, float64, bool, time.Time), direct assignment is safe
		copied.Value = fv.Value
	}

	return copied
}

func (r *fieldValueRepository) GetByCaseID(ctx context.Context, caseID int64) ([]model.FieldValue, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fieldValues := make([]model.FieldValue, 0)
	for key, fv := range r.values {
		if key.CaseID == caseID {
			fieldValues = append(fieldValues, *copyFieldValue(fv))
		}
	}

	return fieldValues, nil
}

func (r *fieldValueRepository) GetByCaseIDs(ctx context.Context, caseIDs []int64) (map[int64][]model.FieldValue, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create a map for quick lookup
	caseIDMap := make(map[int64]bool)
	for _, caseID := range caseIDs {
		caseIDMap[caseID] = true
	}

	// Initialize result map with empty slices for each case ID
	result := make(map[int64][]model.FieldValue)
	for _, caseID := range caseIDs {
		result[caseID] = make([]model.FieldValue, 0)
	}

	// Collect field values for each case
	for key, fv := range r.values {
		if caseIDMap[key.CaseID] {
			result[key.CaseID] = append(result[key.CaseID], *copyFieldValue(fv))
		}
	}

	return result, nil
}

func (r *fieldValueRepository) Save(ctx context.Context, fv *model.FieldValue) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := fieldValueKey{
		CaseID:  fv.CaseID,
		FieldID: fv.FieldID,
	}

	saved := copyFieldValue(fv)
	saved.UpdatedAt = time.Now().UTC()

	r.values[key] = saved
	return nil
}

func (r *fieldValueRepository) DeleteByCaseID(ctx context.Context, caseID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Collect keys to delete
	keysToDelete := make([]fieldValueKey, 0)
	for key := range r.values {
		if key.CaseID == caseID {
			keysToDelete = append(keysToDelete, key)
		}
	}

	// Delete collected keys
	for _, key := range keysToDelete {
		delete(r.values, key)
	}

	return nil
}
