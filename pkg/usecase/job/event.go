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
// this so a Job that runs case_writer / action_writer cannot re-fire
// *itself* via a follow-up event for its own write — the canonical
// infinite-loop surface.
type jobActorContextKey struct{}

// JobActorMarker is the value stored under jobActorContextKey. It
// carries the originating job id, which the publisher compares against
// each candidate job: only the job whose ID matches is suppressed, so a
// Job whose agent triggers a different lifecycle event (e.g. an
// on-created Job that closes the case) still fires the other Jobs
// listening on it (e.g. the on-closed Job).
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
func IsJobActorContext(ctx context.Context) bool {
	_, ok := jobActorFromContext(ctx)
	return ok
}

// jobActorFromContext returns the originating JobActorMarker and whether
// one is present. EventPublisher.Publish uses the marker's JobID to
// suppress only the originating Job's own re-fire, not every Job.
func jobActorFromContext(ctx context.Context) (JobActorMarker, bool) {
	if ctx == nil {
		return JobActorMarker{}, false
	}
	marker, ok := ctx.Value(jobActorContextKey{}).(JobActorMarker)
	return marker, ok
}

// jobQuietContextKey is the private context.Value key carrying the
// per-run "quiet" flag. JobRunner.Run stamps it once at the top of a run
// (mirroring WithJobActor) so the various operational-log post sites
// (starting marker, session-log thread, completion marker, tool-progress
// trace handler) can each self-gate via isQuiet without threading the
// flag through their constructors. It is deliberately unexported: the
// boundary that "quiet does NOT silence the agent's slack__post_message
// tool" is enforced structurally — that tool lives in another package and
// cannot read this key.
type jobQuietContextKey struct{}

// withQuiet returns a context carrying the given quiet flag.
func withQuiet(ctx context.Context, quiet bool) context.Context {
	return context.WithValue(ctx, jobQuietContextKey{}, quiet)
}

// isQuiet reports whether the context was marked quiet via withQuiet.
// Absent marker (or nil ctx) means not quiet.
func isQuiet(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	q, _ := ctx.Value(jobQuietContextKey{}).(bool)
	return q
}
