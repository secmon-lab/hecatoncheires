package wsagent

import (
	_ "embed"
	"strings"
	"sync"
	"text/template"

	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

//go:embed prompts/system.md
var systemPromptTmplSrc string

var (
	systemPromptOnce sync.Once
	systemPromptTmpl *template.Template
	systemPromptErr  error
)

// systemPromptInput is the single typed input for prompts/system.md.
type systemPromptInput struct {
	// WorkspaceName is the workspace display name, falling back to its ID.
	// Empty when neither is known; the template then omits the name.
	WorkspaceName string
	// ThreadMode selects the thread-mode paragraph: cases are Slack threads,
	// no Actions exist, and finishing a case means a board-status move.
	ThreadMode bool
	// BoardStatuses lists the configured case board status ids. Non-empty only
	// in thread mode.
	BoardStatuses []string
	// CustomPrompt is the operator-supplied [slack.workspace_agent] prompt. The
	// template appends it last so it cannot relax the safety rule above it.
	CustomPrompt string
}

// buildSystemPrompt composes the system prompt for one workspace-agent turn:
// a role line, the fixed safety rule (highest priority), the thread-mode
// paragraph when applicable, then the optional operator-supplied prompt.
//
// It deliberately carries NO current time. The turn's instant rides in the first
// user message (agent.PlannerMessage, applied in Durable.StartTurn) so that this
// prompt and the tool definitions stay byte-identical from one turn to the next
// and remain a prompt-cache hit. A value that changes every turn would put the
// whole system block back on the bill each time.
func buildSystemPrompt(ws *model.WorkspaceEntry) (string, error) {
	systemPromptOnce.Do(func() {
		systemPromptTmpl, systemPromptErr = template.New("system").Parse(systemPromptTmplSrc)
	})
	if systemPromptErr != nil {
		return "", goerr.Wrap(systemPromptErr, "parse workspace-agent system prompt template")
	}

	input := systemPromptInput{}
	if ws != nil {
		input.WorkspaceName = ws.Workspace.Name
		if input.WorkspaceName == "" {
			input.WorkspaceName = ws.Workspace.ID
		}
		input.ThreadMode = ws.IsThreadMode()
		if input.ThreadMode && ws.CaseStatusSet != nil {
			input.BoardStatuses = ws.CaseStatusSet.IDs()
		}
		input.CustomPrompt = strings.TrimSpace(ws.WorkspaceAgentPrompt)
	}

	var b strings.Builder
	if err := systemPromptTmpl.Execute(&b, input); err != nil {
		return "", goerr.Wrap(err, "execute workspace-agent system prompt template")
	}
	return strings.TrimSpace(b.String()), nil
}
