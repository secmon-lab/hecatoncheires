package model

import (
	"slices"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/robfig/cron/v3"
)

// CaseLifecycle enumerates the case lifecycle events that a Job can listen
// to via `events.case.on` in TOML. Each value names a distinct, observable
// transition.
type CaseLifecycle string

const (
	// CaseLifecycleCreated fires when a Case is newly created (regardless of
	// its initial status — DRAFT or OPEN both qualify).
	CaseLifecycleCreated CaseLifecycle = "created"
	// CaseLifecycleClosed fires when a Case's status transitions to CLOSED.
	CaseLifecycleClosed CaseLifecycle = "closed"
)

// AllCaseLifecycles returns every valid CaseLifecycle for validation and
// documentation purposes.
func AllCaseLifecycles() []CaseLifecycle {
	return []CaseLifecycle{
		CaseLifecycleCreated,
		CaseLifecycleClosed,
	}
}

// IsValid reports whether the lifecycle value is one of the recognised
// enum members.
func (l CaseLifecycle) IsValid() bool {
	switch l {
	case CaseLifecycleCreated, CaseLifecycleClosed:
		return true
	default:
		return false
	}
}

// String returns the string form for prompt rendering / logging.
func (l CaseLifecycle) String() string { return string(l) }

// JobEventDomain enumerates the event domains the dispatcher recognises.
// Each domain has a distinct filter schema (see CaseEventConfig /
// ScheduledEventConfig).
type JobEventDomain string

const (
	JobEventDomainCase      JobEventDomain = "case"
	JobEventDomainScheduled JobEventDomain = "scheduled"
)

// CaseEventConfig is the listen filter for the `case` event domain. A Job
// fires when the published Case event's CaseLifecycle is contained in On.
type CaseEventConfig struct {
	// On lists the lifecycle events the Job is subscribed to. Always a
	// normalised, non-empty slice produced by the config loader from the
	// TOML "on" field. The domain layer never accepts a single-string form
	// — see config/job.go for the parsing rules.
	On []CaseLifecycle
}

// Matches reports whether the given lifecycle event matches this config's
// On filter.
func (c *CaseEventConfig) Matches(lc CaseLifecycle) bool {
	if c == nil {
		return false
	}
	return slices.Contains(c.On, lc)
}

// Validate enforces invariants for the case event filter:
// - On must be non-empty
// - every value must be a known CaseLifecycle
func (c *CaseEventConfig) Validate() error {
	if c == nil {
		return goerr.New("case event config is nil")
	}
	if len(c.On) == 0 {
		return goerr.New("events.case.on must not be empty")
	}
	seen := make(map[CaseLifecycle]struct{}, len(c.On))
	for _, lc := range c.On {
		if !lc.IsValid() {
			return goerr.New("invalid case lifecycle value",
				goerr.V("value", string(lc)))
		}
		if _, dup := seen[lc]; dup {
			return goerr.New("duplicate case lifecycle in on",
				goerr.V("value", string(lc)))
		}
		seen[lc] = struct{}{}
	}
	return nil
}

// ScheduledEventConfig is the listen filter for the `scheduled` event
// domain. Exactly one of Every / Cron must be populated.
type ScheduledEventConfig struct {
	// Every is the duration since the last run after which the Job becomes
	// due. Mutually exclusive with Cron.
	Every time.Duration

	// Cron is the parsed schedule for cron-style triggers. Mutually
	// exclusive with Every. Parsed at config load time so a malformed
	// expression fails loud before any Job dispatch.
	Cron cron.Schedule

	// CronExpr is the original cron expression preserved for display and
	// system-prompt rendering. Empty when Every is in use.
	CronExpr string
}

// Validate enforces the mutual exclusion: exactly one of Every / Cron.
func (s *ScheduledEventConfig) Validate() error {
	if s == nil {
		return goerr.New("scheduled event config is nil")
	}
	hasEvery := s.Every > 0
	hasCron := s.Cron != nil
	switch {
	case hasEvery && hasCron:
		return goerr.New("events.scheduled must specify exactly one of every or cron, not both")
	case !hasEvery && !hasCron:
		return goerr.New("events.scheduled must specify either every or cron")
	}
	return nil
}

// JobEvents collects every event filter a Job listens to. A nil pointer
// means the corresponding domain is not subscribed; a non-nil pointer
// indicates the Job listens to that domain with the given filter.
type JobEvents struct {
	Case      *CaseEventConfig
	Scheduled *ScheduledEventConfig
}

// Validate enforces invariants for the event map:
// - at least one of Case / Scheduled must be non-nil
// - each non-nil sub-config must itself be valid
func (e *JobEvents) Validate() error {
	if e == nil {
		return goerr.New("events is nil")
	}
	if e.Case == nil && e.Scheduled == nil {
		return goerr.New("job must subscribe to at least one event domain (case or scheduled)")
	}
	if e.Case != nil {
		if err := e.Case.Validate(); err != nil {
			return goerr.Wrap(err, "events.case is invalid")
		}
	}
	if e.Scheduled != nil {
		if err := e.Scheduled.Validate(); err != nil {
			return goerr.Wrap(err, "events.scheduled is invalid")
		}
	}
	return nil
}

// Job is a workspace-scoped, declaratively configured agent that runs in
// response to events. Jobs are loaded from workspace TOML and held in
// memory on the WorkspaceEntry — they are not persisted to a backend.
type Job struct {
	// ID is the workspace-unique identifier (kebab-case).
	ID string

	// Name and Description are human-facing labels used in logs and
	// system prompts.
	Name        string
	Description string

	// Prompt is the user prompt template (Go text/template). Job-specific
	// behaviour is fully expressed here; runtime context (Case / Workspace
	// / Event) is injected as template data.
	Prompt string

	// Disabled defaults to false (= active). Setting it true silently
	// excludes the Job from event matching without removing the TOML
	// entry.
	Disabled bool

	// Events maps the event domains this Job subscribes to. Validate
	// guarantees at least one non-nil entry.
	Events JobEvents
}

// ListensCase reports whether the Job subscribes to the given case lifecycle.
func (j *Job) ListensCase(lc CaseLifecycle) bool {
	if j == nil || j.Disabled {
		return false
	}
	return j.Events.Case.Matches(lc)
}

// ListensScheduled reports whether the Job subscribes to the scheduled domain.
func (j *Job) ListensScheduled() bool {
	if j == nil || j.Disabled {
		return false
	}
	return j.Events.Scheduled != nil
}

// Validate enforces the full Job invariant set: identity, prompt presence,
// and event-map well-formedness.
func (j *Job) Validate() error {
	if j == nil {
		return goerr.New("job is nil")
	}
	if j.ID == "" {
		return goerr.New("job id is empty")
	}
	if j.Prompt == "" {
		return goerr.New("job prompt is empty",
			goerr.V("job_id", j.ID))
	}
	if err := j.Events.Validate(); err != nil {
		return goerr.Wrap(err, "job events invalid",
			goerr.V("job_id", j.ID))
	}
	return nil
}
