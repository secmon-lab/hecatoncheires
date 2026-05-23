package job_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/robfig/cron/v3"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/job"
)

func TestIsDue_Every(t *testing.T) {
	cfg := &model.ScheduledEventConfig{Every: time.Hour}
	now := time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)

	t.Run("first run (no prior)", func(t *testing.T) {
		gt.Bool(t, job.IsDue(cfg, time.Time{}, now)).True()
	})
	t.Run("just at duration", func(t *testing.T) {
		gt.Bool(t, job.IsDue(cfg, now.Add(-time.Hour), now)).True()
	})
	t.Run("just before duration", func(t *testing.T) {
		gt.Bool(t, job.IsDue(cfg, now.Add(-59*time.Minute), now)).False()
	})
}

func TestIsDue_Cron(t *testing.T) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse("0 9 * * *") // every day 09:00 UTC
	gt.NoError(t, err).Required()
	cfg := &model.ScheduledEventConfig{Cron: sched, CronExpr: "0 9 * * *"}

	t.Run("first run is due", func(t *testing.T) {
		gt.Bool(t, job.IsDue(cfg, time.Time{}, time.Now().UTC())).True()
	})
	t.Run("not yet fired today", func(t *testing.T) {
		now := time.Date(2026, 5, 23, 8, 59, 0, 0, time.UTC)
		last := time.Date(2026, 5, 22, 9, 0, 0, 0, time.UTC)
		gt.Bool(t, job.IsDue(cfg, last, now)).False()
	})
	t.Run("just past fire time", func(t *testing.T) {
		now := time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC)
		last := time.Date(2026, 5, 22, 9, 0, 0, 0, time.UTC)
		gt.Bool(t, job.IsDue(cfg, last, now)).True()
	})
}

func TestIsDue_NilOrUnset(t *testing.T) {
	gt.Bool(t, job.IsDue(nil, time.Time{}, time.Now())).False()
	gt.Bool(t, job.IsDue(&model.ScheduledEventConfig{}, time.Time{}, time.Now())).False()
}

// recordingPublisher records every Publish call without invoking any
// runner / executor / LLM.
type recordingPublisher struct {
	mu     sync.Mutex
	events []job.Event
}

func (p *recordingPublisher) Publish(_ context.Context, ev job.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, ev)
}

func (p *recordingPublisher) snapshot() []job.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]job.Event, len(p.events))
	copy(out, p.events)
	return out
}

func TestScheduledScanner_PublishesDueJobs(t *testing.T) {
	ctx := context.Background()
	repo, caseA := setupCase(t, "ws")

	registry := model.NewWorkspaceRegistry()
	dueJob := &model.Job{
		ID:     "stale-check",
		Prompt: "x",
		Events: model.JobEvents{
			Scheduled: &model.ScheduledEventConfig{Every: time.Hour},
		},
	}
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws"},
		Jobs:      []*model.Job{dueJob},
	})

	// Seed a last-run timestamp two hours ago so the job is due.
	gt.NoError(t, repo.JobRun().RecordRun(ctx, model.JobRunKey{
		WorkspaceID: "ws", CaseID: caseA.ID, JobID: dueJob.ID,
	}, model.JobRunStatusSuccess, time.Now().UTC().Add(-2*time.Hour), "", "")).Required()

	pub := &recordingPublisher{}
	scanner := job.NewScheduledScanner(job.ScannerDeps{
		Repo:      repo,
		Registry:  registry,
		Publisher: pub,
	})
	gt.NoError(t, scanner.Scan(ctx)).Required()

	events := pub.snapshot()
	gt.Array(t, events).Length(1).Required()
	gt.Value(t, events[0].Domain).Equal(model.JobEventDomainScheduled)
	gt.Value(t, events[0].WorkspaceID).Equal("ws")
	gt.Value(t, events[0].CaseID).Equal(caseA.ID)
	gt.Value(t, events[0].ActorUserID).Equal(model.SystemActorID)
}

func TestScheduledScanner_SkipsNotYetDue(t *testing.T) {
	ctx := context.Background()
	repo, caseA := setupCase(t, "ws")

	registry := model.NewWorkspaceRegistry()
	j := &model.Job{
		ID:     "stale-check",
		Prompt: "x",
		Events: model.JobEvents{
			Scheduled: &model.ScheduledEventConfig{Every: time.Hour},
		},
	}
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws"},
		Jobs:      []*model.Job{j},
	})

	// Last run was just now — not due.
	gt.NoError(t, repo.JobRun().RecordRun(ctx, model.JobRunKey{
		WorkspaceID: "ws", CaseID: caseA.ID, JobID: j.ID,
	}, model.JobRunStatusSuccess, time.Now().UTC(), "", "")).Required()

	pub := &recordingPublisher{}
	scanner := job.NewScheduledScanner(job.ScannerDeps{
		Repo: repo, Registry: registry, Publisher: pub,
	})
	gt.NoError(t, scanner.Scan(ctx)).Required()
	gt.Array(t, pub.snapshot()).Length(0)
}

func TestScheduledScanner_SkipsDisabledJobs(t *testing.T) {
	ctx := context.Background()
	repo, _ := setupCase(t, "ws")

	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws"},
		Jobs: []*model.Job{{
			ID:       "disabled",
			Prompt:   "x",
			Disabled: true,
			Events:   model.JobEvents{Scheduled: &model.ScheduledEventConfig{Every: time.Hour}},
		}},
	})

	pub := &recordingPublisher{}
	scanner := job.NewScheduledScanner(job.ScannerDeps{
		Repo: repo, Registry: registry, Publisher: pub,
	})
	gt.NoError(t, scanner.Scan(ctx)).Required()
	gt.Array(t, pub.snapshot()).Length(0)
}

func TestScheduledScanner_FirstRunImmediatelyDue(t *testing.T) {
	ctx := context.Background()
	repo, _ := setupCase(t, "ws")

	registry := model.NewWorkspaceRegistry()
	j := &model.Job{
		ID:     "stale",
		Prompt: "x",
		Events: model.JobEvents{
			Scheduled: &model.ScheduledEventConfig{Every: time.Hour},
		},
	}
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws"},
		Jobs:      []*model.Job{j},
	})

	pub := &recordingPublisher{}
	scanner := job.NewScheduledScanner(job.ScannerDeps{Repo: repo, Registry: registry, Publisher: pub})
	gt.NoError(t, scanner.Scan(ctx)).Required()
	// No prior JobRun → due on first scan.
	gt.Array(t, pub.snapshot()).Length(1)
}
