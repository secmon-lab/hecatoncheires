package config

import (
	"regexp"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/robfig/cron/v3"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// jobIDPattern matches kebab-case identifiers. Job IDs are surfaced in
// system prompts and logging, so we keep them human-readable.
var jobIDPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// cronParser uses the standard 5-field schedule (minute hour dom month dow)
// in UTC. Anything more exotic (seconds, descriptors) is rejected at load
// time.
var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// JobSection is the TOML shape of a [[job]] entry. All event-shape
// validation runs at config load time: the runtime layer
// (pkg/usecase/job) trusts that every Job it receives has already been
// vetted.
type JobSection struct {
	ID          string           `toml:"id"`
	Name        string           `toml:"name"`
	Description string           `toml:"description"`
	Prompt      string           `toml:"prompt"`
	Disabled    bool             `toml:"disabled"`
	Events      JobEventsSection `toml:"events"`
}

// JobEventsSection mirrors the `events.<domain> = { ... }` map. At least
// one sub-domain pointer must be non-nil; both may be set simultaneously.
type JobEventsSection struct {
	Case      *CaseEventSection      `toml:"case"`
	Scheduled *ScheduledEventSection `toml:"scheduled"`
}

// CaseEventSection is the filter for `events.case`. The TOML `on` field is
// always an array; we deliberately do not accept a single string so the
// schema stays type-safe.
type CaseEventSection struct {
	On []string `toml:"on"`
}

// ScheduledEventSection is the filter for `events.scheduled`. Exactly one
// of Every / Cron is required.
type ScheduledEventSection struct {
	Every string `toml:"every"`
	Cron  string `toml:"cron"`
}

// Validate parses and validates a single JobSection, returning a fully
// resolved model.Job on success. Returns an error wrapped with the job
// index so the caller can include it in its error message.
func (s *JobSection) Validate() (*model.Job, error) {
	if s == nil {
		return nil, goerr.New("job section is nil")
	}
	if s.ID == "" {
		return nil, goerr.New("job id is required")
	}
	if !jobIDPattern.MatchString(s.ID) {
		return nil, goerr.New("job id must be kebab-case (^[a-z0-9]+(-[a-z0-9]+)*$)",
			goerr.V("job_id", s.ID))
	}
	if s.Prompt == "" {
		return nil, goerr.New("job prompt is required",
			goerr.V("job_id", s.ID))
	}

	events, err := s.Events.toModel()
	if err != nil {
		return nil, goerr.Wrap(err, "invalid job events",
			goerr.V("job_id", s.ID))
	}

	job := &model.Job{
		ID:          s.ID,
		Name:        s.Name,
		Description: s.Description,
		Prompt:      s.Prompt,
		Disabled:    s.Disabled,
		Events:      *events,
	}
	if err := job.Validate(); err != nil {
		return nil, goerr.Wrap(err, "job invariant violation",
			goerr.V("job_id", s.ID))
	}
	return job, nil
}

func (e *JobEventsSection) toModel() (*model.JobEvents, error) {
	if e == nil {
		return nil, goerr.New("events section is nil")
	}
	if e.Case == nil && e.Scheduled == nil {
		return nil, goerr.New("events must subscribe to at least one domain (case or scheduled)")
	}

	out := &model.JobEvents{}

	if e.Case != nil {
		caseCfg, err := e.Case.toModel()
		if err != nil {
			return nil, goerr.Wrap(err, "invalid events.case")
		}
		out.Case = caseCfg
	}
	if e.Scheduled != nil {
		schedCfg, err := e.Scheduled.toModel()
		if err != nil {
			return nil, goerr.Wrap(err, "invalid events.scheduled")
		}
		out.Scheduled = schedCfg
	}
	return out, nil
}

func (c *CaseEventSection) toModel() (*model.CaseEventConfig, error) {
	if c == nil {
		return nil, goerr.New("case event section is nil")
	}
	if len(c.On) == 0 {
		return nil, goerr.New("events.case.on must not be empty")
	}
	seen := make(map[model.CaseLifecycle]struct{}, len(c.On))
	on := make([]model.CaseLifecycle, 0, len(c.On))
	for _, raw := range c.On {
		lc := model.CaseLifecycle(raw)
		if !lc.IsValid() {
			return nil, goerr.New("invalid value in events.case.on",
				goerr.V("value", raw))
		}
		if _, dup := seen[lc]; dup {
			return nil, goerr.New("duplicate value in events.case.on",
				goerr.V("value", raw))
		}
		seen[lc] = struct{}{}
		on = append(on, lc)
	}
	return &model.CaseEventConfig{On: on}, nil
}

func (s *ScheduledEventSection) toModel() (*model.ScheduledEventConfig, error) {
	if s == nil {
		return nil, goerr.New("scheduled event section is nil")
	}
	hasEvery := s.Every != ""
	hasCron := s.Cron != ""
	switch {
	case hasEvery && hasCron:
		return nil, goerr.New("events.scheduled must specify exactly one of every or cron, not both")
	case !hasEvery && !hasCron:
		return nil, goerr.New("events.scheduled must specify either every or cron")
	}

	out := &model.ScheduledEventConfig{}
	if hasEvery {
		d, err := time.ParseDuration(s.Every)
		if err != nil {
			return nil, goerr.Wrap(err, "invalid every duration",
				goerr.V("every", s.Every))
		}
		if d <= 0 {
			return nil, goerr.New("every must be positive",
				goerr.V("every", s.Every))
		}
		out.Every = d
	}
	if hasCron {
		sched, err := cronParser.Parse(s.Cron)
		if err != nil {
			return nil, goerr.Wrap(err, "invalid cron expression",
				goerr.V("cron", s.Cron))
		}
		out.Cron = sched
		out.CronExpr = s.Cron
	}
	return out, nil
}

// resolveJobs walks every [[job]] entry, validating it and collecting the
// resulting model.Jobs. Duplicate IDs within the workspace surface as a
// loud failure.
func (a *AppConfig) resolveJobs() ([]*model.Job, error) {
	if len(a.Jobs) == 0 {
		return nil, nil
	}
	jobs := make([]*model.Job, 0, len(a.Jobs))
	seen := make(map[string]struct{}, len(a.Jobs))
	for idx := range a.Jobs {
		section := &a.Jobs[idx]
		job, err := section.Validate()
		if err != nil {
			return nil, goerr.Wrap(err, "invalid job",
				goerr.V("job_index", idx))
		}
		if _, dup := seen[job.ID]; dup {
			return nil, goerr.New("duplicate job id within workspace",
				goerr.V("job_id", job.ID))
		}
		seen[job.ID] = struct{}{}
		jobs = append(jobs, job)
	}
	return jobs, nil
}
