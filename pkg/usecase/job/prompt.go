package job

import (
	"bytes"
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

//go:embed prompts/system.md
var systemPromptTmplSource string

var (
	systemPromptOnce sync.Once
	systemPromptTmpl *template.Template
	systemPromptErr  error
)

// promptFuncs are the template helpers exposed to the system / user
// prompt templates. Kept minimal — prompts are trusted-author surfaces,
// but we still avoid handing them filesystem / network helpers.
var promptFuncs = template.FuncMap{
	"join": strings.Join,
}

func loadSystemPromptTemplate() (*template.Template, error) {
	systemPromptOnce.Do(func() {
		systemPromptTmpl, systemPromptErr = template.
			New("job-system").
			Funcs(promptFuncs).
			Parse(systemPromptTmplSource)
	})
	if systemPromptErr != nil {
		return nil, goerr.Wrap(systemPromptErr, "parse embedded job system prompt")
	}
	return systemPromptTmpl, nil
}

// PromptInputs bundles the runtime context needed to render the system
// prompt and the user prompt for a single Job run.
type PromptInputs struct {
	Job       *model.Job
	Workspace *model.WorkspaceEntry
	Case      *model.Case
	Actions   []*model.Action
	Event     Event
	Now       time.Time

	// Sources is the resolved set of *model.Source the agent should be
	// aware of for this Case. It is always populated when source
	// listing succeeds — even when the operator has not narrowed the
	// selection — so the LLM has the full catalogue of investigation
	// sources at hand. The distinction "operator narrowed vs. workspace
	// default" is carried separately on SourcesNarrowed so the system
	// prompt can phrase the section accordingly.
	Sources []*model.Source

	// SourcesNarrowed reports whether the Sources slice reflects an
	// explicit operator selection from the Case Agent page
	// (Case.AgentSourceIDs non-empty). When true the system prompt
	// labels the list as a *preference*; when false it labels the list
	// as the full available catalogue and explicitly states no narrowing
	// is in effect. Either way the agent is never told to restrict its
	// search — selection is a hint, not a filter.
	SourcesNarrowed bool
}

// systemPromptData is the typed dot value passed into the system prompt
// template. All template branches read from this single struct so adding
// a new section is a focused change: add a field, add a `{{ if }}` block.
type systemPromptData struct {
	Workspace systemPromptWorkspace
	Case      *systemPromptCase
	Actions   []systemPromptAction
	Trigger   systemPromptTrigger
	Reason    systemPromptReason
	Sources   systemPromptSourceSection
}

type systemPromptSourceSection struct {
	Items    []systemPromptSource
	Narrowed bool
}

type systemPromptSource struct {
	ID          string
	Name        string
	Type        string
	Description string
}

type systemPromptWorkspace struct {
	ID          string
	Name        string
	Description string
	Fields      []systemPromptField
}

type systemPromptField struct {
	ID, Name, Type string
}

type systemPromptCase struct {
	ID                    int64
	Title                 string
	Description           string
	Status                string
	ReporterID            string
	AssigneeIDs           []string
	SlackChannelID        string
	CreatedAt             string
	UpdatedAt             string
	FieldValues           []systemPromptFieldValue
	AgentAdditionalPrompt string
}

type systemPromptFieldValue struct {
	ID    string
	Value string
}

type systemPromptAction struct {
	ID         int64
	Title      string
	Status     string
	AssigneeID string
}

type systemPromptTrigger struct {
	CaseLifecycles []systemPromptLifecycle
	ScheduledEvery string
	ScheduledCron  string
}

type systemPromptLifecycle struct {
	Name        string
	Description string
}

type systemPromptReason struct {
	CaseCreated    bool
	CaseClosed     bool
	ScheduledEvery string
	ScheduledCron  string
	CaseID         int64
	Actor          string
	Timestamp      string
	LastRunAt      string
	ScheduledFor   string
	Elapsed        string
}

// BuildSystemPrompt assembles the structured system prompt the Job agent
// receives. Section content is fixed by the embedded `prompts/system.md`
// template; this function only marshals PromptInputs into the typed
// template data.
func BuildSystemPrompt(in PromptInputs) (string, error) {
	tmpl, err := loadSystemPromptTemplate()
	if err != nil {
		return "", err
	}
	data := buildSystemPromptData(in)
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", goerr.Wrap(err, "execute job system prompt template")
	}
	return buf.String(), nil
}

func buildSystemPromptData(in PromptInputs) systemPromptData {
	data := systemPromptData{}

	if ws := in.Workspace; ws != nil {
		data.Workspace = systemPromptWorkspace{
			ID:          ws.Workspace.ID,
			Name:        ws.Workspace.Name,
			Description: ws.Workspace.Description,
		}
		if schema := ws.FieldSchema; schema != nil {
			for _, f := range schema.Fields {
				data.Workspace.Fields = append(data.Workspace.Fields, systemPromptField{
					ID:   f.ID,
					Name: f.Name,
					Type: string(f.Type),
				})
			}
		}
	}

	if c := in.Case; c != nil {
		cs := &systemPromptCase{
			ID:                    c.ID,
			Title:                 c.Title,
			Description:           c.Description,
			Status:                c.Status.String(),
			ReporterID:            c.ReporterID,
			AssigneeIDs:           append([]string(nil), c.AssigneeIDs...),
			SlackChannelID:        c.SlackChannelID,
			AgentAdditionalPrompt: c.AgentAdditionalPrompt,
		}
		if !c.CreatedAt.IsZero() {
			cs.CreatedAt = c.CreatedAt.UTC().Format(time.RFC3339)
		}
		if !c.UpdatedAt.IsZero() {
			cs.UpdatedAt = c.UpdatedAt.UTC().Format(time.RFC3339)
		}
		if len(c.FieldValues) > 0 {
			keys := make([]string, 0, len(c.FieldValues))
			for k := range c.FieldValues {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				v := c.FieldValues[k]
				cs.FieldValues = append(cs.FieldValues, systemPromptFieldValue{
					ID:    k,
					Value: fmt.Sprint(v.Value),
				})
			}
		}
		data.Case = cs
	}

	for _, a := range in.Actions {
		if a == nil || a.IsArchived() {
			continue
		}
		data.Actions = append(data.Actions, systemPromptAction{
			ID:         a.ID,
			Title:      a.Title,
			Status:     a.Status.String(),
			AssigneeID: a.AssigneeID,
		})
	}

	data.Sources.Narrowed = in.SourcesNarrowed
	for _, s := range in.Sources {
		if s == nil {
			continue
		}
		data.Sources.Items = append(data.Sources.Items, systemPromptSource{
			ID:          string(s.ID),
			Name:        s.Name,
			Type:        string(s.SourceType),
			Description: s.Description,
		})
	}

	if in.Job != nil {
		if cc := in.Job.Events.Case; cc != nil {
			for _, lc := range cc.On {
				data.Trigger.CaseLifecycles = append(data.Trigger.CaseLifecycles, systemPromptLifecycle{
					Name:        string(lc),
					Description: describeCaseLifecycle(lc),
				})
			}
		}
		if sc := in.Job.Events.Scheduled; sc != nil {
			switch {
			case sc.Every > 0:
				data.Trigger.ScheduledEvery = sc.Every.String()
			case sc.Cron != nil:
				data.Trigger.ScheduledCron = sc.CronExpr
			}
		}
	}

	switch in.Event.Domain {
	case model.JobEventDomainCase:
		actor := in.Event.ActorUserID
		if actor == "" {
			actor = "(unknown)"
		}
		data.Reason.Actor = actor
		data.Reason.CaseID = in.Event.CaseID
		data.Reason.Timestamp = in.Event.Timestamp.UTC().Format(time.RFC3339)
		switch in.Event.CaseLifecycle {
		case model.CaseLifecycleCreated:
			data.Reason.CaseCreated = true
		case model.CaseLifecycleClosed:
			data.Reason.CaseClosed = true
		}
	case model.JobEventDomainScheduled:
		data.Reason.Timestamp = in.Event.Timestamp.UTC().Format(time.RFC3339)
		data.Reason.LastRunAt = "(never)"
		if !in.Event.LastRunAt.IsZero() {
			data.Reason.LastRunAt = in.Event.LastRunAt.UTC().Format(time.RFC3339)
		}
		if in.Job != nil && in.Job.Events.Scheduled != nil {
			sc := in.Job.Events.Scheduled
			switch {
			case sc.Every > 0:
				data.Reason.ScheduledEvery = sc.Every.String()
				if !in.Event.LastRunAt.IsZero() {
					data.Reason.Elapsed = in.Event.Timestamp.Sub(in.Event.LastRunAt).String()
				} else {
					data.Reason.Elapsed = "?"
				}
			case sc.Cron != nil:
				data.Reason.ScheduledCron = sc.CronExpr
				if !in.Event.ScheduledFor.IsZero() {
					data.Reason.ScheduledFor = in.Event.ScheduledFor.UTC().Format(time.RFC3339)
				} else {
					data.Reason.ScheduledFor = "(none)"
				}
			}
		}
	}

	return data
}

func describeCaseLifecycle(lc model.CaseLifecycle) string {
	switch lc {
	case model.CaseLifecycleCreated:
		return "a new case is created"
	case model.CaseLifecycleClosed:
		return "the case status transitions to CLOSED"
	default:
		return string(lc)
	}
}

var (
	userPromptCache sync.Map // key: job ID, value: *template.Template
)

// RenderUserPrompt renders the Job's `prompt` field as a Go template
// using PromptInputs as the dot value. Templates are cached per Job ID
// to avoid re-parsing on every invocation.
func RenderUserPrompt(in PromptInputs) (string, error) {
	if in.Job == nil {
		return "", goerr.New("job is nil")
	}
	tmpl, err := userTemplateFor(in.Job)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, in); err != nil {
		return "", goerr.Wrap(err, "render job prompt template",
			goerr.V("job_id", in.Job.ID))
	}
	return buf.String(), nil
}

func userTemplateFor(j *model.Job) (*template.Template, error) {
	if cached, ok := userPromptCache.Load(j.ID); ok {
		return cached.(*template.Template), nil
	}
	tmpl, err := template.New("job-user-" + j.ID).Funcs(promptFuncs).Parse(j.Prompt)
	if err != nil {
		return nil, goerr.Wrap(err, "parse job prompt template",
			goerr.V("job_id", j.ID))
	}
	userPromptCache.Store(j.ID, tmpl)
	return tmpl, nil
}
