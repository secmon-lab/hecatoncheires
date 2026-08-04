package firestore

import (
	"context"
	"sort"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// jobSlotsCollection holds the execution slots of the deployment-wide
// concurrency limit. It is deliberately a top-level collection: the limit
// spans every workspace (it exists to protect one shared LLM quota), so
// nesting it under a workspace would be wrong.
const jobSlotsCollection = "jobSlots"

type jobSlotRepository struct {
	client *firestore.Client
}

var _ interfaces.JobSlotRepository = &jobSlotRepository{}

func newJobSlotRepository(client *firestore.Client) *jobSlotRepository {
	return &jobSlotRepository{client: client}
}

// doc returns the DocumentRef for one slot. The document ID is the decimal
// index, so the index is recoverable from the path alone.
func (r *jobSlotRepository) doc(index int) *firestore.DocumentRef {
	return r.client.Collection(jobSlotsCollection).Doc(strconv.Itoa(index))
}

// List scans the whole collection. No Where / OrderBy is used, so the query
// needs no composite index (see .claude/rules/firestore.md); ordering is done
// in memory over what is at most `limit` documents.
func (r *jobSlotRepository) List(ctx context.Context) ([]*model.JobSlot, error) {
	iter := r.client.Collection(jobSlotsCollection).Documents(ctx)
	defer iter.Stop()

	var out []*model.JobSlot
	for {
		snap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate job slots")
		}
		var slot model.JobSlot
		if err := snap.DataTo(&slot); err != nil {
			return nil, goerr.Wrap(err, "failed to decode job slot",
				goerr.V("doc_path", snap.Ref.Path))
		}
		// Index comes from the path. A non-numeric ID means someone wrote a
		// foreign document into the collection; fail loudly rather than
		// silently under-count the occupied slots.
		index, convErr := strconv.Atoi(snap.Ref.ID)
		if convErr != nil {
			return nil, goerr.Wrap(convErr, "job slot document id is not an index",
				goerr.V("doc_id", snap.Ref.ID))
		}
		slot.Index = index
		out = append(out, &slot)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, nil
}

func (r *jobSlotRepository) TryAcquire(ctx context.Context, slot *model.JobSlot, now time.Time) (bool, error) {
	if err := slot.Validate(); err != nil {
		return false, goerr.Wrap(err, "job slot validation failed before acquire")
	}
	docRef := r.doc(slot.Index)
	var acquired bool
	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		acquired = false
		snap, err := tx.Get(docRef)
		if err != nil && status.Code(err) != codes.NotFound {
			return goerr.Wrap(err, "tx get job slot")
		}
		if err == nil {
			var existing model.JobSlot
			if decErr := snap.DataTo(&existing); decErr != nil {
				return goerr.Wrap(decErr, "decode existing job slot")
			}
			if existing.IsHeld(now) {
				return nil
			}
		}
		if setErr := tx.Set(docRef, slot); setErr != nil {
			return goerr.Wrap(setErr, "tx set job slot")
		}
		acquired = true
		return nil
	})
	if err != nil {
		return false, goerr.Wrap(err, "TryAcquire job slot",
			goerr.V("index", slot.Index),
			goerr.V("holder_id", slot.HolderID))
	}
	return acquired, nil
}

func (r *jobSlotRepository) Renew(ctx context.Context, index int, holderID string, expiresAt time.Time) error {
	if holderID == "" {
		return goerr.New("holder id is empty")
	}
	docRef := r.doc(index)
	// The callback's error is returned as-is (already carrying context) so
	// errors.Is(err, ErrJobSlotNotHeld) holds for the caller.
	return r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(docRef)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return goerr.Wrap(interfaces.ErrJobSlotNotHeld, "job slot record is gone",
					goerr.V("index", index), goerr.V("holder_id", holderID))
			}
			return goerr.Wrap(err, "tx get job slot")
		}
		var slot model.JobSlot
		if decErr := snap.DataTo(&slot); decErr != nil {
			return goerr.Wrap(decErr, "decode existing job slot")
		}
		if slot.HolderID != holderID {
			return goerr.Wrap(interfaces.ErrJobSlotNotHeld, "job slot was taken over",
				goerr.V("index", index),
				goerr.V("holder_id", holderID),
				goerr.V("current_holder_id", slot.HolderID))
		}
		slot.Index = index
		slot.ExpiresAt = expiresAt
		// Validate before every write, Renew included: an expiry that is not
		// after AcquiredAt would persist a record violating the model's own
		// invariant (see .claude/rules/architecture.md § Repository write
		// contract).
		if vErr := slot.Validate(); vErr != nil {
			return goerr.Wrap(vErr, "job slot validation failed before renew",
				goerr.V("index", index), goerr.V("expires_at", expiresAt))
		}
		if setErr := tx.Set(docRef, &slot); setErr != nil {
			return goerr.Wrap(setErr, "tx set job slot")
		}
		return nil
	})
}

func (r *jobSlotRepository) Release(ctx context.Context, index int, holderID string) error {
	if holderID == "" {
		return goerr.New("holder id is empty")
	}
	docRef := r.doc(index)
	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(docRef)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return nil
			}
			return goerr.Wrap(err, "tx get job slot")
		}
		var slot model.JobSlot
		if decErr := snap.DataTo(&slot); decErr != nil {
			return goerr.Wrap(decErr, "decode existing job slot")
		}
		if slot.HolderID != holderID {
			// Taken over after an expiry — releasing must not evict the
			// current holder.
			return nil
		}
		if delErr := tx.Delete(docRef); delErr != nil {
			return goerr.Wrap(delErr, "tx delete job slot")
		}
		return nil
	})
	if err != nil {
		return goerr.Wrap(err, "Release job slot",
			goerr.V("index", index), goerr.V("holder_id", holderID))
	}
	return nil
}
