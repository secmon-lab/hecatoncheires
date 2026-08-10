package kernel_test

import (
	"context"
	"testing"
	"time"

	"github.com/gollem-dev/agentkit"
	agentprocmemory "github.com/gollem-dev/agentkit/repository/memory"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
)

func TestThreadSubject(t *testing.T) {
	got := kernel.ThreadSubject("ssn-1")
	gt.String(t, got.Kind).Equal(kernel.SubjectSlackThread)
	gt.String(t, got.ID).Equal("ssn-1")
}

// TestJobRunSubjectDistinguishesEveryComponent pins that two different runs
// never collide on one subject. A collision would make one Job's run report the
// other as busy and silently skip.
func TestJobRunSubjectDistinguishesEveryComponent(t *testing.T) {
	base := kernel.JobRunSubject("ws-1", 1, "job-a")
	gt.String(t, base.Kind).Equal(kernel.SubjectJobRun)

	gt.String(t, kernel.JobRunSubject("ws-2", 1, "job-a").ID).NotEqual(base.ID)
	gt.String(t, kernel.JobRunSubject("ws-1", 2, "job-a").ID).NotEqual(base.ID)
	gt.String(t, kernel.JobRunSubject("ws-1", 1, "job-b").ID).NotEqual(base.ID)
	gt.String(t, kernel.JobRunSubject("ws-1", 1, "job-a").ID).Equal(base.ID)
}

// TestTriggerKeyDistinguishesEveryComponent pins that the idempotency key
// separates two genuinely different triggers. Collapsing them would drop a real
// mention as a duplicate delivery.
func TestTriggerKeyDistinguishesEveryComponent(t *testing.T) {
	base := kernel.TriggerKey("C1", "1.1", "2.2")

	gt.String(t, kernel.TriggerKey("C2", "1.1", "2.2")).NotEqual(base)
	gt.String(t, kernel.TriggerKey("C1", "9.9", "2.2")).NotEqual(base)
	gt.String(t, kernel.TriggerKey("C1", "1.1", "3.3")).NotEqual(base)
	gt.String(t, kernel.TriggerKey("C1", "1.1", "2.2")).Equal(base)
}

func TestNewLocatorRequiresRepository(t *testing.T) {
	loc, err := kernel.NewLocator(nil)
	gt.Value(t, err).NotNil()
	gt.Value(t, loc).Nil()
}

func TestLocatorBusy(t *testing.T) {
	ctx := context.Background()
	repo := agentprocmemory.New()
	loc, err := kernel.NewLocator(repo)
	gt.NoError(t, err).Required()

	subject := kernel.ThreadSubject("ssn-1")
	created := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

	t.Run("nobody holds the subject", func(t *testing.T) {
		busy, err := loc.Busy(ctx, subject)
		gt.NoError(t, err)
		gt.Value(t, busy).Nil()
	})

	proc := &agentkit.Process{
		ID:        "proc-1",
		Agent:     kernel.AgentCaseThread,
		Status:    agentkit.ProcessRunning,
		RootID:    "proc-1",
		Subject:   &subject,
		CreatedAt: created,
		UpdatedAt: created,
	}
	gt.NoError(t, repo.Apply(ctx, agentkit.ChangeSet{Processes: []*agentkit.Process{proc}})).Required()

	t.Run("an open process is reported with its start time", func(t *testing.T) {
		busy, err := loc.Busy(ctx, subject)
		gt.NoError(t, err).Required()
		gt.Value(t, busy).NotNil().Required()
		gt.Value(t, busy.ProcessID).Equal(agentkit.ProcessID("proc-1"))
		gt.Bool(t, busy.StartedAt.Equal(created)).True()
	})

	t.Run("a finished process releases the subject", func(t *testing.T) {
		done := *proc
		done.Rev = 1
		done.Status = agentkit.ProcessSucceeded
		gt.NoError(t, repo.Apply(ctx, agentkit.ChangeSet{Processes: []*agentkit.Process{&done}})).Required()

		busy, err := loc.Busy(ctx, subject)
		gt.NoError(t, err)
		gt.Value(t, busy).Nil()
	})
}
