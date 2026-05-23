package job

import (
	"context"
	"errors"
	"time"

	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// ScannerDeps groups the dependencies the ScheduledScanner needs.
type ScannerDeps struct {
	Repo      interfaces.Repository
	Registry  *model.WorkspaceRegistry
	Publisher EventPublisher
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
		for _, j := range ws.Jobs {
			if j == nil || j.Disabled {
				continue
			}
			if j.Events.Scheduled != nil {
				scheduledJobs = append(scheduledJobs, j)
			}
		}
		if len(scheduledJobs) == 0 {
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
			for _, j := range scheduledJobs {
				key := model.JobRunKey{WorkspaceID: ws.Workspace.ID, CaseID: c.ID, JobID: j.ID}
				last, err := s.deps.Repo.JobRun().Get(ctx, key)
				switch {
				case err == nil:
				case errors.Is(err, interfaces.ErrJobRunNotFound):
					last = &model.JobRun{Key: key}
				default:
					return goerr.Wrap(err, "load job run for due check",
						goerr.V("workspace_id", ws.Workspace.ID),
						goerr.V("case_id", c.ID),
						goerr.V("job_id", j.ID))
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
					ScheduledFor: NextFireTime(j.Events.Scheduled, last.LastRunAt),
				}
				s.deps.Publisher.Publish(ctx, ev)
			}
		}
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
// to drive due-checks in tests. Returns the zero value when the config is
// unset.
func NextFireTime(cfg *model.ScheduledEventConfig, lastRunAt time.Time) time.Time {
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
			return cfg.Cron.Next(time.Now().UTC())
		}
		return cfg.Cron.Next(lastRunAt)
	default:
		return time.Time{}
	}
}
