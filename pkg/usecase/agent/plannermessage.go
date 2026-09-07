package agent

import (
	"bytes"
	_ "embed"
	"strings"
	"text/template"
	"time"

	"github.com/m-mizutani/goerr/v2"
)

//go:embed prompts/current_time.md
var currentTimeTmpl string

var currentTimeTemplate = template.Must(template.New("agent_current_time").Parse(currentTimeTmpl))

// PlannerMessage is the FIRST USER MESSAGE a plan-execute host hands its planner:
// the turn's current time, followed by the body the host assembled (the thread
// transcript and the mention, or the request text).
//
// Two things put the time here rather than in the system prompt.
//
// For a host whose turns continue the previous turn's conversation
// (agentkit.WithInheritedHistory: threadcase, proposal) it is a requirement. The
// system prompt is not part of that history — it is handed to each Generate call
// as a session option and rebuilt for every turn — so a resumed turn would carry
// the earlier turns' messages with nothing saying when they were written. A dated
// user message makes the history self-dating, and each turn's own section states
// the instant that is current now.
//
// For a host that inherits nothing (wsagent) it is a preference: the system
// prompt and the tool definitions are the prefix Claude's prompt cache matches
// on, so keeping a per-turn value out of them leaves that prefix byte-identical
// from one turn to the next.
//
// casebound, job and assist predate this and still state the time in their own
// system prompt. They inherit no history, so that is a cache cost rather than a
// correctness problem.
//
// Sub-agents receive neither: they are told through TaskContext.Now.
type PlannerMessage struct {
	// Now is the turn's start time. Zero omits the section, so a caller can build
	// the message unconditionally.
	Now time.Time
	// Body is the message the host assembled. Rendered as-is, after the section.
	Body string
}

// currentTimeView is the single typed input for prompts/current_time.md.
type currentTimeView struct {
	// CurrentTime is the instant as an RFC3339 UTC string.
	CurrentTime string
}

// Render returns the assembled user message.
func (m PlannerMessage) Render() (string, error) {
	if m.Now.IsZero() {
		return m.Body, nil
	}
	var buf bytes.Buffer
	if err := currentTimeTemplate.Execute(&buf, currentTimeView{
		CurrentTime: m.Now.UTC().Format(time.RFC3339),
	}); err != nil {
		return "", goerr.Wrap(err, "render the current time section")
	}
	return strings.TrimSpace(buf.String()) + "\n\n" + m.Body, nil
}
