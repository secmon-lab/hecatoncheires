package agent_test

import (
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
)

const plannerBody = "@bot draft a case for the failed deploy"

// The section is what gives a run an absolute instant to resolve "today" and
// "by tomorrow" against. Without it the model answers from whatever date its
// training suggests.
func TestPlannerMessage_StatesTheInstantAheadOfTheBody(t *testing.T) {
	got, err := agent.PlannerMessage{
		Now:  time.Date(2026, 9, 7, 3, 35, 19, 0, time.UTC),
		Body: plannerBody,
	}.Render()
	gt.NoError(t, err).Required()

	gt.String(t, got).Contains("# Current time")
	gt.String(t, got).Contains("2026-09-07T03:35:19Z")
	gt.String(t, got).Contains("(UTC)")
	gt.String(t, got).Contains("absolute")
	gt.String(t, got).Contains(plannerBody)
	gt.Bool(t, strings.Index(got, "# Current time") < strings.Index(got, plannerBody)).True()
	// A turn that inherits the previous turn's conversation carries that turn's
	// section too, so the block must say which of them is now.
	gt.String(t, got).Contains("the latest one is now")
}

// A non-UTC instant is still rendered in UTC, so the section's stated zone and
// its value can never disagree.
func TestPlannerMessage_RendersInUTC(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	got, err := agent.PlannerMessage{
		Now:  time.Date(2026, 9, 7, 12, 35, 19, 0, jst),
		Body: plannerBody,
	}.Render()
	gt.NoError(t, err).Required()

	gt.String(t, got).Contains("2026-09-07T03:35:19Z")
}

// A zero time leaves the body alone rather than rendering Go's zero date as the
// current time, so a caller can build the message unconditionally.
func TestPlannerMessage_ZeroTimeLeavesTheBodyAlone(t *testing.T) {
	got, err := agent.PlannerMessage{Body: plannerBody}.Render()
	gt.NoError(t, err).Required()

	gt.String(t, got).Equal(plannerBody)
}

// Exactly one blank line between the section and the body: the template's own
// trailing newline must not add a second.
func TestPlannerMessage_SeparatesTheSectionWithOneBlankLine(t *testing.T) {
	got, err := agent.PlannerMessage{
		Now:  time.Date(2026, 9, 7, 3, 35, 19, 0, time.UTC),
		Body: plannerBody,
	}.Render()
	gt.NoError(t, err).Required()

	gt.String(t, got).Contains("the latest one is now.\n\n" + plannerBody)
}
