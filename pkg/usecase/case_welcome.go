package usecase

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// welcomeRenderer pre-parses Slack welcome message templates so that they can
// be evaluated quickly when a Case is created. A nil receiver returns no
// messages, which is convenient for workspaces without configured welcome
// messages.
type welcomeRenderer struct {
	templates []*template.Template
}

// welcomeContext is the data exposed to welcome message templates.
//
// Fields uses string keys that match the Field IDs defined in the workspace's
// config.toml. Because Field IDs follow the ^[a-z][a-z0-9_]*$ pattern, they
// are valid Go identifiers and can be referenced via dot notation in templates
// (e.g., {{.Fields.severity.id}}). Each field value is enriched with both an
// `id` (raw stored value or option ID) and a `name` (display name resolved via
// the workspace FieldSchema for select/multi-select fields).
type welcomeContext struct {
	Case      *model.Case
	Workspace model.Workspace
	Fields    map[string]map[string]any
	URL       string
}

// newWelcomeRenderer parses the given message templates. An empty or nil
// messages slice yields a nil renderer, which Render treats as a no-op.
func newWelcomeRenderer(messages []string) (*welcomeRenderer, error) {
	if len(messages) == 0 {
		return nil, nil
	}

	parsed := make([]*template.Template, 0, len(messages))
	for i, msg := range messages {
		tmpl, err := template.New("welcome").Parse(msg)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to parse welcome message template",
				goerr.V("index", i),
				goerr.V("template", msg),
			)
		}
		parsed = append(parsed, tmpl)
	}
	return &welcomeRenderer{templates: parsed}, nil
}

// Render evaluates every template against ctx and returns the resulting
// strings in original order. Empty results (after template execution) are
// dropped so that a conditional template can be used to suppress a message.
func (r *welcomeRenderer) Render(ctx welcomeContext) ([]string, error) {
	if r == nil || len(r.templates) == 0 {
		return nil, nil
	}

	out := make([]string, 0, len(r.templates))
	for i, tmpl := range r.templates {
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, ctx); err != nil {
			return out, goerr.Wrap(err, "failed to execute welcome message template",
				goerr.V("index", i),
			)
		}
		rendered := buf.String()
		if rendered == "" {
			continue
		}
		out = append(out, rendered)
	}
	return out, nil
}

// buildWelcomeFields converts a Case's FieldValues map into the shape exposed
// to templates. Each entry has an `id` (raw stored value, or option ID for
// select/multi-select) and a `name` (display name resolved via the workspace
// FieldSchema for select/multi-select; for other types `name` mirrors `id`).
// Multi-select fields additionally expose an `items` slice for iteration in
// templates.
//
// schema may be nil; in that case all fields fall back to the bare-value
// representation for both id and name.
func buildWelcomeFields(c *model.Case, schema *config.FieldSchema) map[string]map[string]any {
	if c == nil || len(c.FieldValues) == 0 {
		return nil
	}

	defs := make(map[string]config.FieldDefinition)
	if schema != nil {
		for _, def := range schema.Fields {
			defs[def.ID] = def
		}
	}

	fields := make(map[string]map[string]any, len(c.FieldValues))
	for id, fv := range c.FieldValues {
		fields[id] = welcomeFieldEntry(fv, defs[id])
	}
	return fields
}

// welcomeFieldEntry builds the `{id, name, items?}` map for a single field.
// def may be a zero-value FieldDefinition (when the field is not in the
// schema), in which case the raw value is used for both id and name.
func welcomeFieldEntry(fv model.FieldValue, def config.FieldDefinition) map[string]any {
	switch def.Type {
	case types.FieldTypeSelect:
		idStr := stringifyValue(fv.Value)
		return map[string]any{
			"id":   idStr,
			"name": optionNameByID(def.Options, idStr),
		}
	case types.FieldTypeMultiSelect:
		ids := stringifyMultiValue(fv.Value)
		names := make([]string, len(ids))
		items := make([]map[string]string, len(ids))
		for i, id := range ids {
			name := optionNameByID(def.Options, id)
			names[i] = name
			items[i] = map[string]string{"id": id, "name": name}
		}
		return map[string]any{
			"id":    ids,
			"name":  names,
			"items": items,
		}
	default:
		raw := fv.Value
		return map[string]any{
			"id":   raw,
			"name": raw,
		}
	}
}

// optionNameByID returns the option's display name for the given option ID.
// Falls back to the ID itself when no matching option is found, so that the
// template still produces a meaningful string.
func optionNameByID(options []config.FieldOption, id string) string {
	for _, opt := range options {
		if opt.ID == id {
			return opt.Name
		}
	}
	return id
}

// stringifyValue converts the raw stored value of a select field to the option
// ID string. Returns the empty string for unsupported types.
func stringifyValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}

// stringifyMultiValue normalizes the raw stored value of a multi-select field
// (which may be []string or []any) into []string of option IDs.
func stringifyMultiValue(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, elem := range x {
			out = append(out, stringifyValue(elem))
		}
		return out
	case nil:
		return nil
	default:
		return []string{stringifyValue(v)}
	}
}

// postWelcomeMessages renders and posts the workspace's configured welcome
// messages to the freshly-created case channel. Failures are reported via
// errutil.Handle and never propagate, since welcome messages are auxiliary to
// case creation and the channel is already in place by the time this runs.
func (uc *CaseUseCase) postWelcomeMessages(ctx context.Context, workspaceID string, c *model.Case, channelID, caseURL string) {
	if uc.slackService == nil || c == nil || channelID == "" {
		return
	}

	renderer := uc.welcomeRenderers[workspaceID]
	if renderer == nil {
		return
	}

	var (
		workspace model.Workspace
		schema    *config.FieldSchema
	)
	if uc.workspaceRegistry != nil {
		if entry, err := uc.workspaceRegistry.Get(workspaceID); err == nil {
			workspace = entry.Workspace
			schema = entry.FieldSchema
		}
	}

	rendered, err := renderer.Render(welcomeContext{
		Case:      c,
		Workspace: workspace,
		Fields:    buildWelcomeFields(c, schema),
		URL:       caseURL,
	})
	if err != nil {
		errutil.Handle(ctx, err, "failed to render Slack welcome messages")
		// Continue with whatever messages were rendered before the error.
	}

	for i, text := range rendered {
		if _, postErr := uc.slackService.PostMessage(ctx, channelID, nil, text); postErr != nil {
			errutil.Handle(ctx, goerr.Wrap(postErr, "failed to post Slack welcome message",
				goerr.V("workspace_id", workspaceID),
				goerr.V("channel_id", channelID),
				goerr.V("index", i),
			), "failed to post Slack welcome message")
		}
	}
}
