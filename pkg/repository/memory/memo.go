package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// memoRepository stores memos indexed by workspaceID -> caseID -> memoID.
type memoRepository struct {
	mu    sync.RWMutex
	memos map[string]map[int64]map[model.MemoID]*model.Memo
}

func newMemoRepository() *memoRepository {
	return &memoRepository{
		memos: make(map[string]map[int64]map[model.MemoID]*model.Memo),
	}
}

func (r *memoRepository) ensureCase(workspaceID string, caseID int64) {
	if _, ok := r.memos[workspaceID]; !ok {
		r.memos[workspaceID] = make(map[int64]map[model.MemoID]*model.Memo)
	}
	if _, ok := r.memos[workspaceID][caseID]; !ok {
		r.memos[workspaceID][caseID] = make(map[model.MemoID]*model.Memo)
	}
}

// copyMemo creates a full deep copy of a Memo so mutations by the caller
// after Create/Update cannot silently alter the stored value.
func copyMemo(m *model.Memo) *model.Memo {
	copied := &model.Memo{
		ID:          m.ID,
		WorkspaceID: m.WorkspaceID,
		CaseID:      m.CaseID,
		Title:       m.Title,
		CreatorID:   m.CreatorID,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
	if m.ArchivedAt != nil {
		t := *m.ArchivedAt
		copied.ArchivedAt = &t
	}
	if m.FieldValues != nil {
		copied.FieldValues = make(map[string]model.FieldValue, len(m.FieldValues))
		for k, v := range m.FieldValues {
			copied.FieldValues[k] = v
		}
	}
	return copied
}

func (r *memoRepository) Create(ctx context.Context, workspaceID string, memo *model.Memo) (*model.Memo, error) {
	if err := memo.Validate(); err != nil {
		return nil, goerr.Wrap(err, "memo validation failed before create")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.ensureCase(workspaceID, memo.CaseID)

	stored := copyMemo(memo)
	r.memos[workspaceID][memo.CaseID][memo.ID] = stored
	return copyMemo(stored), nil
}

func (r *memoRepository) Get(ctx context.Context, workspaceID string, caseID int64, id model.MemoID) (*model.Memo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, ok := r.memos[workspaceID]
	if !ok {
		return nil, goerr.Wrap(ErrNotFound, "memo not found",
			goerr.V("memo_id", id),
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", caseID),
		)
	}
	cases, ok := ws[caseID]
	if !ok {
		return nil, goerr.Wrap(ErrNotFound, "memo not found",
			goerr.V("memo_id", id),
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", caseID),
		)
	}
	m, ok := cases[id]
	if !ok {
		return nil, goerr.Wrap(ErrNotFound, "memo not found",
			goerr.V("memo_id", id),
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", caseID),
		)
	}
	return copyMemo(m), nil
}

func (r *memoRepository) GetByIDs(ctx context.Context, workspaceID string, caseID int64, ids []model.MemoID) (map[model.MemoID]*model.Memo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[model.MemoID]*model.Memo, len(ids))
	ws, ok := r.memos[workspaceID]
	if !ok {
		return result, nil
	}
	cases, ok := ws[caseID]
	if !ok {
		return result, nil
	}
	for _, id := range ids {
		if m, ok := cases[id]; ok {
			result[id] = copyMemo(m)
		}
	}
	return result, nil
}

func (r *memoRepository) List(ctx context.Context, workspaceID string, caseID int64, opts interfaces.MemoListOptions) ([]*model.Memo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, ok := r.memos[workspaceID]
	if !ok {
		return []*model.Memo{}, nil
	}
	cases, ok := ws[caseID]
	if !ok {
		return []*model.Memo{}, nil
	}

	memos := make([]*model.Memo, 0, len(cases))
	for _, m := range cases {
		if !opts.ArchiveScope.Allows(m.IsArchived()) {
			continue
		}
		memos = append(memos, copyMemo(m))
	}

	// Sort by CreatedAt ascending to mirror the Firestore implementation.
	sort.Slice(memos, func(i, j int) bool {
		return memos[i].CreatedAt.Before(memos[j].CreatedAt)
	})

	return memos, nil
}

func (r *memoRepository) Update(ctx context.Context, workspaceID string, memo *model.Memo) (*model.Memo, error) {
	if err := memo.Validate(); err != nil {
		return nil, goerr.Wrap(err, "memo validation failed before update")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	ws, ok := r.memos[workspaceID]
	if !ok {
		return nil, goerr.Wrap(ErrNotFound, "memo not found",
			goerr.V("memo_id", memo.ID),
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", memo.CaseID),
		)
	}
	cases, ok := ws[memo.CaseID]
	if !ok {
		return nil, goerr.Wrap(ErrNotFound, "memo not found",
			goerr.V("memo_id", memo.ID),
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", memo.CaseID),
		)
	}
	if _, ok := cases[memo.ID]; !ok {
		return nil, goerr.Wrap(ErrNotFound, "memo not found",
			goerr.V("memo_id", memo.ID),
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", memo.CaseID),
		)
	}

	stored := copyMemo(memo)
	r.memos[workspaceID][memo.CaseID][memo.ID] = stored
	return copyMemo(stored), nil
}
