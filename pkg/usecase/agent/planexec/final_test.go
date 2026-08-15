package planexec_test

import (
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
)

func TestFinalUserPrompt_Plain(t *testing.T) {
	got, err := planexec.RenderFinalUserPromptForTest(planexec.FinalPromptInputForTest{
		Observations:    "## phase 1\n(found stuff)\n",
		StructuredFinal: false,
		Language:        "",
	})
	gt.NoError(t, err).Required()
	gt.String(t, got).Contains("(found stuff)")
	gt.String(t, got).Contains("Emit plain natural-language text")
	gt.Bool(t, containsAny(got, "Emit a single JSON object")).False()
	gt.Bool(t, containsAny(got, "MUST be written in")).False()
}

func TestFinalUserPrompt_StructuredAndLanguage(t *testing.T) {
	got, err := planexec.RenderFinalUserPromptForTest(planexec.FinalPromptInputForTest{
		Observations:    "(obs)",
		StructuredFinal: true,
		Language:        "Japanese",
	})
	gt.NoError(t, err).Required()
	gt.String(t, got).Contains("Emit a single JSON object")
	gt.String(t, got).Contains("Japanese")
	gt.Bool(t, containsAny(got, "Emit plain natural-language text")).False()
}

func TestFinalUserPrompt_EmptyObservationsLabel(t *testing.T) {
	got := planexec.RenderObservationsForFinalForTest(nil)
	gt.String(t, got).Contains("no investigations were run")
}

// The terminal LLM call itself is exercised on the live path by
// TestPlanCollectReplanFinal (plain text) and
// TestStructuredFinalIsValidatedAndRegenerated (JSON, with regeneration) in
// strategy_test.go, which drive the strategy rather than a helper.

// containsAny is a thin substring helper used by negative-presence
// assertions in this file.
func containsAny(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
