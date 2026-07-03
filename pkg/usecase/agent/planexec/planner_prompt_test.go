package planexec_test

import (
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
)

func TestPlannerPrompt_HappyPath(t *testing.T) {
	got, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		HostPrompt:      "You are the proposal planner for case drafts.",
		Language:        "Japanese",
		KnownToolIDs:    []string{"slack_ro", "github"},
		AllowQuestion:   true,
		StructuredFinal: true,
	})
	gt.NoError(t, err).Required()
	// HostPrompt is prepended verbatim.
	gt.String(t, got).Contains("You are the proposal planner for case drafts.")
	// Loop description section.
	gt.String(t, got).Contains("Planner protocol")
	gt.String(t, got).Contains("Round 1")
	gt.String(t, got).Contains("Round 2 and later")
	// AllowQuestion = true → question section appears.
	gt.String(t, got).Contains("question")
	gt.String(t, got).Contains("ask the user")
	// StructuredFinal = true → structured-final mention.
	gt.String(t, got).Contains("structured JSON")
	// Language directive references the supplied language.
	gt.String(t, got).Contains("Japanese")
	// Known tool IDs are listed.
	gt.String(t, got).Contains("`slack_ro`")
	gt.String(t, got).Contains("`github`")
}

func TestPlannerPrompt_QuestionDisabled(t *testing.T) {
	got, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		HostPrompt:      "Run scheduled job analysis.",
		KnownToolIDs:    []string{"default"},
		AllowQuestion:   false,
		StructuredFinal: false,
	})
	gt.NoError(t, err).Required()
	// AllowQuestion = false → no "ask the user" section.
	gt.String(t, got).Contains("Run scheduled job analysis.")
	// The "question" word will still appear in the Loop shape's
	// "neither" sentence, but the bullet describing question itself
	// should NOT be present. We assert on the action verb "ask the
	// user" which is unique to the conditional block.
	gt.Bool(t, contains(got, "ask the user")).False()
	// StructuredFinal = false → "plain text" mention.
	gt.String(t, got).Contains("plain text")
}

func TestPlannerPrompt_LanguageOmitted(t *testing.T) {
	got, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		HostPrompt:    "x",
		KnownToolIDs:  []string{"a"},
		AllowQuestion: false,
		// Language: ""
	})
	gt.NoError(t, err).Required()
	// No "MUST be written in" directive when language is empty.
	gt.Bool(t, contains(got, "MUST be written in")).False()
}

func TestPlannerPrompt_DirectEnabled(t *testing.T) {
	got, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		HostPrompt:    "You are the thread planner.",
		KnownToolIDs:  []string{"slack_ro"},
		AllowQuestion: true,
		AllowDirect:   true,
	})
	gt.NoError(t, err).Required()
	// The Direct answer section and its strict guard appear.
	gt.String(t, got).Contains("Direct answer")
	gt.String(t, got).Contains("`direct`")
	gt.String(t, got).Contains("When in ANY doubt")
}

func TestPlannerPrompt_DirectDisabled(t *testing.T) {
	got, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		HostPrompt:    "You are the thread planner.",
		KnownToolIDs:  []string{"slack_ro"},
		AllowQuestion: true,
		// AllowDirect: false
	})
	gt.NoError(t, err).Required()
	// No Direct answer section when the host did not opt in.
	gt.Bool(t, contains(got, "Direct answer")).False()
}

func TestPlannerPrompt_SubAgentWritesEnabled(t *testing.T) {
	got, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		HostPrompt:          "Run scheduled job analysis.",
		KnownToolIDs:        []string{"default"},
		AllowSubAgentWrites: true,
	})
	gt.NoError(t, err).Required()
	// The "Actions and writes" guidance and its key rules appear.
	gt.String(t, got).Contains("Actions and writes")
	gt.String(t, got).Contains("investigate first")
	gt.String(t, got).Contains("self-evident")
	gt.String(t, got).Contains("same phase")
	gt.String(t, got).Contains("final response cannot perform actions")
}

func TestPlannerPrompt_SubAgentWritesDisabled(t *testing.T) {
	got, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		HostPrompt:   "You are the thread planner.",
		KnownToolIDs: []string{"slack_ro"},
		// AllowSubAgentWrites: false
	})
	gt.NoError(t, err).Required()
	// No write guidance when the host did not opt in.
	gt.Bool(t, contains(got, "Actions and writes")).False()
}

func TestPlannerPrompt_RejectsEmptyHostPrompt(t *testing.T) {
	_, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		KnownToolIDs: []string{"a"},
	})
	gt.Error(t, err)
}

func TestPlannerPrompt_RejectsEmptyKnownToolIDs(t *testing.T) {
	_, err := planexec.RenderPlannerPromptForTest(planexec.PlannerPromptInputForTest{
		HostPrompt: "x",
	})
	gt.Error(t, err)
}

// contains is a thin substring check used by the negative-presence
// assertions above. We deliberately use a helper here rather than
// gt.String(...).NotContains because the test asserts the *absence*
// of a sub-string with a custom failure message that names what we
// expected to be missing.
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
