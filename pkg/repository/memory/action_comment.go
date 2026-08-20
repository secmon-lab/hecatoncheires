package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type actionCommentRepository struct {
	mu       sync.RWMutex
	comments map[string][]*model.ActionComment // key: "{workspaceID}/{actionID}"
}

var _ interfaces.ActionCommentRepository = &actionCommentRepository{}

func newActionCommentRepository() *actionCommentRepository {
	return &actionCommentRepository{comments: make(map[string][]*model.ActionComment)}
}

func actionCommentKey(workspaceID string, actionID int64) string {
	return fmt.Sprintf("%s/%d", workspaceID, actionID)
}

func copyActionComment(c *model.ActionComment) *model.ActionComment {
	dup := *c
	return &dup
}

func (r *actionCommentRepository) Put(ctx context.Context, workspaceID string, actionID int64, comment *model.ActionComment) error {
	if err := comment.Validate(); err != nil {
		return goerr.Wrap(err, "action comment validation failed before put")
	}
	// The comment is stored under the actionID parameter's key; reject a struct
	// whose own ActionID points elsewhere so the two can never diverge.
	if comment.ActionID != actionID {
		return goerr.Wrap(model.ErrActionCommentValidation, "action comment ActionID does not match parameter",
			goerr.V("param", actionID), goerr.V("comment", comment.ActionID))
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := actionCommentKey(workspaceID, actionID)
	existing := r.comments[key]
	for i, c := range existing {
		if c.ID == comment.ID {
			existing[i] = copyActionComment(comment)
			r.comments[key] = existing
			return nil
		}
	}
	r.comments[key] = append(existing, copyActionComment(comment))
	return nil
}

func (r *actionCommentRepository) Get(ctx context.Context, workspaceID string, actionID int64, commentID string) (*model.ActionComment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, c := range r.comments[actionCommentKey(workspaceID, actionID)] {
		if c.ID == commentID {
			return copyActionComment(c), nil
		}
	}
	return nil, goerr.Wrap(ErrNotFound, "action comment not found",
		goerr.V("workspace_id", workspaceID),
		goerr.V("action_id", actionID),
		goerr.V("comment_id", commentID))
}

func (r *actionCommentRepository) List(ctx context.Context, workspaceID string, actionID int64, limit int, cursor string) ([]*model.ActionComment, string, error) {
	if limit <= 0 {
		limit = 100
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	comments := r.comments[actionCommentKey(workspaceID, actionID)]
	if len(comments) == 0 {
		return []*model.ActionComment{}, "", nil
	}

	sorted := make([]*model.ActionComment, len(comments))
	copy(sorted, comments)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})

	startIdx := 0
	if cursor != "" {
		found := -1
		for i, c := range sorted {
			if c.ID == cursor {
				found = i
				break
			}
		}
		if found < 0 {
			return []*model.ActionComment{}, "", nil
		}
		startIdx = found + 1
	}

	end := startIdx + limit
	hasMore := end < len(sorted)
	if end > len(sorted) {
		end = len(sorted)
	}

	result := make([]*model.ActionComment, 0, end-startIdx)
	for _, c := range sorted[startIdx:end] {
		result = append(result, copyActionComment(c))
	}

	var nextCursor string
	if hasMore && len(result) > 0 {
		nextCursor = result[len(result)-1].ID
	}
	return result, nextCursor, nil
}

func (r *actionCommentRepository) Delete(ctx context.Context, workspaceID string, actionID int64, commentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := actionCommentKey(workspaceID, actionID)
	existing := r.comments[key]
	for i, c := range existing {
		if c.ID == commentID {
			r.comments[key] = append(existing[:i:i], existing[i+1:]...)
			return nil
		}
	}
	return nil
}
