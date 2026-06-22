package usecase

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casewriter"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// caseToolAdapter bridges the casewriter tool package onto CaseUseCase. The
// two CaseUpdate structs are intentionally separate so the agent tool package
// does not import pkg/usecase (which would create an import cycle). This
// adapter lives in the usecase layer (mirroring NewActionToolAdapter) so every
// host — Jobs, the case-bound mention agent, the eval harness — wires the same
// concrete bridge rather than each reimplementing it.
type caseToolAdapter struct {
	uc *CaseUseCase
}

// NewCaseToolAdapter wraps a CaseUseCase as a casewriter.CaseMutator. Returns
// nil when uc is nil so callers can pass the result straight through to
// casewriter.Deps (a nil mutator disables the tool, which fails loudly at
// runtime rather than silently degrading).
func NewCaseToolAdapter(uc *CaseUseCase) casewriter.CaseMutator {
	if uc == nil {
		return nil
	}
	return &caseToolAdapter{uc: uc}
}

func (a *caseToolAdapter) UpdateCase(ctx context.Context, workspaceID string, id int64, patch casewriter.CaseUpdate) (*model.Case, error) {
	in := CaseUpdate{
		Title:       patch.Title,
		Description: patch.Description,
		Fields:      patch.Fields,
	}
	return a.uc.UpdateCase(ctx, workspaceID, id, in)
}

func (a *caseToolAdapter) UpdateCaseStatus(ctx context.Context, workspaceID string, id int64, boardStatus string) (*model.Case, error) {
	return a.uc.UpdateCaseStatus(ctx, workspaceID, id, boardStatus)
}

func (a *caseToolAdapter) CloseCase(ctx context.Context, workspaceID string, id int64) (*model.Case, error) {
	return a.uc.CloseCase(ctx, workspaceID, id)
}

func (a *caseToolAdapter) AssignCase(ctx context.Context, workspaceID string, id int64, userIDs []string) (*model.Case, error) {
	return a.uc.AssignCase(ctx, workspaceID, id, userIDs)
}

func (a *caseToolAdapter) UnassignCase(ctx context.Context, workspaceID string, id int64, userIDs []string) (*model.Case, error) {
	return a.uc.UnassignCase(ctx, workspaceID, id, userIDs)
}
