package agent

import (
	"bytes"
	_ "embed"
	"strings"
	"text/template"

	"github.com/m-mizutani/goerr/v2"
)

//go:embed prompts/task_context.md
var taskContextTmpl string

var taskContextTemplate = template.Must(template.New("agent_task_context").Parse(taskContextTmpl))

// TaskContext is the identifier block a plan-execute host hands to every
// sub-agent it spawns (planexec.Input.TaskContext).
//
// It exists because a sub-agent's system prompt is built from the planner's task
// text alone, while its tools are pinned to the run's subject by the kernel tool
// factory. Without the identifiers a task told to read the case conversation has
// to invent a channel id and a message timestamp, and slack__get_messages then
// rejects the call ("targets[0] requires both channel_id and ts") or looks up a
// message that does not exist. The host knows the values its tools were pinned
// to, so the host supplies them.
//
// Every field is optional: a run that is not pinned to a case leaves CaseID zero,
// and a workspace with no Slack leaves the Slack fields empty. Render then omits
// those lines rather than emitting an empty value the model could pass on.
type TaskContext struct {
	WorkspaceID string
	// CaseID is the case the run works on. Zero when there is none yet (a
	// case-draft turn) or the run spans several (the workspace-channel agent).
	CaseID int64
	// SlackChannelID is the channel the run's Slack tools are pinned to.
	SlackChannelID string
	// SlackThreadTS is the thread the run's conversation lives in: the case
	// thread for a thread-mode case, the triggering thread for a mention.
	SlackThreadTS string
}

// IsZero reports whether there is nothing worth telling a sub-agent.
func (c TaskContext) IsZero() bool {
	return c.WorkspaceID == "" && c.CaseID == 0 && c.SlackChannelID == "" && c.SlackThreadTS == ""
}

// Render returns the block, or an empty string when there is nothing to say —
// so a host can pass the result through unconditionally and the sub-agent
// prompt simply omits the section.
func (c TaskContext) Render() (string, error) {
	if c.IsZero() {
		return "", nil
	}
	var buf bytes.Buffer
	if err := taskContextTemplate.Execute(&buf, c); err != nil {
		return "", goerr.Wrap(err, "render the sub-agent task context",
			goerr.V("workspace_id", c.WorkspaceID), goerr.V("case_id", c.CaseID))
	}
	return strings.TrimSpace(buf.String()), nil
}
