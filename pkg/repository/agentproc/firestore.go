// Package agentproc implements agentkit.Repository on Firestore. It is the
// durable store behind the agent runtime: one document per agentkit Process,
// with awaits and events as subcollections, plus two guard collections that
// carry the uniqueness constraints the kernel relies on.
//
// It is a separate package from pkg/repository/firestore because it implements
// a different contract (agentkit's SPI, not interfaces.Repository) and is
// consumed by the kernel rather than by the usecase layer.
package agentproc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"maps"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gollem-dev/agentkit"
	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	processesCollection = "agentProcesses"
	awaitsCollection    = "awaits"
	eventsCollection    = "events"
	keysCollection      = "agentProcessKeys"
	subjectsCollection  = "agentProcessSubjects"
	subjectRefsSub      = "refs"
)

// claimPageSize is how many candidates one query page carries. It bounds memory
// per page, NOT how many rows a claim will inspect: the scan pages on until it
// takes a row or the eligible set is exhausted.
//
// Capping the total would break the contract. Concurrent claimers all read the
// same ordered head, so a claimer that gave up after N candidates would report
// "nothing runnable" while a backlog of runnable rows sat behind the N its peers
// had just taken. The scan is bounded in practice by the eligible set itself:
// a claimed row's ClaimAt jumps to its lease expiry, so it leaves the set at
// once.
const claimPageSize = 32

// claimNever marks a row that no passage of time makes claimable: a terminal
// Process, or one waiting for a response that will never arrive on its own.
//
// A far-future timestamp is used rather than a null because Firestore's
// inequality filters compare ACROSS types by type order — a null-valued field
// would satisfy "ClaimAt <= now" and hand the claim query rows it must never
// see. Keeping every value a timestamp makes the comparison mean what it says.
var claimNever = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

// claimImmediately marks a row that is runnable whenever a worker asks: a
// pending Process with no wake time, or a running one whose lease is absent.
//
// It is the zero time rather than the row's own CreatedAt/UpdatedAt because the
// caller supplies its own `now`, which can precede a row written moments ago.
// Deriving the claim time from the row would make a freshly inserted Process
// unclaimable to a worker whose clock is a few milliseconds behind, which the
// contract does not allow. Ordering is not lost: ties on ClaimAt break on the
// document id, and those are uuid v7, so the oldest Process still goes first.
var claimImmediately = time.Time{}

// processRow is the stored shape of a Process: the agentkit value nested whole,
// plus the one derived value the claim query ranges over.
//
// It is NOT the mirror-"doc" pattern the repository rules ban. That pattern
// enumerates a model's fields into a parallel struct, which silently drops any
// field the model gains later; this type enumerates nothing — the Process rides
// along as a single nested value, so it cannot lose a field. The wrapper exists
// only because ClaimAt has to be written in the same document as the Process to
// stay atomic with it, and Firestore has no computed fields.
type processRow struct {
	Process agentkit.Process
	ClaimAt time.Time
	// EventSeq is how many events this Process has appended. It is the source of
	// the per-event Seq below, and it advances in the same transaction as the
	// events it numbers.
	EventSeq int64
}

// eventRow is the stored shape of an Event: the agentkit value nested whole,
// plus the append position.
//
// The position cannot be derived from the EventID. IDs are uuid v7, and this
// codebase already records what that costs — interfaces.JobRunEventRepository
// says in as many words that uuid v7 doc ids "may diverge under clock skew" and
// orders its own timeline on a sequence instead. A Process moves between
// instances between claims, so ordering these on the id would reorder the
// timeline whenever two machines' clocks disagree, and agentkit's Repository
// contract asks for append order specifically.
type eventRow struct {
	Event agentkit.Event
	Seq   int64
}

// keyGuard enforces idempotency-key uniqueness. It is created with the Process
// and never deleted: the key must keep deduplicating after the Process ends.
type keyGuard struct {
	Key       string
	ProcessID agentkit.ProcessID
}

// subjectGuard enforces "at most one open Process per Subject". It is created
// with the Process and deleted by the commit that makes the Process terminal,
// which is what releases the subject.
type subjectGuard struct {
	Kind      string
	ID        string
	ProcessID agentkit.ProcessID
}

// Repository is the Firestore-backed agentkit.Repository.
type Repository struct {
	client *firestore.Client
	// owned reports whether this Repository created the client and must close
	// it. A Repository built from a caller-supplied client leaves it alone.
	owned bool
}

var _ agentkit.Repository = (*Repository)(nil)

// New builds a Repository over an existing Firestore client. Closing the
// Repository does not close the client.
func New(client *firestore.Client) (*Repository, error) {
	if client == nil {
		return nil, goerr.New("firestore client is required")
	}
	return &Repository{client: client}, nil
}

// NewWithProject builds a Repository with its own Firestore client. The caller
// must Close it.
func NewWithProject(ctx context.Context, projectID, databaseID string) (*Repository, error) {
	if projectID == "" {
		return nil, goerr.New("firestore project id is required")
	}
	var client *firestore.Client
	var err error
	if databaseID != "" {
		client, err = firestore.NewClientWithDatabase(ctx, projectID, databaseID)
	} else {
		client, err = firestore.NewClient(ctx, projectID)
	}
	if err != nil {
		return nil, goerr.Wrap(err, "create firestore client for agent processes",
			goerr.V("project_id", projectID), goerr.V("database_id", databaseID))
	}
	return &Repository{client: client, owned: true}, nil
}

// Close releases the Firestore client when this Repository created it.
func (r *Repository) Close() error {
	if r == nil || !r.owned {
		return nil
	}
	if err := r.client.Close(); err != nil {
		return goerr.Wrap(err, "close firestore client for agent processes")
	}
	return nil
}

func (r *Repository) processRef(pid agentkit.ProcessID) *firestore.DocumentRef {
	return r.client.Collection(processesCollection).Doc(string(pid))
}

func (r *Repository) awaitRef(pid agentkit.ProcessID, key agentkit.AwaitKey) *firestore.DocumentRef {
	return r.processRef(pid).Collection(awaitsCollection).Doc(hashID(string(key)))
}

func (r *Repository) eventRef(pid agentkit.ProcessID, id agentkit.EventID) *firestore.DocumentRef {
	return r.processRef(pid).Collection(eventsCollection).Doc(string(id))
}

func (r *Repository) keyRef(key string) *firestore.DocumentRef {
	return r.client.Collection(keysCollection).Doc(hashID(key))
}

func (r *Repository) subjectRef(s agentkit.SubjectRef) *firestore.DocumentRef {
	return r.client.Collection(subjectsCollection).Doc(hashID(s.Kind)).
		Collection(subjectRefsSub).Doc(hashID(s.ID))
}

// hashID turns a caller-supplied identifier into a Firestore-safe document ID.
// Await keys, idempotency keys and subject ids are all composed by the
// application, so none of them is guaranteed to satisfy Firestore's document-id
// rules (no "/", not "." or "..", at most 1500 bytes). The raw value is always
// stored in a field as well, so nothing becomes unreadable.
func hashID(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// claimAtFor computes the instant from which a Process becomes claimable. It is
// recomputed on every write, so the stored value always matches the row.
func claimAtFor(p *agentkit.Process) time.Time {
	switch {
	case p.Status.Terminal():
		return claimNever
	case p.Status == agentkit.ProcessRunning:
		// An expired OR absent lease means the previous claim died
		// mid-transition and the row is reclaimable (Repository contract 4).
		if p.LeaseUntil == nil {
			return claimImmediately
		}
		return *p.LeaseUntil
	case p.Status == agentkit.ProcessWaiting:
		// A waiting row without a deadline is waiting for a response and must
		// never wake by itself.
		if p.WakeAt == nil {
			return claimNever
		}
		return *p.WakeAt
	default: // pending
		if p.WakeAt != nil {
			return *p.WakeAt // the worker's retry backoff
		}
		return claimImmediately
	}
}

// GetProcess returns the Process. Absent -> ErrProcessNotFound.
func (r *Repository) GetProcess(ctx context.Context, pid agentkit.ProcessID) (*agentkit.Process, error) {
	snap, err := r.processRef(pid).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(agentkit.ErrProcessNotFound, "no such process", goerr.V("process", pid))
		}
		return nil, goerr.Wrap(err, "get agent process", goerr.V("process", pid))
	}
	return decodeProcess(snap)
}

func decodeProcess(snap *firestore.DocumentSnapshot) (*agentkit.Process, error) {
	var doc processRow
	if err := snap.DataTo(&doc); err != nil {
		return nil, goerr.Wrap(err, "decode agent process", goerr.V("doc_id", snap.Ref.ID))
	}
	return &doc.Process, nil
}

// FindProcessByIdempotencyKey resolves the guard document and reads the Process
// it names. Absent -> ErrProcessNotFound.
func (r *Repository) FindProcessByIdempotencyKey(ctx context.Context, key string) (*agentkit.Process, error) {
	snap, err := r.keyRef(key).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(agentkit.ErrProcessNotFound, "no process for idempotency key")
		}
		return nil, goerr.Wrap(err, "get idempotency key guard")
	}
	var g keyGuard
	if err := snap.DataTo(&g); err != nil {
		return nil, goerr.Wrap(err, "decode idempotency key guard")
	}
	return r.GetProcess(ctx, g.ProcessID)
}

// FindOpenProcessBySubject resolves the subject guard and reads the Process it
// names. The guard is deleted by the commit that makes the Process terminal, so
// its presence means "open"; a terminal Process found through a guard that
// somehow survived is reported as absent rather than as an open holder.
func (r *Repository) FindOpenProcessBySubject(ctx context.Context, subject agentkit.SubjectRef) (*agentkit.Process, error) {
	snap, err := r.subjectRef(subject).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(agentkit.ErrProcessNotFound, "no open process for subject",
				goerr.V("subject_kind", subject.Kind))
		}
		return nil, goerr.Wrap(err, "get subject guard", goerr.V("subject_kind", subject.Kind))
	}
	var g subjectGuard
	if err := snap.DataTo(&g); err != nil {
		return nil, goerr.Wrap(err, "decode subject guard")
	}
	proc, err := r.GetProcess(ctx, g.ProcessID)
	if err != nil {
		return nil, err
	}
	if proc.Status.Terminal() {
		return nil, goerr.Wrap(agentkit.ErrProcessNotFound, "subject guard names a terminal process",
			goerr.V("process", proc.ID), goerr.V("status", proc.Status))
	}
	return proc, nil
}

// ClaimNextProcess atomically claims one runnable Process. It ranges over the
// derived ClaimAt field, then re-checks and writes each candidate inside its own
// transaction, so two workers reading the same candidate list cannot both win.
func (r *Repository) ClaimNextProcess(ctx context.Context, workerID string, leaseUntil, now time.Time) (*agentkit.Process, error) {
	base := r.client.Collection(processesCollection).
		Where("ClaimAt", "<=", now).
		OrderBy("ClaimAt", firestore.Asc).
		OrderBy(firestore.DocumentID, firestore.Asc)

	var cursor *firestore.DocumentSnapshot
	for {
		query := base.Limit(claimPageSize)
		if cursor != nil {
			query = query.StartAfter(cursor)
		}
		snaps, err := query.Documents(ctx).GetAll()
		if err != nil {
			return nil, goerr.Wrap(err, "query claimable agent processes")
		}
		if len(snaps) == 0 {
			return nil, nil
		}
		for _, snap := range snaps {
			claimed, err := r.claimOne(ctx, snap.Ref, workerID, leaseUntil, now)
			if err != nil {
				return nil, err
			}
			if claimed != nil {
				return claimed, nil
			}
			// The candidate was taken or is no longer eligible; try the next.
		}
		if len(snaps) < claimPageSize {
			return nil, nil
		}
		cursor = snaps[len(snaps)-1]
	}
}

// claimOne re-reads a candidate under a transaction and takes it when it is
// still eligible. It returns (nil, nil) when the row was taken by someone else
// or has moved on, which is the signal to try the next candidate.
func (r *Repository) claimOne(ctx context.Context, ref *firestore.DocumentRef, workerID string, leaseUntil, now time.Time) (*agentkit.Process, error) {
	var claimed *agentkit.Process
	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		claimed = nil
		snap, err := tx.Get(ref)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return nil
			}
			return goerr.Wrap(err, "tx get claim candidate", goerr.V("doc_id", ref.ID))
		}
		var doc processRow
		if err := snap.DataTo(&doc); err != nil {
			return goerr.Wrap(err, "decode claim candidate", goerr.V("doc_id", ref.ID))
		}
		if claimAtFor(&doc.Process).After(now) {
			return nil // no longer eligible
		}

		// Taking over a running row means the previous claim died
		// mid-transition. That count is what bounds re-execution after a crash;
		// a claim from pending or waiting leaves it alone, and no claim ever
		// touches StepAttempts.
		wasRunning := doc.Process.Status == agentkit.ProcessRunning

		p := doc.Process
		p.Status = agentkit.ProcessRunning
		p.LeaseOwner = workerID
		// A fresh token on EVERY claim, re-claims included: it is the fence
		// identity a worker uses to tell "I still hold this" from "someone took
		// it".
		p.LeaseToken = uuid.Must(uuid.NewV7()).String()
		p.LeaseUntil = &leaseUntil
		p.WakeAt = nil
		p.UpdatedAt = now
		p.Rev = doc.Process.Rev + 1
		if wasRunning {
			p.UncleanReclaims = doc.Process.UncleanReclaims + 1
		}

		if err := tx.Set(ref, processRow{Process: p, ClaimAt: claimAtFor(&p)}); err != nil {
			return goerr.Wrap(err, "tx set claimed process", goerr.V("doc_id", ref.ID))
		}
		claimed = &p
		return nil
	})
	if err != nil {
		// A losing racer sees Aborted after the client's own retries; that is
		// "someone else took it", not a failure to report.
		if status.Code(err) == codes.Aborted {
			return nil, nil
		}
		return nil, goerr.Wrap(err, "claim agent process", goerr.V("doc_id", ref.ID))
	}
	return claimed, nil
}

// ListAwaits returns all awaits of a Process.
func (r *Repository) ListAwaits(ctx context.Context, pid agentkit.ProcessID) ([]*agentkit.Await, error) {
	snaps, err := r.processRef(pid).Collection(awaitsCollection).Documents(ctx).GetAll()
	if err != nil {
		return nil, goerr.Wrap(err, "list agent process awaits", goerr.V("process", pid))
	}
	out := make([]*agentkit.Await, 0, len(snaps))
	for _, snap := range snaps {
		var aw agentkit.Await
		if err := snap.DataTo(&aw); err != nil {
			return nil, goerr.Wrap(err, "decode await", goerr.V("doc_id", snap.Ref.ID))
		}
		out = append(out, &aw)
	}
	return out, nil
}

// ListEvents returns a Process's events in append order, which is the stored
// Seq — not the document id. See eventRow for why the id cannot stand in for it.
func (r *Repository) ListEvents(ctx context.Context, pid agentkit.ProcessID, q agentkit.EventQuery) ([]*agentkit.Event, error) {
	coll := r.processRef(pid).Collection(eventsCollection)
	query := coll.OrderBy("Seq", firestore.Asc)

	if q.After != "" {
		// A cursor the Process has no event for means the caller's stored
		// position went stale. Returning the whole list instead would reach it
		// as a burst of new events it cannot tell from the real thing.
		afterSnap, err := coll.Doc(string(q.After)).Get(ctx)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return nil, goerr.Wrap(agentkit.ErrEventNotFound, "no such event for cursor",
					goerr.V("process", pid), goerr.V("after", q.After))
			}
			return nil, goerr.Wrap(err, "get event cursor", goerr.V("process", pid))
		}
		var after eventRow
		if err := afterSnap.DataTo(&after); err != nil {
			return nil, goerr.Wrap(err, "decode event cursor", goerr.V("process", pid))
		}
		query = query.Where("Seq", ">", after.Seq)
	}
	if q.Limit > 0 {
		query = query.Limit(q.Limit)
	}

	snaps, err := query.Documents(ctx).GetAll()
	if err != nil {
		return nil, goerr.Wrap(err, "list agent process events", goerr.V("process", pid))
	}
	out := make([]*agentkit.Event, 0, len(snaps))
	for _, snap := range snaps {
		var row eventRow
		if err := snap.DataTo(&row); err != nil {
			return nil, goerr.Wrap(err, "decode event", goerr.V("doc_id", snap.Ref.ID))
		}
		ev := row.Event
		out = append(out, &ev)
	}
	return out, nil
}

// Apply applies the whole ChangeSet atomically. Every precondition is evaluated
// from the transaction's own reads, so a violation writes nothing and returns
// ErrConflict.
func (r *Repository) Apply(ctx context.Context, cs agentkit.ChangeSet) error {
	// Two inserts in the SAME ChangeSet can collide on a uniqueness constraint,
	// and the transaction's reads cannot see that: neither row is stored yet, so
	// both would pass the stored-state check and the second Set would silently
	// overwrite the first guard. Reject it before opening the transaction.
	if err := checkChangeSetUniqueness(cs); err != nil {
		return err
	}

	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// --- read phase (Firestore requires every read before any write) ---
		refs, order := r.collectReadRefs(cs)
		snaps := map[string]*firestore.DocumentSnapshot{}
		if len(refs) > 0 {
			got, err := tx.GetAll(refs)
			if err != nil {
				return goerr.Wrap(err, "tx get change set documents")
			}
			for i, snap := range got {
				snaps[order[i]] = snap
			}
		}

		// --- precondition phase ---
		for _, g := range cs.Guards {
			snap := snaps[r.processRef(g.ProcessID).Path]
			if snap == nil || !snap.Exists() {
				return goerr.Wrap(agentkit.ErrConflict, "guarded process does not exist",
					goerr.V("process", g.ProcessID))
			}
			var doc processRow
			if err := snap.DataTo(&doc); err != nil {
				return goerr.Wrap(err, "decode guarded process", goerr.V("process", g.ProcessID))
			}
			if doc.Process.Rev != g.Rev {
				return goerr.Wrap(agentkit.ErrConflict, "guard revision mismatch",
					goerr.V("process", g.ProcessID),
					goerr.V("want_rev", g.Rev), goerr.V("stored_rev", doc.Process.Rev))
			}
		}

		for _, p := range cs.Processes {
			snap := snaps[r.processRef(p.ID).Path]
			exists := snap != nil && snap.Exists()
			if p.Rev == 0 {
				if exists {
					return goerr.Wrap(agentkit.ErrConflict, "process already exists",
						goerr.V("process", p.ID))
				}
				continue
			}
			if !exists {
				return goerr.Wrap(agentkit.ErrConflict, "process does not exist",
					goerr.V("process", p.ID), goerr.V("want_rev", p.Rev))
			}
			var row processRow
			if err := snap.DataTo(&row); err != nil {
				return goerr.Wrap(err, "decode process for revision check", goerr.V("process", p.ID))
			}
			if row.Process.Rev != p.Rev {
				return goerr.Wrap(agentkit.ErrConflict, "process revision mismatch",
					goerr.V("process", p.ID),
					goerr.V("want_rev", p.Rev), goerr.V("stored_rev", row.Process.Rev))
			}
		}

		for _, p := range cs.Processes {
			if p.Rev != 0 {
				continue
			}
			if p.IdempotencyKey != "" {
				if snap := snaps[r.keyRef(p.IdempotencyKey).Path]; snap != nil && snap.Exists() {
					return goerr.Wrap(agentkit.ErrConflict, "idempotency key already taken",
						goerr.V("process", p.ID))
				}
			}
			if p.Subject != nil {
				if snap := snaps[r.subjectRef(*p.Subject).Path]; snap != nil && snap.Exists() {
					return goerr.Wrap(agentkit.ErrConflict, "subject already held",
						goerr.V("process", p.ID), goerr.V("subject_kind", p.Subject.Kind))
				}
			}
		}

		// Number this change set's events, continuing each Process's own count.
		// The numbers are handed out in ChangeSet order, which is the append
		// order the Repository contract asks ListEvents to reproduce.
		seqBase, err := r.eventSeqBases(cs, snaps, r.processRef)
		if err != nil {
			return err
		}
		seqNext := maps.Clone(seqBase)

		// --- write phase ---
		written := map[agentkit.ProcessID]struct{}{}
		for _, p := range cs.Processes {
			next := *p
			next.Rev = p.Rev + 1
			row := processRow{Process: next, ClaimAt: claimAtFor(&next), EventSeq: eventSeqAfter(cs, p.ID, seqBase)}
			if err := tx.Set(r.processRef(p.ID), row); err != nil {
				return goerr.Wrap(err, "tx set process", goerr.V("process", p.ID))
			}
			written[p.ID] = struct{}{}
			if p.Rev == 0 {
				if p.IdempotencyKey != "" {
					if err := tx.Set(r.keyRef(p.IdempotencyKey), keyGuard{Key: p.IdempotencyKey, ProcessID: p.ID}); err != nil {
						return goerr.Wrap(err, "tx set idempotency guard", goerr.V("process", p.ID))
					}
				}
				if p.Subject != nil {
					if err := tx.Set(r.subjectRef(*p.Subject), subjectGuard{
						Kind: p.Subject.Kind, ID: p.Subject.ID, ProcessID: p.ID,
					}); err != nil {
						return goerr.Wrap(err, "tx set subject guard", goerr.V("process", p.ID))
					}
				}
			}
			// Reaching a terminal state releases the subject. Deleting an
			// already-absent document is not an error, so re-applying the same
			// terminal row is harmless.
			if p.Subject != nil && p.Status.Terminal() {
				if err := tx.Delete(r.subjectRef(*p.Subject)); err != nil {
					return goerr.Wrap(err, "tx delete subject guard", goerr.V("process", p.ID))
				}
			}
		}

		for _, aw := range cs.Awaits {
			if err := tx.Set(r.awaitRef(aw.ProcessID, aw.Key), aw); err != nil {
				return goerr.Wrap(err, "tx set await",
					goerr.V("process", aw.ProcessID), goerr.V("await_key", aw.Key))
			}
		}
		for _, ev := range cs.Events {
			seq := seqNext[ev.ProcessID]
			seqNext[ev.ProcessID] = seq + 1
			// Create, not Set: an event is an append. Re-using an id would
			// otherwise replace an event a caller may already hold as a cursor,
			// changing what that cursor resumes from.
			if err := tx.Create(r.eventRef(ev.ProcessID, ev.ID), eventRow{Event: *ev, Seq: seq}); err != nil {
				return goerr.Wrap(err, "tx create event",
					goerr.V("process", ev.ProcessID), goerr.V("event", ev.ID))
			}
		}

		// A Process whose events were appended without its row being rewritten
		// still has to record how far its numbering got. Update touches only that
		// field, so the Process keeps its Rev — the caller did not ask for a
		// revision bump and a CAS the kernel did not expect would fence it out.
		for pid, next := range seqNext {
			if _, ok := written[pid]; ok {
				continue
			}
			if next == seqBase[pid] {
				continue
			}
			if err := tx.Update(r.processRef(pid), []firestore.Update{
				{Path: "EventSeq", Value: next},
			}); err != nil {
				return goerr.Wrap(err, "tx advance event sequence", goerr.V("process", pid))
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, agentkit.ErrConflict) {
			return err
		}
		// A uniqueness violation that slipped past the read phase (two inserts
		// racing on the same key) surfaces here. The kernel discriminates on
		// ErrConflict, so it must not reach it as a bare gRPC status.
		switch status.Code(err) {
		case codes.AlreadyExists, codes.Aborted, codes.FailedPrecondition:
			return goerr.Wrap(agentkit.ErrConflict, "change set conflicted")
		}
		return goerr.Wrap(err, "apply agent process change set")
	}
	return nil
}

// eventSeqBases reads, for every Process this ChangeSet appends events to, the
// number of events it has already appended.
//
// An event for a Process that is neither stored nor being inserted here has
// nothing to be appended to, and numbering it would put it in an order nobody
// can reproduce, so it is a conflict rather than a silent write.
func (r *Repository) eventSeqBases(
	cs agentkit.ChangeSet,
	snaps map[string]*firestore.DocumentSnapshot,
	refOf func(agentkit.ProcessID) *firestore.DocumentRef,
) (map[agentkit.ProcessID]int64, error) {
	inserting := map[agentkit.ProcessID]struct{}{}
	for _, p := range cs.Processes {
		if p.Rev == 0 {
			inserting[p.ID] = struct{}{}
		}
	}

	bases := map[agentkit.ProcessID]int64{}
	for _, ev := range cs.Events {
		if _, seen := bases[ev.ProcessID]; seen {
			continue
		}
		snap := snaps[refOf(ev.ProcessID).Path]
		if snap == nil || !snap.Exists() {
			if _, ok := inserting[ev.ProcessID]; ok {
				bases[ev.ProcessID] = 0
				continue
			}
			return nil, goerr.Wrap(agentkit.ErrConflict, "event for a process that does not exist",
				goerr.V("process", ev.ProcessID), goerr.V("event", ev.ID))
		}
		var row processRow
		if err := snap.DataTo(&row); err != nil {
			return nil, goerr.Wrap(err, "decode process for event numbering",
				goerr.V("process", ev.ProcessID))
		}
		bases[ev.ProcessID] = row.EventSeq
	}
	return bases, nil
}

// eventSeqAfter is the event count a Process row carries once this ChangeSet's
// events are appended.
func eventSeqAfter(cs agentkit.ChangeSet, pid agentkit.ProcessID, bases map[agentkit.ProcessID]int64) int64 {
	next := bases[pid]
	for _, ev := range cs.Events {
		if ev.ProcessID == pid {
			next++
		}
	}
	return next
}

// checkChangeSetUniqueness rejects a ChangeSet whose own inserts collide on an
// idempotency key or a subject. Only inserts (Rev == 0) claim a guard, so an
// update carrying the same key as the row it updates is not a collision.
func checkChangeSetUniqueness(cs agentkit.ChangeSet) error {
	keys := map[string]agentkit.ProcessID{}
	subjects := map[agentkit.SubjectRef]agentkit.ProcessID{}
	for _, p := range cs.Processes {
		if p.Rev != 0 {
			continue
		}
		if p.IdempotencyKey != "" {
			if other, dup := keys[p.IdempotencyKey]; dup {
				return goerr.Wrap(agentkit.ErrConflict, "duplicate idempotency key within one change set",
					goerr.V("process", p.ID), goerr.V("other_process", other))
			}
			keys[p.IdempotencyKey] = p.ID
		}
		if p.Subject != nil {
			if other, dup := subjects[*p.Subject]; dup {
				return goerr.Wrap(agentkit.ErrConflict, "duplicate subject within one change set",
					goerr.V("process", p.ID), goerr.V("other_process", other),
					goerr.V("subject_kind", p.Subject.Kind))
			}
			subjects[*p.Subject] = p.ID
		}
	}
	return nil
}

// collectReadRefs returns the documents Apply must read before it writes, plus
// their paths in the same order so the results can be indexed back.
func (r *Repository) collectReadRefs(cs agentkit.ChangeSet) ([]*firestore.DocumentRef, []string) {
	var refs []*firestore.DocumentRef
	var order []string
	seen := map[string]struct{}{}
	add := func(ref *firestore.DocumentRef) {
		if _, ok := seen[ref.Path]; ok {
			return
		}
		seen[ref.Path] = struct{}{}
		refs = append(refs, ref)
		order = append(order, ref.Path)
	}
	for _, g := range cs.Guards {
		add(r.processRef(g.ProcessID))
	}
	for _, p := range cs.Processes {
		add(r.processRef(p.ID))
		if p.Rev != 0 {
			continue
		}
		if p.IdempotencyKey != "" {
			add(r.keyRef(p.IdempotencyKey))
		}
		if p.Subject != nil {
			add(r.subjectRef(*p.Subject))
		}
	}
	// An event's Process row carries the append counter, and a ChangeSet may
	// append events without rewriting that row, so it has to be read too.
	for _, ev := range cs.Events {
		add(r.processRef(ev.ProcessID))
	}
	return refs, order
}
