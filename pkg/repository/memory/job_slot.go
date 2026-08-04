package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// jobSlotRepository is the in-memory execution-slot store. The mutex gives
// each slot's transitions the same atomicity a Firestore transaction does, so
// two goroutines racing for one index cannot both win. Being process-local it
// bounds concurrency only within a single instance — production runs on
// Firestore (documented in docs/operations.md).
type jobSlotRepository struct {
	mu    sync.Mutex
	slots map[int]model.JobSlot
}

var _ interfaces.JobSlotRepository = &jobSlotRepository{}

func newJobSlotRepository() *jobSlotRepository {
	return &jobSlotRepository{
		slots: make(map[int]model.JobSlot),
	}
}

func (r *jobSlotRepository) List(_ context.Context) ([]*model.JobSlot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]*model.JobSlot, 0, len(r.slots))
	for _, s := range r.slots {
		copied := s
		out = append(out, &copied)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, nil
}

func (r *jobSlotRepository) TryAcquire(_ context.Context, slot *model.JobSlot, now time.Time) (bool, error) {
	if err := slot.Validate(); err != nil {
		return false, goerr.Wrap(err, "job slot validation failed before acquire")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.slots[slot.Index]; ok && existing.IsHeld(now) {
		return false, nil
	}
	r.slots[slot.Index] = *slot
	return true, nil
}

func (r *jobSlotRepository) Renew(_ context.Context, index int, holderID string, expiresAt time.Time) error {
	if holderID == "" {
		return goerr.New("holder id is empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.slots[index]
	if !ok || existing.HolderID != holderID {
		return goerr.Wrap(interfaces.ErrJobSlotNotHeld, "cannot renew job slot",
			goerr.V("index", index), goerr.V("holder_id", holderID))
	}
	// existing is a copy, so the stored record stays untouched when validation
	// rejects the new expiry. Validate before every write, Renew included (see
	// .claude/rules/architecture.md § Repository write contract).
	existing.Index = index
	existing.ExpiresAt = expiresAt
	if err := existing.Validate(); err != nil {
		return goerr.Wrap(err, "job slot validation failed before renew",
			goerr.V("index", index), goerr.V("expires_at", expiresAt))
	}
	r.slots[index] = existing
	return nil
}

func (r *jobSlotRepository) Release(_ context.Context, index int, holderID string) error {
	if holderID == "" {
		return goerr.New("holder id is empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.slots[index]
	if !ok || existing.HolderID != holderID {
		// Already free, or taken over after an expiry — never evict the
		// current holder.
		return nil
	}
	delete(r.slots, index)
	return nil
}
