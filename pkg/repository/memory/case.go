package memory

import (
	"context"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

type caseRepository struct {
	mu     sync.RWMutex
	cases  map[string]map[int64]*model.Case
	nextID map[string]int64
}

func newCaseRepository() *caseRepository {
	return &caseRepository{
		cases:  make(map[string]map[int64]*model.Case),
		nextID: make(map[string]int64),
	}
}

func (r *caseRepository) ensureWorkspace(workspaceID string) {
	if _, exists := r.cases[workspaceID]; !exists {
		r.cases[workspaceID] = make(map[int64]*model.Case)
	}
	if _, exists := r.nextID[workspaceID]; !exists {
		r.nextID[workspaceID] = 1
	}
}

// copyFieldValue creates a deep copy of a field value
func copyFieldValue(fv model.FieldValue) model.FieldValue {
	copied := model.FieldValue{
		FieldID: fv.FieldID,
		Type:    fv.Type,
	}
	switch v := fv.Value.(type) {
	case []string:
		s := make([]string, len(v))
		copy(s, v)
		copied.Value = s
	case []interface{}:
		s := make([]interface{}, len(v))
		copy(s, v)
		copied.Value = s
	default:
		copied.Value = fv.Value
	}
	return copied
}

// copyCase creates a deep copy of a case
func copyCase(c *model.Case) *model.Case {
	assigneeIDs := make([]string, len(c.AssigneeIDs))
	copy(assigneeIDs, c.AssigneeIDs)

	var fieldValues map[string]model.FieldValue
	if c.FieldValues != nil {
		fieldValues = make(map[string]model.FieldValue, len(c.FieldValues))
		for k, v := range c.FieldValues {
			fieldValues[k] = copyFieldValue(v)
		}
	}

	channelUserIDs := make([]string, len(c.ChannelUserIDs))
	copy(channelUserIDs, c.ChannelUserIDs)

	var agentSourceIDs []model.SourceID
	if c.AgentSourceIDs != nil {
		agentSourceIDs = make([]model.SourceID, len(c.AgentSourceIDs))
		copy(agentSourceIDs, c.AgentSourceIDs)
	}

	return &model.Case{
		ID:                    c.ID,
		Title:                 c.Title,
		Description:           c.Description,
		Status:                c.Status,
		ReporterID:            c.ReporterID,
		AssigneeIDs:           assigneeIDs,
		SlackChannelID:        c.SlackChannelID,
		IsPrivate:             c.IsPrivate,
		ChannelUserIDs:        channelUserIDs,
		FieldValues:           fieldValues,
		RequestKey:            c.RequestKey,
		AgentAdditionalPrompt: c.AgentAdditionalPrompt,
		AgentSourceIDs:        agentSourceIDs,
		CreatedAt:             c.CreatedAt,
		UpdatedAt:             c.UpdatedAt,
	}
}

func (r *caseRepository) Create(ctx context.Context, workspaceID string, c *model.Case) (*model.Case, error) {
	// Validate at the persistence boundary so a usecase / handler bug
	// that forgets to inject the reporter (e.g. Slack interactivity
	// callback without auth.ContextWithToken) fails loudly the first
	// time it runs — instead of silently writing a case the UI cannot
	// attribute to anyone.
	if err := c.ValidateNew(); err != nil {
		return nil, goerr.Wrap(err, "case validation failed before create")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.ensureWorkspace(workspaceID)

	created := copyCase(c)
	created.ID = r.nextID[workspaceID]
	r.nextID[workspaceID]++

	r.cases[workspaceID][created.ID] = created
	return copyCase(created), nil
}

func (r *caseRepository) Get(ctx context.Context, workspaceID string, id int64) (*model.Case, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.cases[workspaceID]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", id))
	}

	c, exists := ws[id]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", id))
	}

	return copyCase(c), nil
}

func (r *caseRepository) GetByIDs(ctx context.Context, workspaceID string, ids []int64) (map[int64]*model.Case, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[int64]*model.Case, len(ids))
	ws, exists := r.cases[workspaceID]
	if !exists {
		return result, nil
	}

	for _, id := range ids {
		if c, ok := ws[id]; ok {
			result[id] = copyCase(c)
		}
	}

	return result, nil
}

func (r *caseRepository) List(ctx context.Context, workspaceID string, opts ...interfaces.ListCaseOption) ([]*model.Case, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.cases[workspaceID]
	if !exists {
		return []*model.Case{}, nil
	}

	cfg := interfaces.BuildListCaseConfig(opts...)

	cases := make([]*model.Case, 0, len(ws))
	for _, c := range ws {
		// Apply status filter. When no filter is set, exclude drafts so the
		// default listing never leaks unsubmitted entries; callers that want
		// drafts must go through ListDrafts (author-scoped) or pass
		// WithStatus(CaseStatusDraft) explicitly.
		if statusFilter := cfg.Status(); statusFilter != nil {
			if c.Status.Normalize() != *statusFilter {
				continue
			}
		} else if c.IsDraft() {
			continue
		}
		cases = append(cases, copyCase(c))
	}

	return cases, nil
}

func (r *caseRepository) ListDrafts(ctx context.Context, workspaceID string) ([]*model.Case, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.cases[workspaceID]
	if !exists {
		return []*model.Case{}, nil
	}

	drafts := make([]*model.Case, 0)
	for _, c := range ws {
		if !c.IsDraft() {
			continue
		}
		drafts = append(drafts, copyCase(c))
	}

	return drafts, nil
}

func (r *caseRepository) Update(ctx context.Context, workspaceID string, c *model.Case) (*model.Case, error) {
	if err := c.Validate(); err != nil {
		return nil, goerr.Wrap(err, "case validation failed before update")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	ws, exists := r.cases[workspaceID]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", c.ID))
	}

	if _, exists := ws[c.ID]; !exists {
		return nil, goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", c.ID))
	}

	updated := copyCase(c)
	r.cases[workspaceID][updated.ID] = updated
	return copyCase(updated), nil
}

func (r *caseRepository) Delete(ctx context.Context, workspaceID string, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ws, exists := r.cases[workspaceID]
	if !exists {
		return goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", id))
	}

	if _, exists := ws[id]; !exists {
		return goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", id))
	}

	delete(r.cases[workspaceID], id)
	return nil
}

func (r *caseRepository) GetBySlackChannelID(ctx context.Context, workspaceID string, channelID string) (*model.Case, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.cases[workspaceID]
	if !exists {
		return nil, nil
	}

	for _, c := range ws {
		if c.SlackChannelID == channelID {
			return copyCase(c), nil
		}
	}

	return nil, nil
}

func (r *caseRepository) GetByRequestKey(_ context.Context, workspaceID string, key string) (*model.Case, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.cases[workspaceID]
	if !exists {
		return nil, nil
	}

	for _, c := range ws {
		if c.RequestKey == key {
			return copyCase(c), nil
		}
	}

	return nil, nil
}

func (r *caseRepository) CountFieldValues(_ context.Context, workspaceID string, fieldID string, fieldType types.FieldType, validValues []string) (int64, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws := r.cases[workspaceID]
	validSet := make(map[string]bool, len(validValues))
	for _, v := range validValues {
		validSet[v] = true
	}

	var total, valid int64
	for _, c := range ws {
		fv, ok := c.FieldValues[fieldID]
		if !ok || fv.Type != fieldType {
			continue
		}
		total++
		if fv.IsValueInSet(fieldType, validSet) {
			valid++
		}
	}

	return total, valid, nil
}

func (r *caseRepository) FindCaseWithInvalidFieldValue(_ context.Context, workspaceID string, fieldID string, fieldType types.FieldType, validValues []string) (*model.Case, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws := r.cases[workspaceID]
	validSet := make(map[string]bool, len(validValues))
	for _, v := range validValues {
		validSet[v] = true
	}

	for _, c := range ws {
		fv, ok := c.FieldValues[fieldID]
		if !ok || fv.Type != fieldType {
			continue
		}
		if !fv.IsValueInSet(fieldType, validSet) {
			return copyCase(c), nil
		}
	}

	return nil, nil
}
