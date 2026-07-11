package memory

import (
	"context"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
)

type reactionClaimRepository struct {
	mu     sync.Mutex
	claims map[string]struct{}
}

var _ interfaces.ReactionClaimRepository = &reactionClaimRepository{}

func newReactionClaimRepository() *reactionClaimRepository {
	return &reactionClaimRepository{claims: make(map[string]struct{})}
}

// reactionClaimKey joins the identity with NUL separators so distinct triples
// can never collide even if an ID happens to contain the separator character.
func reactionClaimKey(workspaceID, sourceChannelID, sourceMessageTS string) string {
	return workspaceID + "\x00" + sourceChannelID + "\x00" + sourceMessageTS
}

func (r *reactionClaimRepository) Claim(_ context.Context, workspaceID, sourceChannelID, sourceMessageTS string) (bool, error) {
	if workspaceID == "" || sourceChannelID == "" || sourceMessageTS == "" {
		return false, goerr.New("workspaceID, sourceChannelID and sourceMessageTS are required")
	}
	key := reactionClaimKey(workspaceID, sourceChannelID, sourceMessageTS)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.claims[key]; ok {
		return false, nil
	}
	r.claims[key] = struct{}{}
	return true, nil
}

func (r *reactionClaimRepository) Release(_ context.Context, workspaceID, sourceChannelID, sourceMessageTS string) error {
	if workspaceID == "" || sourceChannelID == "" || sourceMessageTS == "" {
		return goerr.New("workspaceID, sourceChannelID and sourceMessageTS are required")
	}
	key := reactionClaimKey(workspaceID, sourceChannelID, sourceMessageTS)
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.claims, key)
	return nil
}
