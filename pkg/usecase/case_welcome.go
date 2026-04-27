package usecase

import (
	"bytes"
	"context"
	"text/template"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
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
// (e.g., {{.Fields.severity}}).
type welcomeContext struct {
	Case      *model.Case
	Workspace model.Workspace
	Fields    map[string]any
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
// to templates. The returned map keys are the FieldIDs, and values are the
// raw field values stored on the Case.
func buildWelcomeFields(c *model.Case) map[string]any {
	if c == nil || len(c.FieldValues) == 0 {
		return nil
	}
	fields := make(map[string]any, len(c.FieldValues))
	for id, fv := range c.FieldValues {
		fields[id] = fv.Value
	}
	return fields
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

	var workspace model.Workspace
	if uc.workspaceRegistry != nil {
		if entry, err := uc.workspaceRegistry.Get(workspaceID); err == nil {
			workspace = entry.Workspace
		}
	}

	rendered, err := renderer.Render(welcomeContext{
		Case:      c,
		Workspace: workspace,
		Fields:    buildWelcomeFields(c),
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
