package agent_test

import (
	"strings"
	"testing"
	"time"

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

// A sub-agent is built from the planner's task text alone, so without an
// absolute instant a task saying "last week" or "by today" has nothing to
// resolve against and the model falls back on its training's idea of the date.
func TestTaskContext_RendersTheCurrentTime(t *testing.T) {
	got, err := agent.TaskContext{
		WorkspaceID: "ws-security",
		Now:         time.Date(2026, 9, 7, 3, 35, 19, 0, time.UTC),
	}.Render()
	gt.NoError(t, err).Required()

	gt.String(t, got).Contains("- current_time: 2026-09-07T03:35:19Z (UTC)")
	gt.String(t, got).Contains("absolute form")
}

// A non-UTC instant is still rendered in UTC, so the block's stated zone and its
// value can never disagree.
func TestTaskContext_RendersTheCurrentTimeInUTC(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	got, err := agent.TaskContext{
		WorkspaceID: "ws-security",
		Now:         time.Date(2026, 9, 7, 12, 35, 19, 0, jst),
	}.Render()
	gt.NoError(t, err).Required()

	gt.String(t, got).Contains("- current_time: 2026-09-07T03:35:19Z (UTC)")
}

// A zero time omits the line rather than rendering Go's zero date, which the
// model would take at face value.
func TestTaskContext_OmitsAnUnsetCurrentTime(t *testing.T) {
	got, err := agent.TaskContext{WorkspaceID: "ws-security"}.Render()
	gt.NoError(t, err).Required()

	gt.Bool(t, containsAny(got, "current_time", "0001-01-01")).False()
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
// unconditionally and the sub-agent prompt simply omits the section. A context
// carrying only the time is NOT zero: the instant is worth telling on its own.
func TestTaskContext_ZeroRendersNothing(t *testing.T) {
	gt.Bool(t, agent.TaskContext{}.IsZero()).True()
	gt.Bool(t, agent.TaskContext{Now: time.Date(2026, 9, 7, 3, 35, 19, 0, time.UTC)}.IsZero()).False()

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
