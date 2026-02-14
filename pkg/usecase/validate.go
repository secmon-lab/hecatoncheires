package usecase

import (
	"context"
	"fmt"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// ValidationIssue represents a single validation issue found during DB consistency check
type ValidationIssue struct {
	WorkspaceID string
	CaseID      int64
	FieldID     string
	Message     string
	Expected    string
	Actual      string
}

// ValidationResult holds the results of DB validation
type ValidationResult struct {
	Issues []ValidationIssue
}

// HasIssues returns true if there are any validation issues
func (r *ValidationResult) HasIssues() bool {
	return len(r.Issues) > 0
}

// AddIssue adds a validation issue to the result
func (r *ValidationResult) AddIssue(issue ValidationIssue) {
	r.Issues = append(r.Issues, issue)
}

// ValidateDB validates that select/multi-select field values in the DB
// are consistent with the field schema defined in configuration.
// It uses count-based detection to avoid transferring document data,
// and only fetches a sample case when an inconsistency is found.
// It does NOT modify any data.
func (uc *UseCases) ValidateDB(ctx context.Context) (*ValidationResult, error) {
	result := &ValidationResult{}

	entries := uc.workspaceRegistry.List()
	for _, entry := range entries {
		wsID := entry.Workspace.ID
		schema := entry.FieldSchema

		for _, fieldDef := range schema.Fields {
			if fieldDef.Type != types.FieldTypeSelect && fieldDef.Type != types.FieldTypeMultiSelect {
				continue
			}

			validOptionIDs := extractOptionIDs(fieldDef)

			// Phase 1: Count-based detection (no document data transfer)
			total, valid, err := uc.repo.Case().CountFieldValues(
				ctx, wsID, fieldDef.ID, fieldDef.Type, validOptionIDs,
			)
			if err != nil {
				return nil, goerr.Wrap(err, "failed to count field values",
					goerr.V("workspace_id", wsID),
					goerr.V("field_id", fieldDef.ID))
			}

			invalidCount := total - valid
			if invalidCount == 0 {
				continue
			}

			// Phase 2: Fetch one sample case for the error report
			sample, err := uc.repo.Case().FindCaseWithInvalidFieldValue(
				ctx, wsID, fieldDef.ID, fieldDef.Type, validOptionIDs,
			)
			if err != nil {
				return nil, goerr.Wrap(err, "failed to find invalid field value sample",
					goerr.V("workspace_id", wsID),
					goerr.V("field_id", fieldDef.ID))
			}

			actual := "<unknown>"
			if sample != nil {
				if fv, ok := sample.FieldValues[fieldDef.ID]; ok {
					actual = fmt.Sprint(fv.Value)
				}

				result.AddIssue(ValidationIssue{
					WorkspaceID: wsID,
					CaseID:      sample.ID,
					FieldID:     fieldDef.ID,
					Message:     fmt.Sprintf("found %d case(s) with invalid option value", invalidCount),
					Expected:    "valid option ID",
					Actual:      actual,
				})
			}
		}
	}

	return result, nil
}

// extractOptionIDs returns the list of valid option IDs from a field definition
func extractOptionIDs(fieldDef config.FieldDefinition) []string {
	ids := make([]string, len(fieldDef.Options))
	for i, opt := range fieldDef.Options {
		ids[i] = opt.ID
	}
	return ids
}
