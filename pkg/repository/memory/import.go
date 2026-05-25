package memory

import (
	"context"
	"maps"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type importRepository struct {
	mu       sync.RWMutex
	sessions map[string]map[model.ImportSessionID]*model.ImportSession // workspaceID -> ID -> session
}

func newImportRepository() *importRepository {
	return &importRepository{
		sessions: make(map[string]map[model.ImportSessionID]*model.ImportSession),
	}
}

// cloneImportSession makes a deep enough copy that the caller cannot
// mutate stored state by editing the returned value. Slices and the
// FieldValues map are duplicated; FieldValue.Value (which is `any`) is
// copied by reference — callers must treat it as immutable.
func cloneImportSession(s *model.ImportSession) *model.ImportSession {
	if s == nil {
		return nil
	}
	cp := *s
	if s.ExecutedAt != nil {
		t := *s.ExecutedAt
		cp.ExecutedAt = &t
	}
	cp.Issues = cloneIssues(s.Issues)
	cp.Snapshot = cloneSnapshot(s.Snapshot)
	return &cp
}

func cloneSnapshot(sn model.ImportSnapshot) model.ImportSnapshot {
	out := model.ImportSnapshot{Version: sn.Version}
	if sn.Cases == nil {
		return out
	}
	out.Cases = make([]model.ImportSnapshotCase, len(sn.Cases))
	for i, c := range sn.Cases {
		out.Cases[i] = cloneSnapshotCase(c)
	}
	return out
}

func cloneSnapshotCase(c model.ImportSnapshotCase) model.ImportSnapshotCase {
	cp := c
	if c.AssigneeIDs != nil {
		cp.AssigneeIDs = make([]string, len(c.AssigneeIDs))
		copy(cp.AssigneeIDs, c.AssigneeIDs)
	}
	if c.FieldValues != nil {
		cp.FieldValues = make(map[string]model.FieldValue, len(c.FieldValues))
		maps.Copy(cp.FieldValues, c.FieldValues)
	}
	cp.Issues = cloneIssues(c.Issues)
	if c.Actions != nil {
		cp.Actions = make([]model.ImportSnapshotAction, len(c.Actions))
		for i, a := range c.Actions {
			cp.Actions[i] = cloneSnapshotAction(a)
		}
	}
	cp.Result = cloneCaseResult(c.Result)
	return cp
}

func cloneSnapshotAction(a model.ImportSnapshotAction) model.ImportSnapshotAction {
	cp := a
	if a.DueDate != nil {
		t := *a.DueDate
		cp.DueDate = &t
	}
	cp.Issues = cloneIssues(a.Issues)
	cp.Result = cloneActionResult(a.Result)
	return cp
}

func cloneIssues(src []model.ImportIssue) []model.ImportIssue {
	if src == nil {
		return nil
	}
	out := make([]model.ImportIssue, len(src))
	copy(out, src)
	return out
}

func cloneCaseResult(r model.ImportCaseResult) model.ImportCaseResult {
	cp := r
	if r.CreatedCaseID != nil {
		v := *r.CreatedCaseID
		cp.CreatedCaseID = &v
	}
	if r.Error != nil {
		v := *r.Error
		cp.Error = &v
	}
	return cp
}

func cloneActionResult(r model.ImportActionResult) model.ImportActionResult {
	cp := r
	if r.CreatedActionID != nil {
		v := *r.CreatedActionID
		cp.CreatedActionID = &v
	}
	if r.Error != nil {
		v := *r.Error
		cp.Error = &v
	}
	return cp
}

func (r *importRepository) Create(ctx context.Context, workspaceID string, s *model.ImportSession) (*model.ImportSession, error) {
	if err := s.Validate(); err != nil {
		return nil, goerr.Wrap(err, "import session validation failed",
			goerr.V("workspace_id", workspaceID))
	}
	if s.WorkspaceID != workspaceID {
		return nil, goerr.New("workspaceID mismatch",
			goerr.V("argument", workspaceID),
			goerr.V("session", s.WorkspaceID))
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.sessions[workspaceID]; !ok {
		r.sessions[workspaceID] = make(map[model.ImportSessionID]*model.ImportSession)
	}
	if _, exists := r.sessions[workspaceID][s.ID]; exists {
		return nil, goerr.New("import session already exists",
			goerr.V("workspace_id", workspaceID),
			goerr.V("import_id", s.ID))
	}
	r.sessions[workspaceID][s.ID] = cloneImportSession(s)
	return cloneImportSession(s), nil
}

func (r *importRepository) Update(ctx context.Context, workspaceID string, s *model.ImportSession) (*model.ImportSession, error) {
	if err := s.Validate(); err != nil {
		return nil, goerr.Wrap(err, "import session validation failed",
			goerr.V("workspace_id", workspaceID))
	}
	if s.WorkspaceID != workspaceID {
		return nil, goerr.New("workspaceID mismatch",
			goerr.V("argument", workspaceID),
			goerr.V("session", s.WorkspaceID))
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	bucket, ok := r.sessions[workspaceID]
	if !ok {
		return nil, goerr.Wrap(ErrNotFound, "import session not found",
			goerr.V("workspace_id", workspaceID),
			goerr.V("import_id", s.ID))
	}
	if _, ok := bucket[s.ID]; !ok {
		return nil, goerr.Wrap(ErrNotFound, "import session not found",
			goerr.V("workspace_id", workspaceID),
			goerr.V("import_id", s.ID))
	}
	bucket[s.ID] = cloneImportSession(s)
	return cloneImportSession(s), nil
}

func (r *importRepository) Get(ctx context.Context, workspaceID string, id model.ImportSessionID) (*model.ImportSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	bucket, ok := r.sessions[workspaceID]
	if !ok {
		return nil, goerr.Wrap(ErrNotFound, "import session not found",
			goerr.V("workspace_id", workspaceID),
			goerr.V("import_id", id))
	}
	s, ok := bucket[id]
	if !ok {
		return nil, goerr.Wrap(ErrNotFound, "import session not found",
			goerr.V("workspace_id", workspaceID),
			goerr.V("import_id", id))
	}
	return cloneImportSession(s), nil
}
