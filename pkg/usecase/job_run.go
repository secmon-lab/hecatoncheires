package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"golang.org/x/sync/errgroup"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

// JobRunUseCase exposes read-only access to Job execution history and the
// per-Run timeline. It is intentionally narrow: the CaseAgent page and the
// JobRunLog detail page both rely on these three methods (plus
// CaseUseCase for the parent-Case access check, which is reused here).
//
// Access control follows the same Private Case pattern as CaseUseCase:
// the parent Case is loaded first; if the caller does not belong to its
// channel, the read is refused with ErrAccessDenied. System contexts
// without an auth token (background runner, etc.) bypass the check —
// the same backward-compatibility carveout already used elsewhere.
type JobRunUseCase struct {
	repo     interfaces.Repository
	registry *model.WorkspaceRegistry
	trigger  JobTrigger
}

// JobTrigger is the Job runtime as the manual-trigger path needs it.
// Implemented by *job.JobRunner; declared here so the usecase layer does not
// import the Job runtime package (which would invert the dependency).
type JobTrigger interface {
	// CanRunManual reports whether a manual run may start right now for the
	// given tuple. false means a run already holds the slot. The decision
	// lives in the runtime because it depends on the lease, the run log and
	// the unanswered-question timeout that the runtime owns.
	CanRunManual(ctx context.Context, workspaceID string, caseID int64, jobID string) (bool, error)

	// RunManual executes the Job. It is called from a background goroutine,
	// so it blocks for the whole run.
	RunManual(ctx context.Context, workspaceID string, caseID int64, jobID, actorUserID string) error
}

// NewJobRunUseCase wires the JobRunUseCase. registry may be nil: the
// jobName field on returned logs falls back to the JobID when no entry
// is registered (e.g. early bootstrap), and never blocks the call.
func NewJobRunUseCase(repo interfaces.Repository, registry *model.WorkspaceRegistry) *JobRunUseCase {
	return &JobRunUseCase{repo: repo, registry: registry}
}

// SetTrigger wires the manual-trigger runtime. Called once at startup
// after the JobRunner has been constructed — it cannot be a New() option
// because the runner is built from the UseCases themselves. Leaving it
// unset makes TriggerJob fail loudly rather than accept a silent no-op.
func (uc *JobRunUseCase) SetTrigger(t JobTrigger) {
	uc.trigger = t
}

// JobRunLogPageDefaultSize is the page size used when the caller does
// not specify one (or specifies <= 0). Aligned with the design target
// for the CaseAgent dashboard run-log table.
const JobRunLogPageDefaultSize = 20

// JobRunLogPageMaxSize caps the upper bound a caller can request, to
// keep a single page round-trip small enough to stay snappy.
const JobRunLogPageMaxSize = 100

// JobRunLogPage is the result of ListLogsByCase. NextCursor is non-nil
// only when more pages exist; the value is an opaque base64 token that
// the caller passes back as `after` on the next call.
type JobRunLogPage struct {
	Items      []*model.JobRunLog
	NextCursor *string
}

// jobRunLogCursor is the cursor payload. We sort by StartedAt DESC,
// breaking ties on RunID (which is globally unique per Run). Encoded as
// base64-of-JSON so the wire token is opaque and can evolve without a
// breaking change.
type jobRunLogCursor struct {
	StartedNanos int64  `json:"s"`
	RunID        string `json:"r"`
}

func encodeJobRunLogCursor(c jobRunLogCursor) (string, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return "", goerr.Wrap(err, "encode cursor json")
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeJobRunLogCursor(s string) (jobRunLogCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return jobRunLogCursor{}, goerr.Wrap(ErrInvalidArgument, "invalid cursor encoding")
	}
	var c jobRunLogCursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return jobRunLogCursor{}, goerr.Wrap(ErrInvalidArgument, "invalid cursor payload")
	}
	if c.RunID == "" {
		return jobRunLogCursor{}, goerr.Wrap(ErrInvalidArgument, "cursor missing run id")
	}
	return c, nil
}

// ListLogsByCase returns one page of JobRunLogs for the given Case
// across every Job that has ever run against it, ordered newest-first.
//
// Pagination semantics: page is the requested size (clamped to
// [1, JobRunLogPageMaxSize], defaulting to JobRunLogPageDefaultSize on
// zero/negative). `after` is the opaque cursor from a previous call;
// nil/empty means "start at the head".
//
// Implementation note: the storage layout is per-Job (logs live under
// jobRuns/{job}/logs/), so a true cross-job query would need a
// Firestore collectionGroup (and a composite index). To stay within
// the project's "no new indexes" rule we fan out to one List per Job
// in parallel (small N: handful of Jobs per Case) and merge in
// memory. The cursor is then applied to the merged slice.
func (uc *JobRunUseCase) ListLogsByCase(ctx context.Context, workspaceID string, caseID int64, page int, after *string) (*JobRunLogPage, error) {
	if workspaceID == "" {
		return nil, goerr.Wrap(ErrInvalidArgument, "workspace id is empty")
	}
	if caseID == 0 {
		return nil, goerr.Wrap(ErrInvalidArgument, "case id is zero")
	}

	// Access control via the parent Case.
	if err := uc.checkCaseAccess(ctx, workspaceID, caseID); err != nil {
		return nil, err
	}

	size := page
	switch {
	case size <= 0:
		size = JobRunLogPageDefaultSize
	case size > JobRunLogPageMaxSize:
		size = JobRunLogPageMaxSize
	}

	var cursor *jobRunLogCursor
	if after != nil && *after != "" {
		c, err := decodeJobRunLogCursor(*after)
		if err != nil {
			return nil, err
		}
		cursor = &c
	}

	runs, err := uc.repo.JobRun().ListByCase(ctx, workspaceID, caseID)
	if err != nil {
		return nil, goerr.Wrap(err, "list job runs for case",
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", caseID))
	}

	merged, err := uc.collectLogs(ctx, workspaceID, caseID, runs)
	if err != nil {
		return nil, err
	}

	// Sort newest first; ties broken by RunID DESC so cursor comparisons
	// are total. RunID uniqueness gives a strict total order, so the
	// non-stable variant is sufficient.
	sort.Slice(merged, func(i, j int) bool {
		if !merged[i].StartedAt.Equal(merged[j].StartedAt) {
			return merged[i].StartedAt.After(merged[j].StartedAt)
		}
		return merged[i].RunID > merged[j].RunID
	})

	if cursor != nil {
		startIdx := -1
		for i, log := range merged {
			if log.StartedAt.UnixNano() == cursor.StartedNanos && log.RunID == cursor.RunID {
				startIdx = i + 1
				break
			}
			// Cursor's StartedAt may not appear if logs changed concurrently;
			// fall back to "first item strictly earlier than the cursor".
			if log.StartedAt.UnixNano() < cursor.StartedNanos ||
				(log.StartedAt.UnixNano() == cursor.StartedNanos && log.RunID < cursor.RunID) {
				startIdx = i
				break
			}
		}
		if startIdx == -1 {
			startIdx = len(merged)
		}
		merged = merged[startIdx:]
	}

	out := &JobRunLogPage{}
	if len(merged) > size {
		page := merged[:size]
		next := page[size-1]
		token, err := encodeJobRunLogCursor(jobRunLogCursor{
			StartedNanos: next.StartedAt.UnixNano(),
			RunID:        next.RunID,
		})
		if err != nil {
			return nil, err
		}
		out.Items = page
		out.NextCursor = &token
	} else {
		out.Items = merged
	}
	return out, nil
}

// collectLogs fans out one List call per JobRun (Job × Case). The
// upper bound on per-job results is JobRunLogPageMaxSize so that even
// with several Jobs the merge cost stays bounded.
func (uc *JobRunUseCase) collectLogs(ctx context.Context, workspaceID string, caseID int64, runs []*model.JobRun) ([]*model.JobRunLog, error) {
	if len(runs) == 0 {
		return nil, nil
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(8)

	var mu sync.Mutex
	merged := make([]*model.JobRunLog, 0)

	for _, r := range runs {
		key := model.JobRunKey{WorkspaceID: workspaceID, CaseID: caseID, JobID: r.JobID}
		g.Go(func() error {
			logs, err := uc.repo.JobRunLog().List(gctx, key, JobRunLogPageMaxSize)
			if err != nil {
				return goerr.Wrap(err, "list job run logs",
					goerr.V("workspace_id", workspaceID),
					goerr.V("case_id", caseID),
					goerr.V("job_id", key.JobID))
			}
			mu.Lock()
			merged = append(merged, logs...)
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return merged, nil
}

// GetLog returns a single JobRunLog by RunID. The runID is unique per
// Run, but the storage layout includes the JobID in the document
// path, so we resolve the JobID by walking the per-Case JobRun list
// (small) and probing each one. This avoids a collectionGroup query.
func (uc *JobRunUseCase) GetLog(ctx context.Context, workspaceID string, caseID int64, runID string) (*model.JobRunLog, error) {
	if workspaceID == "" {
		return nil, goerr.Wrap(ErrInvalidArgument, "workspace id is empty")
	}
	if caseID == 0 {
		return nil, goerr.Wrap(ErrInvalidArgument, "case id is zero")
	}
	if runID == "" {
		return nil, goerr.Wrap(ErrInvalidArgument, "run id is empty")
	}

	if err := uc.checkCaseAccess(ctx, workspaceID, caseID); err != nil {
		return nil, err
	}

	log, err := uc.findLog(ctx, workspaceID, caseID, runID)
	if err != nil {
		return nil, err
	}
	if log == nil {
		return nil, goerr.Wrap(interfaces.ErrJobRunLogNotFound, "job run log not found",
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", caseID),
			goerr.V("run_id", runID))
	}
	return log, nil
}

// ListEvents returns every JobRunEvent under (workspace, case, run)
// in ascending Sequence order. As with GetLog, the JobID is resolved
// by walking the per-Case JobRun list first.
func (uc *JobRunUseCase) ListEvents(ctx context.Context, workspaceID string, caseID int64, runID string) ([]*model.JobRunEvent, error) {
	if workspaceID == "" {
		return nil, goerr.Wrap(ErrInvalidArgument, "workspace id is empty")
	}
	if caseID == 0 {
		return nil, goerr.Wrap(ErrInvalidArgument, "case id is zero")
	}
	if runID == "" {
		return nil, goerr.Wrap(ErrInvalidArgument, "run id is empty")
	}

	if err := uc.checkCaseAccess(ctx, workspaceID, caseID); err != nil {
		return nil, err
	}

	log, err := uc.findLog(ctx, workspaceID, caseID, runID)
	if err != nil {
		return nil, err
	}
	if log == nil {
		return nil, goerr.Wrap(interfaces.ErrJobRunLogNotFound, "job run log not found",
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", caseID),
			goerr.V("run_id", runID))
	}

	events, err := uc.repo.JobRunEvent().List(ctx, model.JobRunKey{
		WorkspaceID: workspaceID,
		CaseID:      caseID,
		JobID:       log.JobID,
	}, runID)
	if err != nil {
		return nil, goerr.Wrap(err, "list job run events",
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", caseID),
			goerr.V("run_id", runID))
	}
	return events, nil
}

// ResolveJobName returns the human-readable Job name from the workspace
// TOML registry, falling back to the raw JobID when no entry exists.
// Exposed so resolvers can label runs without re-loading the registry
// themselves. A mention-triggered run (eventType == EventTypeMention) is not a
// configured Job — its JobID is an opaque per-turn id — so it resolves to a
// localized "Mention" label instead.
func (uc *JobRunUseCase) ResolveJobName(ctx context.Context, workspaceID, jobID, eventType string) string {
	if eventType == model.EventTypeMention {
		return i18n.T(ctx, i18n.MsgAgentMentionRunName)
	}
	if uc.registry == nil {
		return jobID
	}
	entry, err := uc.registry.Get(workspaceID)
	if err != nil || entry == nil {
		return jobID
	}
	for _, j := range entry.Jobs {
		if j != nil && j.ID == jobID {
			if j.Name != "" {
				return j.Name
			}
			return j.ID
		}
	}
	return jobID
}

// findLog probes each JobRun for the matching RunID. The per-Case
// JobRun list is small (one entry per Job that has ever run against
// the Case), so the linear scan is acceptable and keeps us off
// collectionGroup queries.
func (uc *JobRunUseCase) findLog(ctx context.Context, workspaceID string, caseID int64, runID string) (*model.JobRunLog, error) {
	runs, err := uc.repo.JobRun().ListByCase(ctx, workspaceID, caseID)
	if err != nil {
		return nil, goerr.Wrap(err, "list job runs for case",
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", caseID))
	}
	for _, r := range runs {
		key := model.JobRunKey{WorkspaceID: workspaceID, CaseID: caseID, JobID: r.JobID}
		log, err := uc.repo.JobRunLog().Get(ctx, key, runID)
		if err != nil {
			if errors.Is(err, interfaces.ErrJobRunLogNotFound) {
				continue
			}
			return nil, goerr.Wrap(err, "get job run log",
				goerr.V("workspace_id", workspaceID),
				goerr.V("case_id", caseID),
				goerr.V("job_id", key.JobID),
				goerr.V("run_id", runID))
		}
		return log, nil
	}
	return nil, nil
}

// ListCaseJobs returns the enabled Job definitions that can fire against
// the given Case. Definitions come from the in-memory Workspace registry
// (workspace TOML), never a repository. A scheduled Job is included only
// while the Case is OPEN — mirroring the ScheduledScanner, which skips
// DRAFT/CLOSED cases — whereas a case-lifecycle Job is always included.
//
// Access control matches the other JobRun reads: the parent Case is
// loaded first and a non-member caller is refused with ErrAccessDenied;
// system contexts without an auth token bypass the check. When the
// registry is unset (early bootstrap) the result is an empty slice.
func (uc *JobRunUseCase) ListCaseJobs(ctx context.Context, workspaceID string, caseID int64) ([]*model.Job, error) {
	if workspaceID == "" {
		return nil, goerr.Wrap(ErrInvalidArgument, "workspace id is empty")
	}
	if caseID == 0 {
		return nil, goerr.Wrap(ErrInvalidArgument, "case id is zero")
	}

	c, err := uc.repo.Case().Get(ctx, workspaceID, caseID)
	if err != nil {
		return nil, goerr.Wrap(ErrCaseNotFound, "case not found",
			goerr.V(CaseIDKey, caseID))
	}
	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil && !model.IsCaseAccessible(c, token.Sub) {
		return nil, goerr.Wrap(ErrAccessDenied,
			"cannot read agent jobs of private case",
			goerr.V(CaseIDKey, caseID),
			goerr.V("user_id", token.Sub))
	}

	return uc.jobsForCase(workspaceID, c)
}

// jobsForCase resolves the workspace registry entry and returns the Jobs that
// can fire against c. It performs NO access control — the caller has already
// gated the Case — so that the listing and the manual trigger share one
// definition of "runnable for this Case" and cannot drift apart. An unset
// registry or a workspace with no entry yields an empty slice rather than an
// error, matching ResolveJobName's tolerance during early bootstrap.
func (uc *JobRunUseCase) jobsForCase(workspaceID string, c *model.Case) ([]*model.Job, error) {
	if uc.registry == nil {
		return []*model.Job{}, nil
	}
	entry, err := uc.registry.Get(workspaceID)
	if err != nil {
		return nil, goerr.Wrap(err, "get workspace from registry",
			goerr.V("workspace_id", workspaceID))
	}
	if entry == nil {
		return []*model.Job{}, nil
	}

	isOpen := c.Status.Normalize() == types.CaseStatusOpen
	out := make([]*model.Job, 0, len(entry.Jobs))
	for _, j := range entry.Jobs {
		if j == nil || j.Disabled {
			continue
		}
		// A scheduled Job only fires while the Case is OPEN (the scanner
		// never sweeps DRAFT/CLOSED cases), so it is irrelevant to surface
		// it on a closed Case. A case-lifecycle Job stays listed regardless
		// of the current status — it describes how this Case reacts to its
		// own transitions.
		listensCaseEvent := j.Events.Case != nil
		listensScheduledWhenOpen := j.Events.Scheduled != nil && isOpen
		if listensCaseEvent || listensScheduledWhenOpen {
			out = append(out, j)
		}
	}
	return out, nil
}

// TriggerJob starts a manual run of jobID against the Case on behalf of the
// authenticated caller (the web UI's Run button).
//
// Every check that can be made synchronously is made here — the Case exists
// and is readable, the Job is one the Case can run, and no run is currently
// in flight — so the operator gets an immediate answer. The run itself is
// dispatched to the background: it takes minutes (LLM round-trips) and the
// GraphQL request must not block on it.
//
// The triggerable set comes from jobsForCase — the same filter the caseJobs
// query applies — so the mutation and the query can never disagree about
// which Jobs are available.
//
// Returns:
//   - ErrInvalidArgument   — empty workspace id / zero case id / empty job id
//   - ErrCaseNotFound      — the Case does not exist
//   - ErrAccessDenied      — private Case and the caller cannot access it
//   - ErrJobNotFound       — jobID is not triggerable for this Case
//   - ErrJobAlreadyRunning — a live lease or an open question holds the slot
func (uc *JobRunUseCase) TriggerJob(ctx context.Context, workspaceID string, caseID int64, jobID string) error {
	if workspaceID == "" {
		return goerr.Wrap(ErrInvalidArgument, "workspace id is empty")
	}
	if caseID == 0 {
		return goerr.Wrap(ErrInvalidArgument, "case id is zero")
	}
	if jobID == "" {
		return goerr.Wrap(ErrInvalidArgument, "job id is empty")
	}
	if uc.trigger == nil {
		return goerr.New("job trigger runtime is not configured")
	}

	// Starting a Job lets its agent write to the Case, so this is a Case
	// write path and goes through the shared access gate — not the read-side
	// check ListCaseJobs uses. The gate is draft-aware: the reporter of their
	// own private draft is admitted, which an open-coded channel-membership
	// test would refuse.
	c, err := loadCaseForWrite(ctx, uc.repo, workspaceID, caseID)
	if err != nil {
		return err
	}
	jobs, err := uc.jobsForCase(workspaceID, c)
	if err != nil {
		return err
	}
	found := false
	for _, j := range jobs {
		if j != nil && j.ID == jobID {
			found = true
			break
		}
	}
	if !found {
		return goerr.Wrap(ErrJobNotFound, "job is not triggerable for this case",
			goerr.V("workspace_id", workspaceID),
			goerr.V(CaseIDKey, caseID),
			goerr.V("job_id", jobID))
	}

	// Whether a run already holds the slot is the runtime's judgement, not a
	// second opinion formed here: it owns the lease, the unanswered-question
	// timeout, and the run-log state that together decide it. Asking it keeps
	// this check and the one Run performs from drifting apart.
	canRun, err := uc.trigger.CanRunManual(ctx, workspaceID, caseID, jobID)
	if err != nil {
		// Fail closed on a read failure: a transient error must not be read
		// as "idle" and let a second run start alongside a live one.
		return goerr.Wrap(err, "check job run state before manual trigger",
			goerr.V("workspace_id", workspaceID),
			goerr.V(CaseIDKey, caseID),
			goerr.V("job_id", jobID))
	}
	if !canRun {
		return goerr.Wrap(ErrJobAlreadyRunning, "a run for this job is already in flight",
			goerr.V("workspace_id", workspaceID),
			goerr.V(CaseIDKey, caseID),
			goerr.V("job_id", jobID))
	}

	// System contexts without a token (never the web UI) run with an empty
	// actor, the same carve-out the reads above use.
	actorUserID := ""
	if token, tokenErr := auth.TokenFromContext(ctx); tokenErr == nil {
		actorUserID = token.Sub
	}

	trigger := uc.trigger
	async.Dispatch(ctx, func(bgCtx context.Context) error {
		return trigger.RunManual(bgCtx, workspaceID, caseID, jobID, actorUserID)
	})
	return nil
}

// checkCaseAccess loads the parent Case and refuses the call when a
// non-member is asking about a private Case. System contexts without
// an auth token skip the check (same carveout as CaseUseCase).
func (uc *JobRunUseCase) checkCaseAccess(ctx context.Context, workspaceID string, caseID int64) error {
	c, err := uc.repo.Case().Get(ctx, workspaceID, caseID)
	if err != nil {
		return goerr.Wrap(ErrCaseNotFound, "case not found",
			goerr.V(CaseIDKey, caseID))
	}
	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil && !model.IsCaseAccessible(c, token.Sub) {
		return goerr.Wrap(ErrAccessDenied,
			"cannot read agent history of private case",
			goerr.V(CaseIDKey, caseID),
			goerr.V("user_id", token.Sub))
	}
	return nil
}
