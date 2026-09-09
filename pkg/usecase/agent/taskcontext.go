package agent

import (
	"bytes"
	_ "embed"
	"strings"
	"text/template"
	"time"

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
	// Now is the turn's start time, rendered as the sub-agent's absolute current
	// time. Without it a task whose text says "last week" or "by today" has no
	// instant to resolve against and the model falls back on whatever date its
	// training suggests.
	//
	// The turn's instant rather than the call's, because a sub-agent's system
	// prompt is built once when it is spawned and never rebuilt. That also keeps
	// the value out of the per-call cache prefix: it is constant for the whole
	// child, so its prompt stays a cache hit across the child's calls.
	Now time.Time
}

// taskContextView is what prompts/task_context.md is executed against. The
// rendered forms are computed here rather than in the template so the template
// holds no formatting logic.
type taskContextView struct {
	WorkspaceID    string
	CaseID         int64
	SlackChannelID string
	SlackThreadTS  string
	// CurrentTime is Now as an RFC3339 UTC string, empty when Now is zero.
	CurrentTime string
}

// IsZero reports whether there is nothing worth telling a sub-agent.
func (c TaskContext) IsZero() bool {
	return c.WorkspaceID == "" && c.CaseID == 0 && c.SlackChannelID == "" &&
		c.SlackThreadTS == "" && c.Now.IsZero()
}

// Render returns the block, or an empty string when there is nothing to say —
// so a host can pass the result through unconditionally and the sub-agent
// prompt simply omits the section.
func (c TaskContext) Render() (string, error) {
	if c.IsZero() {
		return "", nil
	}
	view := taskContextView{
		WorkspaceID:    c.WorkspaceID,
		CaseID:         c.CaseID,
		SlackChannelID: c.SlackChannelID,
		SlackThreadTS:  c.SlackThreadTS,
	}
	if !c.Now.IsZero() {
		view.CurrentTime = c.Now.UTC().Format(time.RFC3339)
	}
	var buf bytes.Buffer
	if err := taskContextTemplate.Execute(&buf, view); err != nil {
		return "", goerr.Wrap(err, "render the sub-agent task context",
			goerr.V("workspace_id", c.WorkspaceID), goerr.V("case_id", c.CaseID))
	}
	return strings.TrimSpace(buf.String()), nil
}
