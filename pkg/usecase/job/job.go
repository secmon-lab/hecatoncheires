package job

import (
	"context"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// UseCase is the host-side dispatcher for Job events. It is the
// EventPublisher implementation upstream callers (CaseUseCase,
// ScheduledScanner) target. Each Publish call matches the event against
// every workspace Job and fires the matching ones in async goroutines.
type UseCase struct {
	registry *model.WorkspaceRegistry
	runner   *JobRunner
}

// NewUseCase wires the JobUseCase with the workspace registry and a
// pre-built runner. The runner is built outside this package because
// it needs the LLM / executor / tool builder, which are wired at main.
func NewUseCase(registry *model.WorkspaceRegistry, runner *JobRunner) *UseCase {
	return &UseCase{registry: registry, runner: runner}
}

// Publish is the EventPublisher implementation. It is non-blocking: each
// matching (job, case) pair runs in its own async.Dispatch goroutine, so
// the caller (typically inside a usecase mutation) returns immediately
// without paying for the LLM round-trip.
//
// Re-entrant Publish calls from within a Job's tool tail suppress ONLY
// the originating Job: an agent that calls case_writer.update_case cannot
// cause its own follow-up event to re-fire the same Job, but any other
// Job listening on the resulting lifecycle event still fires. This is what
// lets an on-created Job that closes the case trigger the on-closed Job.
//
// This does NOT structurally prevent cross-Job loops: in thread mode a
// status can be reopened and re-closed, so two Jobs listening on the same
// lifecycle whose agents reopen+re-close could ping-pong. Loop-freedom
// relies on agents not doing that (see JobActorMarker), not on topology.
func (uc *UseCase) Publish(ctx context.Context, ev Event) {
	if uc == nil || uc.registry == nil || uc.runner == nil {
		return
	}

	logger := logging.From(ctx)
	jobs, err := uc.matchJobs(ev)
	if err != nil {
		errutil.Handle(ctx, err, "job match failed")
		return
	}
	if len(jobs) == 0 {
		return
	}

	originator, isActor := jobActorFromContext(ctx)

	for _, j := range jobs {
		if isActor && j.ID == originator.JobID {
			// The originating Job's own write must not re-fire itself.
			continue
		}
		event := ev
		async.Dispatch(ctx, func(bgCtx context.Context) error {
			return uc.runner.Run(bgCtx, j, event)
		})
	}
	_ = logger
}

// PublishCaseLifecycle is the convenience adapter consumed by
// CaseUseCase. It wraps the lifecycle + Case into an Event and forwards
// to Publish. Kept here so usecase/case.go does not need to know the
// Event struct shape.
func (uc *UseCase) PublishCaseLifecycle(ctx context.Context, workspaceID string, c *model.Case, lifecycle model.CaseLifecycle, actorUserID string) {
	if uc == nil || c == nil {
		return
	}
	uc.Publish(ctx, Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   workspaceID,
		CaseID:        c.ID,
		CaseLifecycle: lifecycle,
		Timestamp:     time.Now().UTC(),
		ActorUserID:   actorUserID,
	})
}

// matchJobs returns every Job in the event's workspace that subscribes to
// the event's domain (and, for case domain, the specific lifecycle).
func (uc *UseCase) matchJobs(ev Event) ([]*model.Job, error) {
	ws, err := uc.registry.Get(ev.WorkspaceID)
	if err != nil {
		// Treat unknown workspace as "no match" rather than an error so
		// dispatch never blocks a write path on a stale registry.
		return nil, nil //nolint:nilerr // intentional: event for unknown workspace is a no-op
	}

	out := make([]*model.Job, 0, len(ws.Jobs))
	for _, j := range ws.Jobs {
		if j == nil {
			continue
		}
		switch ev.Domain {
		case model.JobEventDomainCase:
			if j.ListensCase(ev.CaseLifecycle) {
				out = append(out, j)
			}
		case model.JobEventDomainScheduled:
			if j.ListensScheduled() {
				out = append(out, j)
			}
		}
	}
	return out, nil
}
