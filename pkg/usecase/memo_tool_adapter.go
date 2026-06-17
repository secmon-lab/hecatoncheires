package usecase

import (
	"context"

	memotool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/memo"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// memoToolAdapter wraps a *MemoUseCase so the memo agent tools see it through
// the narrow memotool.MemoMutator surface, without the tool package importing
// pkg/usecase (which would create an import cycle). Memo writes derive their
// creator from the request context (empty for the token-less agent context, so
// agent-authored memos surface as "Agent"); there is no actor knob to pin here.
type memoToolAdapter struct {
	uc *MemoUseCase
}

// NewMemoToolAdapter returns a memotool.MemoMutator backed by the supplied
// MemoUseCase. Returns nil when uc is nil so the tool wiring can detect an
// unconfigured memo feature.
func NewMemoToolAdapter(uc *MemoUseCase) memotool.MemoMutator {
	if uc == nil {
		return nil
	}
	return &memoToolAdapter{uc: uc}
}

func (a *memoToolAdapter) CreateMemo(ctx context.Context, workspaceID string, caseID int64, title string, fields map[string]model.FieldValue) (*model.Memo, error) {
	return a.uc.CreateMemo(ctx, workspaceID, CreateMemoInput{
		CaseID:      caseID,
		Title:       title,
		FieldValues: fields,
	})
}

func (a *memoToolAdapter) UpdateMemo(ctx context.Context, workspaceID string, caseID int64, id model.MemoID, title *string, fields map[string]model.FieldValue) (*model.Memo, error) {
	return a.uc.UpdateMemo(ctx, workspaceID, UpdateMemoInput{
		ID:          id,
		CaseID:      caseID,
		Title:       title,
		FieldValues: fields,
	})
}

func (a *memoToolAdapter) ArchiveMemo(ctx context.Context, workspaceID string, caseID int64, id model.MemoID) (*model.Memo, error) {
	return a.uc.ArchiveMemo(ctx, workspaceID, caseID, id)
}
