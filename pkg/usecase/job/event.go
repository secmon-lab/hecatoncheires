// Package job hosts the event-driven Agent Job lifecycle: a usecase that
// publishes Events, matches them to workspace Jobs, and dispatches the
// Job runtime in a background goroutine. State that must survive across
// requests lives in the JobRunRepository (Firestore in production).
package job

import (
	"context"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// Event is the runtime payload published by the EventPublisher when a
// case-lifecycle transition or a scheduled tick occurs. The payload
// carries publish-time facts only; static config (cron schedule, prompts)
// is resolved from the Job model at prompt-build time.
type Event struct {
	Domain      model.JobEventDomain
	WorkspaceID string
	CaseID      int64
	Timestamp   time.Time
	ActorUserID string

	// Domain == case
	CaseLifecycle model.CaseLifecycle

	// Domain == scheduled
	LastRunAt    time.Time
	ScheduledFor time.Time
}

// EventPublisher is the dispatch entry point exposed to upstream callers
// (CaseUseCase, ScheduledScanner). Publish is non-blocking; the actual
// Job runs in a background goroutine via async.Dispatch so the calling
// request returns immediately.
type EventPublisher interface {
	Publish(ctx context.Context, ev Event)
}

// jobActorContextKey is the private context.Value key used to mark a
// mutation as originating from a Job's tool. The publisher consults
// this so a Job that runs case_writer / action_writer cannot publish a
// follow-up event for its own write — the canonical infinite-loop
// surface.
type jobActorContextKey struct{}

// JobActorMarker is the value stored under jobActorContextKey. It
// carries the originating job id for trace purposes; equality of the
// marker is what suppresses re-publication, not equality of the JobID.
type JobActorMarker struct {
	JobID string
}

// WithJobActor attaches the JobActorMarker to ctx so downstream code
// (tools, repos, then back to EventPublisher.Publish) can recognise
// the call as Job-originated and skip event re-publication.
func WithJobActor(ctx context.Context, marker JobActorMarker) context.Context {
	return context.WithValue(ctx, jobActorContextKey{}, marker)
}

// IsJobActorContext reports whether the context carries a JobActorMarker.
// EventPublisher implementations call this in the Publish hot path to
// short-circuit recursive dispatch.
func IsJobActorContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	_, ok := ctx.Value(jobActorContextKey{}).(JobActorMarker)
	return ok
}
