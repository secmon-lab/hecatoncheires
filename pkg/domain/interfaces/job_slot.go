package interfaces

import (
	"context"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// ErrJobSlotNotHeld is returned by JobSlotRepository.Renew when the record for
// the index is gone or owned by a different holder — the caller's slot expired
// and was taken over. Callers must discriminate with errors.Is.
var ErrJobSlotNotHeld = goerr.New("job slot is not held by this holder")

// JobSlotRepository persists the execution slots backing the deployment-wide
// concurrency limit on scheduled Job runs (see model.JobSlot).
//
// A free slot has NO record: TryAcquire creates it and Release deletes it, so
// the stored set is exactly the set of occupied slots. Backends must serialise
// each slot's transitions per record (Firestore RunTransaction, in-memory
// mutex) so two acquirers racing for the same index cannot both win.
type JobSlotRepository interface {
	// List returns every stored slot record in ascending Index order. The
	// caller derives "free" from an Index that has no record and from
	// model.JobSlot.IsHeld, so an expired record may be returned.
	List(ctx context.Context) ([]*model.JobSlot, error)

	// TryAcquire claims slot.Index for slot.HolderID when the stored record
	// is absent or no longer IsHeld(now). Returns false when a live holder
	// is present. The record is validated before the write.
	TryAcquire(ctx context.Context, slot *model.JobSlot, now time.Time) (acquired bool, err error)

	// Renew pushes ExpiresAt forward while holderID still owns index. It
	// returns ErrJobSlotNotHeld when the record is absent or held by another
	// holder; it never (re-)creates a record, so a released slot stays free.
	Renew(ctx context.Context, index int, holderID string, expiresAt time.Time) error

	// Release deletes the record when holderID owns it. Idempotent: an
	// absent record, or one taken over by another holder, is a no-op rather
	// than an error — releasing must never evict the new holder.
	Release(ctx context.Context, index int, holderID string) error
}
