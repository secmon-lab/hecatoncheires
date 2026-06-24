package job

import (
	"context"
	"time"

	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// DefaultUnansweredTimeout is how long an interactive Job run may stay
// suspended awaiting user input before the scheduled sweep expires it
// (FAILED) and frees the (job, case) slot. Long enough that a human has a
// full day to answer; short enough that an abandoned question does not pin
// the slot forever.
const DefaultUnansweredTimeout = 24 * time.Hour

// expireLeaseDuration is the short lease the sweep takes while expiring a
// stale suspended run, just long enough to mutate the records atomically
// against a concurrent resume.
const expireLeaseDuration = time.Minute

// ScannerDeps groups the dependencies the ScheduledScanner needs.
type ScannerDeps struct {
	Repo      interfaces.Repository
	Registry  *model.WorkspaceRegistry
	Publisher EventPublisher

	// UnansweredTimeout overrides DefaultUnansweredTimeout for the
	// stale-suspended-run sweep. 0 → DefaultUnansweredTimeout.
	UnansweredTimeout time.Duration
}

// ScheduledScanner walks every workspace's scheduled Jobs and publishes
// an Event for each (job, case) tuple that has become due since its last
// run. Triggered externally — by the `hecatoncheires scheduled` CLI or
// by `POST /hooks/scheduled` — not on a wall-clock timer.
type ScheduledScanner struct {
	deps ScannerDeps
}

// NewScheduledScanner wires the scanner.
func NewScheduledScanner(deps ScannerDeps) *ScheduledScanner {
	return &ScheduledScanner{deps: deps}
}

// Scan walks every workspace, finds Jobs that subscribe to the scheduled
// domain, and for each non-CLOSED case decides whether to publish a
// scheduled event. Errors loading any individual case are logged but do
// not abort the sweep; loud Repo / Registry failures stop the sweep.
func (s *ScheduledScanner) Scan(ctx context.Context) error {
	if s == nil || s.deps.Registry == nil {
		return goerr.New("scanner has no registry")
	}
	now := time.Now().UTC()

	for _, ws := range s.deps.Registry.List() {
		if ws == nil {
			continue
		}
		scheduledJobs := make([]*model.Job, 0, len(ws.Jobs))
		hasInteractive := false
		for _, j := range ws.Jobs {
			if j == nil || j.Disabled {
				continue
			}
			if j.Events.Scheduled != nil {
				scheduledJobs = append(scheduledJobs, j)
			}
			if j.Interactive {
				hasInteractive = true
			}
		}
		// Fetch cases when there is scheduled work to dispatch OR an
		// interactive Job whose suspended runs may need sweeping. A
		// workspace with neither needs no per-case scan.
		if len(scheduledJobs) == 0 && !hasInteractive {
			continue
		}

		// Active (non-closed) cases only. status filter is OPEN since
		// the spec excludes CLOSED; DRAFT is also excluded because Jobs
		// run against published cases.
		openStatus := types.CaseStatusOpen
		cases, err := s.deps.Repo.Case().List(ctx, ws.Workspace.ID, interfaces.WithStatus(openStatus))
		if err != nil {
			return goerr.Wrap(err, "list cases in workspace",
				goerr.V("workspace_id", ws.Workspace.ID))
		}

		for _, c := range cases {
			// One subcollection query per OPEN case: returns this case's
			// existing JobRuns (typically <= number of scheduled jobs
			// in the workspace). Cross-case access patterns do not exist
			// for JobRun, so we keep the per-case scope that matches the
			// Firestore subcollection layout.
			runs, err := s.deps.Repo.JobRun().ListByCase(ctx, ws.Workspace.ID, c.ID)
			if err != nil {
				return goerr.Wrap(err, "list job runs for case",
					goerr.V("workspace_id", ws.Workspace.ID),
					goerr.V("case_id", c.ID))
			}
			byJobID := make(map[string]*model.JobRun, len(runs))
			for _, r := range runs {
				byJobID[r.JobID] = r
			}
			// Expire interactive runs that have been awaiting input past the
			// timeout, freeing the slot. A failure to expire one run is
			// non-fatal — log and continue the sweep.
			if hasInteractive {
				for _, r := range runs {
					if !r.IsSuspended() {
						continue
					}
					if expErr := s.expireSuspendedRun(ctx, r, now); expErr != nil {
						errutil.Handle(ctx, goerr.Wrap(expErr, "expire stale suspended run",
							goerr.V("workspace_id", r.WorkspaceID),
							goerr.V("case_id", r.CaseID),
							goerr.V("job_id", r.JobID)), "job: expire stale suspended run")
					}
				}
			}
			for _, j := range scheduledJobs {
				last, ok := byJobID[j.ID]
				if !ok {
					last = &model.JobRun{
						WorkspaceID: ws.Workspace.ID,
						CaseID:      c.ID,
						JobID:       j.ID,
					}
				}
				if !IsDue(j.Events.Scheduled, last.LastRunAt, now) {
					continue
				}
				ev := Event{
					Domain:       model.JobEventDomainScheduled,
					WorkspaceID:  ws.Workspace.ID,
					CaseID:       c.ID,
					Timestamp:    now,
					ActorUserID:  model.SystemActorID,
					LastRunAt:    last.LastRunAt,
					ScheduledFor: NextFireTime(j.Events.Scheduled, last.LastRunAt, now),
				}
				s.deps.Publisher.Publish(ctx, ev)
			}
		}
	}
	return nil
}

// expireSuspendedRun fails an interactive run that has been awaiting user
// input past the timeout, freeing the (job, case) slot. It takes a short
// lease as the exclusion gate against a concurrent resume, re-reads to
// confirm the run is still suspended (TOCTOU), and transitions the run log to
// FAILED before recording the terminal outcome (which clears the suspension
// marker). A run that is not yet stale, already resumed, or actively leased is
// left untouched.
func (s *ScheduledScanner) expireSuspendedRun(ctx context.Context, run *model.JobRun, now time.Time) error {
	timeout := s.deps.UnansweredTimeout
	if timeout <= 0 {
		timeout = DefaultUnansweredTimeout
	}
	if run.SuspendedAt.IsZero() || now.Sub(run.SuspendedAt) < timeout {
		return nil
	}
	key := run.Key()

	acquired, err := s.deps.Repo.JobRun().TryAcquireLease(ctx, key, now, expireLeaseDuration)
	if err != nil {
		return goerr.Wrap(err, "acquire lease to expire suspended run")
	}
	if !acquired {
		// A resume (or another sweep) is in flight — leave it alone.
		return nil
	}
	defer func() {
		if relErr := s.deps.Repo.JobRun().ReleaseLease(context.Background(), key); relErr != nil {
			_ = relErr
		}
	}()

	// Re-read under the lease: a resume may have cleared the suspension
	// between the ListByCase snapshot and acquiring the lease.
	fresh, err := s.deps.Repo.JobRun().Get(ctx, key)
	if err != nil {
		return goerr.Wrap(err, "re-read job run before expiry")
	}
	if fresh == nil || !fresh.IsSuspended() || now.Sub(fresh.SuspendedAt) < timeout {
		return nil
	}
	runID := fresh.SuspendedRunID

	const reason = "unanswered: no response within the interactive timeout"
	traceID := ""
	if logRec, logErr := s.deps.Repo.JobRunLog().Get(ctx, key, runID); logErr == nil && logRec != nil {
		traceID = logRec.TraceID
		// Finalize any non-terminal log behind the suspension — AWAITING_INPUT
		// (unanswered) or RUNNING (a resume that crashed mid-flight) — so it
		// does not linger forever; a terminal log is left as-is.
		if logRec.Stage != model.JobRunStageSuccess && logRec.Stage != model.JobRunStageFailed {
			logRec.Stage = model.JobRunStageFailed
			logRec.EndedAt = now
			logRec.Error = reason
			logRec.PendingInteraction = nil
			if finErr := s.deps.Repo.JobRunLog().Finish(ctx, logRec); finErr != nil {
				errutil.Handle(ctx, finErr, "job: finish expired run log")
			}
		}
	}

	// RecordRun clears the suspension marker and the lease, freeing the slot.
	if recErr := s.deps.Repo.JobRun().RecordRun(ctx, key, model.JobRunStatusFailed, now, runID, traceID, reason); recErr != nil {
		return goerr.Wrap(recErr, "record expired run")
	}
	return nil
}

// IsDue reports whether a scheduled event filter is due to fire given the
// timestamp of its last run and the current wall-clock time. When the
// last run is the zero value, every configuration is considered due
// (first-run semantics).
func IsDue(cfg *model.ScheduledEventConfig, lastRunAt, now time.Time) bool {
	if cfg == nil {
		return false
	}
	switch {
	case cfg.Every > 0:
		if lastRunAt.IsZero() {
			return true
		}
		return now.Sub(lastRunAt) >= cfg.Every
	case cfg.Cron != nil:
		if lastRunAt.IsZero() {
			return true
		}
		next := cfg.Cron.Next(lastRunAt)
		return !next.After(now)
	default:
		return false
	}
}

// NextFireTime returns the next scheduled firing time given the last run.
// Used to surface the "scheduled_for" context in the system prompt and
// to drive due-checks in tests. The reference time `now` is the anchor
// used when there is no prior run (first-fire semantics); take it from
// the caller so the function remains deterministic and trivially
// testable. Returns the zero value when the config is unset.
func NextFireTime(cfg *model.ScheduledEventConfig, lastRunAt, now time.Time) time.Time {
	if cfg == nil {
		return time.Time{}
	}
	switch {
	case cfg.Every > 0:
		if lastRunAt.IsZero() {
			return time.Time{}
		}
		return lastRunAt.Add(cfg.Every)
	case cfg.Cron != nil:
		if lastRunAt.IsZero() {
			return cfg.Cron.Next(now)
		}
		return cfg.Cron.Next(lastRunAt)
	default:
		return time.Time{}
	}
}
