package planexec_test

import (
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
)

// The host's persona prompt is the spine of the direct prompt: without it the
// reply has no idea who it is or what thread it is in, so an empty one is refused
// rather than rendered into a prompt that reads as if a section went missing.
func TestDirectPromptRequiresTheHostPrompt(t *testing.T) {
	_, err := planexec.RenderDirectPromptForTest(planexec.DirectPromptInputForTest{
		Language: "Japanese",
	})
	gt.Error(t, err).Required()
}

// Both optional sections are driven by the host, and either may be absent: a host
// that sets no language leaves the model to follow the conversation, and one whose
// tools need no identifiers has no context to pin. Neither absence may leave a
// dangling heading behind.
func TestDirectPromptOptionalSectionsAppearOnlyWhenSupplied(t *testing.T) {
	bare, err := planexec.RenderDirectPromptForTest(planexec.DirectPromptInputForTest{
		HostPrompt: "you answer in this case thread",
	})
	gt.NoError(t, err).Required()
	gt.String(t, bare).Contains("you answer in this case thread")
	gt.String(t, bare).NotContains("Run context")
	gt.String(t, bare).NotContains("MUST be written in")

	full, err := planexec.RenderDirectPromptForTest(planexec.DirectPromptInputForTest{
		HostPrompt: "you answer in this case thread",
		Language:   "Japanese",
		Context:    "channel_id: C123",
	})
	gt.NoError(t, err).Required()
	gt.String(t, full).Contains("Run context")
	gt.String(t, full).Contains("channel_id: C123")
	gt.String(t, full).Contains("MUST be written in **Japanese**")
}

// The host prompt is written for the planner, so it names decision shapes
// (`respond`, `materialize`) and demands JSON. The direct child can emit neither,
// and everything it writes is published, so the prompt must override that half of
// the host's instructions explicitly rather than leave the model to reconcile
// them — a child that emitted a JSON decision would have it posted as prose.
func TestDirectPromptOverridesTheHostsDecisionInstructions(t *testing.T) {
	out, err := planexec.RenderDirectPromptForTest(planexec.DirectPromptInputForTest{
		HostPrompt: "Your terminal decision is ONE of: respond, or materialize.",
	})
	gt.NoError(t, err).Required()
	gt.String(t, out).Contains("Disregard any instruction above about emitting a decision")
	gt.String(t, out).Contains("Do NOT emit JSON")
}
