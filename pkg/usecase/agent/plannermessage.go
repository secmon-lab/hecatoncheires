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
// The time is stated here rather than in the system prompt for the hosts whose
// turns continue the previous turn's conversation
// (agentkit.WithInheritedHistory: threadcase, proposal). The system prompt is not
// part of that history — it is handed to each Generate call as a session option
// and rebuilt for every turn — so a resumed turn would carry the earlier turns'
// messages with nothing saying when they were written. A dated user message makes
// the history self-dating, and each turn's own section states the instant that is
// current now.
//
// A host whose turns never inherit a conversation (casebound, wsagent, job) has
// no earlier message to date and states the time in its system prompt instead,
// where the value is constant for the turn.
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
