package model

import (
	"time"

	"github.com/m-mizutani/goerr/v2"
)

// JobSlot is one execution slot of the deployment-wide concurrency limit
// applied to scheduled Job runs. The limit N is expressed as N slot records
// (Index 0..N-1): a stored record whose ExpiresAt is still in the future
// means "occupied", and its absence (or an elapsed ExpiresAt) means "free".
//
// Representing the limit as N records rather than a counter means the number
// of occupied records IS the number of in-flight runs — there is no counter to
// drift out of sync when an increment or decrement is lost.
//
// The holder extends ExpiresAt periodically while its run is alive, so a
// crashed instance's slot frees itself within one TTL and no cleanup sweep is
// needed.
type JobSlot struct {
	// Index is the slot number within [0, limit). It is also the storage
	// document ID, so the persistence layer restores it from the path.
	Index int

	// HolderID is an opaque token identifying the current holder, generated
	// fresh for every acquisition. Renew / Release only act when it matches,
	// so a run that lost its slot to an expiry cannot clobber the new holder.
	HolderID string

	// WorkspaceID / CaseID / JobID name the run holding the slot. They are
	// carried for observability only — the gate itself does not read them.
	WorkspaceID string
	CaseID      int64
	JobID       string

	AcquiredAt time.Time
	// ExpiresAt is the wall-clock time after which the slot counts as free.
	// The holder's heartbeat pushes it forward; it is never consulted for
	// anything but the free / occupied decision.
	ExpiresAt time.Time
}

// IsHeld reports whether the slot is occupied by a live holder at now. A slot
// whose ExpiresAt is exactly now counts as free (the boundary favours reuse,
// matching the "free within one TTL" contract).
func (s *JobSlot) IsHeld(now time.Time) bool {
	if s == nil {
		return false
	}
	return s.HolderID != "" && s.ExpiresAt.After(now)
}

// Validate enforces the invariants of an occupied slot record. Repositories
// call it before every write so an incomplete record never reaches storage.
func (s *JobSlot) Validate() error {
	if s == nil {
		return goerr.New("job slot is nil")
	}
	if s.Index < 0 {
		return goerr.New("job slot index is negative", goerr.V("index", s.Index))
	}
	if s.HolderID == "" {
		return goerr.New("job slot holder id is empty", goerr.V("index", s.Index))
	}
	if s.WorkspaceID == "" {
		return goerr.New("job slot workspace id is empty", goerr.V("index", s.Index))
	}
	if s.CaseID == 0 {
		return goerr.New("job slot case id is zero", goerr.V("index", s.Index))
	}
	if s.JobID == "" {
		return goerr.New("job slot job id is empty", goerr.V("index", s.Index))
	}
	if s.AcquiredAt.IsZero() {
		return goerr.New("job slot acquired_at is zero", goerr.V("index", s.Index))
	}
	if !s.ExpiresAt.After(s.AcquiredAt) {
		return goerr.New("job slot expires_at must be after acquired_at",
			goerr.V("index", s.Index),
			goerr.V("acquired_at", s.AcquiredAt),
			goerr.V("expires_at", s.ExpiresAt))
	}
	return nil
}
