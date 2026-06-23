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
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
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
	// Memos is the set of ACTIVE memos for the Case (the agent's working
	// memory). The system prompt embeds at most memoSystemPromptMax id+title
	// pairs and, when there are more, the total count; full content is fetched
	// on demand via the memo tools.
	Memos []*model.Memo

	// RecentMessages is the thread's recent Slack messages, oldest first,
	// already bounded by the caller to the last recentMessageWindow and at
	// most recentMessageMaxCount items. It is populated only for thread-mode
	// workspaces (channel-mode Jobs leave it nil); the system prompt renders
	// the section only when the workspace manages no Actions.
	RecentMessages []*slack.Message

	Event Event
	Now   time.Time

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
	// Now is the turn's start time (RFC3339, UTC), rendered as the
	// agent's absolute "current time". Empty when PromptInputs.Now is
	// the zero value so the template can omit the section entirely.
	Now       string
	Workspace systemPromptWorkspace
	Case      *systemPromptCase
	// ManagesActions is false for thread-mode workspaces, which manage no
	// Actions. When false the template omits the Actions section and the
	// action-specific guardrails so the prompt never references a concept the
	// agent has no tools for.
	ManagesActions bool
	Actions        []systemPromptAction
	Memo           systemPromptMemoSection
	// RecentMessages is the thread's recent Slack messages, oldest first.
	// Only populated for thread-mode workspaces (ManagesActions == false);
	// the template gates the whole section on ManagesActions and shows
	// "(none)" when the slice is empty.
	RecentMessages []systemPromptMessage
	Trigger        systemPromptTrigger
	Reason         systemPromptReason
	Sources        systemPromptSourceSection
}

// recentMessageTruncateRunes bounds how many runes of each Slack message body
// the system prompt embeds. Bodies longer than this are truncated and their
// full rune count is annotated so the agent knows content was elided. Kept as
// a fixed feature parameter (not configurable) per the spec.
const recentMessageTruncateRunes = 140

// systemPromptMessage is one recent thread message, formatted for the system
// prompt. Text is already truncated to recentMessageTruncateRunes; FullRuneCount
// carries the original rune count only when truncation occurred (0 otherwise),
// so the template can append the "[N chars total]" annotation conditionally.
type systemPromptMessage struct {
	Timestamp     string
	Author        string
	Text          string
	FullRuneCount int
}

// memoSystemPromptMax bounds how many memo id+title pairs are embedded in the
// system prompt. Beyond this the prompt reports only the total count and the
// agent fetches details with the memo tools — this keeps the prompt bounded
// even for cases that accumulate many memories.
const memoSystemPromptMax = 20

// systemPromptMemoSection carries the workspace memo definition, the memo field
// schema, and a bounded preview of the case's active memos.
type systemPromptMemoSection struct {
	Enabled    bool
	Definition string
	Fields     []systemPromptField
	Items      []systemPromptMemoBrief
	TotalCount int
	Overflow   bool
}

type systemPromptMemoBrief struct {
	ID    string
	Title string
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
	ID          string
	Name        string
	Type        string
	Required    bool
	Description string
	Options     []systemPromptFieldOption
}

type systemPromptFieldOption struct {
	ID          string
	Name        string
	Description string
	Metadata    []systemPromptKV
}

type systemPromptKV struct {
	Key   string
	Value string
}

type systemPromptCase struct {
	ID                    int64
	Title                 string
	Description           string
	Status                string
	ReporterID            string
	AssigneeIDs           []string
	SlackChannelID        string
	SlackThreadTS         string
	CreatedAt             string
	UpdatedAt             string
	FieldValues           []systemPromptFieldValue
	AgentAdditionalPrompt string
}

type systemPromptFieldValue struct {
	ID    string
	Value string
	// Resolved is populated when the field type is select / multi-select
	// and at least one raw element of Value matched a known option ID.
	// select produces at most one entry, multi-select may produce many.
	Resolved []systemPromptFieldValueOption
}

type systemPromptFieldValueOption struct {
	OptionID   string
	OptionName string
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

	// Thread-mode workspaces manage no Actions; the Job agent gets no action
	// tools there, so the prompt must not advertise an Actions section or
	// action guardrails. nil workspace defaults to channel-mode (actions on).
	data.ManagesActions = in.Workspace == nil || !in.Workspace.IsThreadMode()

	// Now is the turn's start time (runner.go injects r.clock()); a zero
	// value means the caller did not supply it, so the section is skipped
	// rather than rendering a bogus "0001-01-01" timestamp.
	if !in.Now.IsZero() {
		data.Now = in.Now.UTC().Format(time.RFC3339)
	}

	// fieldMetaByID lets the Case loop below look up field type and
	// option metadata when rendering field_values. It is built once
	// from the workspace schema and shared across both sections so the
	// custom-fields description and the field_values resolution stay
	// in sync.
	fieldMetaByID := map[string]fieldMeta{}

	if ws := in.Workspace; ws != nil {
		data.Workspace = systemPromptWorkspace{
			ID:          ws.Workspace.ID,
			Name:        ws.Workspace.Name,
			Description: ws.Workspace.Description,
		}
		if schema := ws.FieldSchema; schema != nil {
			for _, f := range schema.Fields {
				meta := fieldMeta{fieldType: f.Type}
				field := systemPromptField{
					ID:          f.ID,
					Name:        f.Name,
					Type:        string(f.Type),
					Required:    f.Required,
					Description: f.Description,
				}
				if len(f.Options) > 0 {
					meta.options = make(map[string]config.FieldOption, len(f.Options))
				}
				for _, o := range f.Options {
					meta.options[o.ID] = o
					field.Options = append(field.Options, systemPromptFieldOption{
						ID:          o.ID,
						Name:        o.Name,
						Description: o.Description,
						Metadata:    fieldOptionMetadata(o.Metadata),
					})
				}
				fieldMetaByID[f.ID] = meta
				data.Workspace.Fields = append(data.Workspace.Fields, field)
			}
		}

		// Memo section: the workspace's strong definition + memo field schema.
		// Rendered only when the workspace enabled memos.
		if ws.MemoConfig.Enabled() {
			data.Memo.Enabled = true
			data.Memo.Definition = ws.MemoConfig.Description
			for _, f := range ws.MemoConfig.FieldSchema.Fields {
				field := systemPromptField{
					ID:          f.ID,
					Name:        f.Name,
					Type:        string(f.Type),
					Required:    f.Required,
					Description: f.Description,
				}
				for _, o := range f.Options {
					field.Options = append(field.Options, systemPromptFieldOption{
						ID:          o.ID,
						Name:        o.Name,
						Description: o.Description,
						Metadata:    fieldOptionMetadata(o.Metadata),
					})
				}
				data.Memo.Fields = append(data.Memo.Fields, field)
			}
		}
	}

	// Memo preview: at most memoSystemPromptMax id+title pairs, plus the total
	// count when there are more (the agent reads the rest via the memo tools).
	if data.Memo.Enabled {
		data.Memo.TotalCount = len(in.Memos)
		for i, m := range in.Memos {
			if m == nil {
				continue
			}
			if i >= memoSystemPromptMax {
				data.Memo.Overflow = true
				break
			}
			data.Memo.Items = append(data.Memo.Items, systemPromptMemoBrief{
				ID:    string(m.ID),
				Title: m.Title,
			})
		}
	}

	// Recent thread messages: thread-mode only. The caller has already
	// bounded the slice to the last recentMessageWindow and recentMessageMaxCount
	// and ordered it oldest-first; here we only format and rune-truncate each
	// body. Channel-mode workspaces (ManagesActions) never reach this block, so
	// the section is absent from their prompt entirely.
	if !data.ManagesActions {
		for _, m := range in.RecentMessages {
			if m == nil {
				continue
			}
			author := m.UserName()
			if author == "" {
				author = m.UserID()
			}
			ts := ""
			if t := m.CreatedAt(); !t.IsZero() {
				ts = t.UTC().Format(time.RFC3339)
			}
			text, fullCount := truncateRunes(m.Text(), recentMessageTruncateRunes)
			data.RecentMessages = append(data.RecentMessages, systemPromptMessage{
				Timestamp:     ts,
				Author:        author,
				Text:          text,
				FullRuneCount: fullCount,
			})
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
			SlackThreadTS:         c.SlackThreadTS,
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
				meta := fieldMetaByID[k]
				cs.FieldValues = append(cs.FieldValues, systemPromptFieldValue{
					ID:       k,
					Value:    formatFieldValue(meta.fieldType, v.Value),
					Resolved: resolveFieldValueOptions(meta, v.Value),
				})
			}
		}
		data.Case = cs
	}

	// Skip the Actions section entirely for workspaces that manage no Actions
	// (thread-mode); the template gates on ManagesActions too, but leaving the
	// slice empty keeps the rendered prompt honest if that guard ever changes.
	if data.ManagesActions {
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

// fieldMeta is the small lookup shape buildSystemPromptData uses to
// bridge the workspace FieldSchema and the Case.FieldValues loop. It
// holds the field's declared type and an option index keyed by option
// ID. Kept package-private — internal scaffolding for prompt rendering.
type fieldMeta struct {
	fieldType types.FieldType
	options   map[string]config.FieldOption
}

// fieldOptionMetadata flattens the option's freeform metadata map into
// a stable, sorted slice of key/value pairs so the rendered prompt is
// deterministic across runs. Empty / nil input returns nil so the
// template's {{ if .Metadata }} guard short-circuits cleanly.
func fieldOptionMetadata(meta map[string]any) []systemPromptKV {
	if len(meta) == 0 {
		return nil
	}
	keys := make([]string, 0, len(meta))
	for k := range meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]systemPromptKV, 0, len(keys))
	for _, k := range keys {
		out = append(out, systemPromptKV{Key: k, Value: fmt.Sprint(meta[k])})
	}
	return out
}

// formatFieldValue renders the raw field value as a single human-
// readable string for the prompt. multi-select inputs (whether
// `[]string` or `[]any` carrying strings) are joined with ", " so the
// rendered line stays compact; everything else falls back to fmt.Sprint
// which already matches the previous behaviour.
func formatFieldValue(ft types.FieldType, raw any) string {
	if ft == types.FieldTypeMultiSelect {
		if parts, ok := multiSelectIDs(raw); ok {
			return strings.Join(parts, ", ")
		}
	}
	return fmt.Sprint(raw)
}

// resolveFieldValueOptions returns the option metadata for every
// element of the raw value that matched a known option ID. For
// non-select fields (or when no option matches) it returns nil so the
// template's {{ if .Resolved }} guard skips the parenthetical.
func resolveFieldValueOptions(meta fieldMeta, raw any) []systemPromptFieldValueOption {
	switch meta.fieldType {
	case types.FieldTypeSelect:
		s, ok := raw.(string)
		if !ok {
			return nil
		}
		opt, ok := meta.options[s]
		if !ok {
			return nil
		}
		return []systemPromptFieldValueOption{{OptionID: opt.ID, OptionName: opt.Name}}
	case types.FieldTypeMultiSelect:
		ids, ok := multiSelectIDs(raw)
		if !ok {
			return nil
		}
		var out []systemPromptFieldValueOption
		for _, id := range ids {
			opt, ok := meta.options[id]
			if !ok {
				continue
			}
			out = append(out, systemPromptFieldValueOption{OptionID: opt.ID, OptionName: opt.Name})
		}
		return out
	default:
		return nil
	}
}

// multiSelectIDs normalises the two on-the-wire shapes for a multi-
// select value (`[]string` from Go callers, `[]any` from Firestore
// JSON decoding) into a single `[]string`. The bool return signals
// whether the input matched a recognised multi-select shape so the
// caller can fall back to the generic fmt.Sprint path.
func multiSelectIDs(raw any) ([]string, bool) {
	switch v := raw.(type) {
	case []string:
		return v, true
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

// truncateRunes caps s at max runes (not bytes — the spec counts 文字), so a
// multi-byte body is never split mid-character the way a byte cap would. It
// returns the truncated string and, when truncation occurred, the original
// rune count so the caller can annotate the elision; when s already fits, the
// second return is 0 to signal "no annotation needed".
//
// Single pass: ranging over a string yields rune-boundary byte indices, so we
// record the cut point at the (max+1)-th rune and keep counting to the end —
// the full rune count falls out of the same loop, avoiding a separate
// utf8.RuneCountInString scan.
func truncateRunes(s string, max int) (string, int) {
	if max <= 0 {
		return "", 0
	}
	cut := -1
	count := 0
	for i := range s {
		if count == max {
			cut = i
		}
		count++
	}
	if cut < 0 {
		return s, 0
	}
	return s[:cut], count
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
