package kernel

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gollem-dev/agentkit"
	"github.com/m-mizutani/goerr/v2"
)

// Subject kinds. A Subject is agentkit's single-flight lock: at most one open
// Process may hold a given (Kind, ID) pair, and Spawn reports ErrSubjectBusy for
// the second.
const (
	// SubjectSlackThread serialises the agent turns of one Slack thread. Its ID
	// is the model.Session.ID, which is why a host must claim (and therefore
	// persist) the Session before it spawns.
	SubjectSlackThread = "slack-thread"
	// SubjectJobRun serialises the runs of one configured Job on one case.
	SubjectJobRun = "job-run"
)

// ThreadSubject builds the turn-lock subject for a Slack thread.
func ThreadSubject(sessionID string) agentkit.SubjectRef {
	return agentkit.SubjectRef{Kind: SubjectSlackThread, ID: sessionID}
}

// JobRunSubject builds the turn-lock subject for one Job on one case.
func JobRunSubject(workspaceID string, caseID int64, jobID string) agentkit.SubjectRef {
	return agentkit.SubjectRef{Kind: SubjectJobRun, ID: jobRunSubjectID(workspaceID, caseID, jobID)}
}

// TriggerKey builds the idempotency key for a Slack-triggered turn. Spawn
// resolves an existing key to the Process it already created, which is how a
// re-delivered Slack event is dropped instead of starting a second turn.
//
// agentkit evaluates the idempotency key BEFORE the subject, so a duplicate
// delivery is answered with the original Process rather than with "busy" — the
// same precedence the Session turn lock applied.
func TriggerKey(channelID, threadTS, triggerTS string) string {
	return strings.Join([]string{"slack", channelID, threadTS, triggerTS}, "\x00")
}

// jobRunSubjectID is the opaque id half of a job-run subject. The separator is a
// NUL so no component can forge a boundary; the value is never parsed back.
func jobRunSubjectID(workspaceID string, caseID int64, jobID string) string {
	return strings.Join([]string{workspaceID, strconv.FormatInt(caseID, 10), jobID}, "\x00")
}

// BusyTurn describes the run currently holding a subject. It is what a host
// renders its "already working on this" message from.
type BusyTurn struct {
	ProcessID agentkit.ProcessID
	// StartedAt is when the holding Process was created, which is the moment the
	// turn began.
	StartedAt time.Time
}

// Locator answers "who holds this subject". It is deliberately narrower than
// agentkit.Repository: the application does not call the persistence SPI, and
// the only thing a host legitimately needs from it is what to put in a busy
// message.
type Locator interface {
	Busy(ctx context.Context, subject agentkit.SubjectRef) (*BusyTurn, error)
}

type repoLocator struct {
	repo agentkit.Repository
}

// NewLocator wraps a Repository as a Locator.
func NewLocator(repo agentkit.Repository) (Locator, error) {
	if repo == nil {
		return nil, goerr.New("agent process repository is required")
	}
	return &repoLocator{repo: repo}, nil
}

// Busy returns the open Process holding subject, or (nil, nil) when none does.
// "Nobody holds it" is an ordinary answer, not an error: the caller asks exactly
// when a Spawn reported busy, and the holder may have finished in between.
func (l *repoLocator) Busy(ctx context.Context, subject agentkit.SubjectRef) (*BusyTurn, error) {
	proc, err := l.repo.FindOpenProcessBySubject(ctx, subject)
	if err != nil {
		if errors.Is(err, agentkit.ErrProcessNotFound) {
			return nil, nil
		}
		return nil, goerr.Wrap(err, "find the process holding the subject",
			goerr.V("subject_kind", subject.Kind))
	}
	return &BusyTurn{ProcessID: proc.ID, StartedAt: proc.CreatedAt}, nil
}
