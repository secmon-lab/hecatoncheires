package agent_test

import (
	"strings"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
)

func TestTaskContext_RendersEveryIdentifier(t *testing.T) {
	got, err := agent.TaskContext{
		WorkspaceID:    "ws-security",
		CaseID:         42,
		SlackChannelID: "C0123456789",
		SlackThreadTS:  "1700000000.000100",
	}.Render()
	gt.NoError(t, err).Required()

	gt.String(t, got).Contains("- workspace_id: ws-security")
	gt.String(t, got).Contains("- case_id: 42")
	gt.String(t, got).Contains("- slack_channel_id: C0123456789")
	gt.String(t, got).Contains("- slack_thread_ts: 1700000000.000100")
	// The thread is the one identifier a sub-agent cannot derive from anything
	// else, so the block must say what to do with it.
	gt.String(t, got).Contains("slack__get_messages")
}

// A run with no case must not emit a case_id line at all: "case_id: 0" is a
// value the model would pass on to a tool.
func TestTaskContext_OmitsUnsetIdentifiers(t *testing.T) {
	got, err := agent.TaskContext{
		WorkspaceID:    "ws-security",
		SlackChannelID: "C0123456789",
	}.Render()
	gt.NoError(t, err).Required()

	gt.String(t, got).Contains("- workspace_id: ws-security")
	gt.String(t, got).Contains("- slack_channel_id: C0123456789")
	gt.Bool(t, containsAny(got, "case_id", "slack_thread_ts")).False()
	// Nothing to read means no reading instruction.
	gt.Bool(t, containsAny(got, "slack__get_messages")).False()
}

// A workspace with no Slack leaves only the ids that exist; the block must stay
// well-formed rather than rendering dangling labels.
func TestTaskContext_WorkspaceOnly(t *testing.T) {
	got, err := agent.TaskContext{WorkspaceID: "ws-security"}.Render()
	gt.NoError(t, err).Required()
	gt.String(t, got).Equal("The run you are part of is pinned to:\n- workspace_id: ws-security")
}

// A zero context renders nothing, so the host can pass the result through
// unconditionally and the sub-agent prompt simply omits the section.
func TestTaskContext_ZeroRendersNothing(t *testing.T) {
	gt.Bool(t, agent.TaskContext{}.IsZero()).True()

	got, err := agent.TaskContext{}.Render()
	gt.NoError(t, err).Required()
	gt.String(t, got).Equal("")
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
