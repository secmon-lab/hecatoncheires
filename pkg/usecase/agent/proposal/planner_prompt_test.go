package proposal_test

import (
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/proposal"
)

func TestRenderPlannerPrompt_NoWorkspaces(t *testing.T) {
	got, err := proposal.RenderPlannerPromptForTest(nil, "English")
	gt.NoError(t, err).Required()

	gt.S(t, got).Contains("First, identify the workspace")
	gt.S(t, got).Contains("Tools you can use")
	gt.S(t, got).Contains("Before asking the user, gather minimum context")
	gt.S(t, got).Contains("get_workspace")
	gt.S(t, got).Contains("list_workspaces")
	gt.S(t, got).Contains("No workspaces are registered")
	gt.S(t, got).Contains("**English**")
	// Length / shape limits are part of the prompt contract — drift here
	// re-opens the Slack `invalid_arguments` regression these caps prevent.
	gt.S(t, got).Contains("Length and shape limits")
	gt.S(t, got).Contains("about 80 characters or fewer")
	gt.S(t, got).Contains("never exceed 2,000 characters")
	gt.S(t, got).Contains("real Slack user ID")
}

func TestRenderPlannerPrompt_WorkspacesIdentityOnly(t *testing.T) {
	// Use unique sentinel strings for the field / option identifiers so the
	// test can detect leakage without picking up generic prompt vocabulary.
	r := model.NewWorkspaceRegistry()
	r.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{
			ID:          "ws-zzfixture",
			Name:        "ZZFixtureWorkspace",
			Description: "Fixture workspace for prompt-rendering tests",
		},
		// FieldSchema is intentionally populated to confirm it does NOT
		// leak into the system prompt — the planner must pull this via
		// the `get_workspace` tool, not from the prompt body.
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{
					ID:       "zz_unique_field_id",
					Name:     "ZZUniqueFieldName",
					Type:     types.FieldTypeSelect,
					Required: true,
					Options: []config.FieldOption{
						{ID: "zz_opt_alpha", Name: "ZZAlpha", Description: "alpha", Metadata: map[string]any{"score": 1}},
						{ID: "zz_opt_beta", Name: "ZZBeta", Description: "beta"},
					},
				},
			},
		},
	})

	got, err := proposal.RenderPlannerPromptForTest(r, "Japanese")
	gt.NoError(t, err).Required()

	gt.S(t, got).Contains("`ws-zzfixture` — ZZFixtureWorkspace")
	gt.S(t, got).Contains("Fixture workspace for prompt-rendering tests")
	gt.S(t, got).Contains("**Japanese**")

	// Schema details must NOT be in the prompt — those are the planner's
	// job to fetch via get_workspace. If any of these strings leak, the
	// system prompt is reverting to the legacy inlined-schema design.
	gt.Bool(t, strings.Contains(got, "Required fields:")).False()
	gt.Bool(t, strings.Contains(got, "Optional fields:")).False()
	gt.Bool(t, strings.Contains(got, "zz_unique_field_id")).False()
	gt.Bool(t, strings.Contains(got, "ZZUniqueFieldName")).False()
	gt.Bool(t, strings.Contains(got, "zz_opt_alpha")).False()
	gt.Bool(t, strings.Contains(got, "ZZAlpha")).False()
}

func TestRenderPlannerPrompt_LanguageSuppressed(t *testing.T) {
	got, err := proposal.RenderPlannerPromptForTest(nil, "")
	gt.NoError(t, err).Required()
	gt.Bool(t, strings.Contains(got, "## Language")).False()
}

// TestRenderPlannerPrompt_InvestigateBeforeMaterializeRule pins the
// "Investigate-before-materialize when sources advertise relevant
// context" rule. The rule exists because real-LLM runs were
// emitting `materialize` with guessed field values even when the
// workspace's get_workspace response advertised matching Slack /
// Notion sources (and sometimes when the user explicitly asked for
// the sources to be consulted). Dropping this rule re-opens that
// failure mode and TestRunTurn_RealLLM_InfersFieldsFromSources
// starts flaking again.
func TestRenderPlannerPrompt_InvestigateBeforeMaterializeRule(t *testing.T) {
	got, err := proposal.RenderPlannerPromptForTest(nil, "English")
	gt.NoError(t, err).Required()
	gt.S(t, got).Contains("Investigate-before-materialize when sources advertise relevant context")
	gt.S(t, got).Contains("you **MUST** emit at least one `investigate` round")
	gt.S(t, got).Contains("`slack_ro`")
	gt.S(t, got).Contains("`notion`")
	gt.S(t, got).Contains("`github`")
}

// TestRenderPlannerPrompt_ListWorkspacesDoesNotCount pins the
// reinforced wording that prevents the planner from skipping
// get_workspace by calling list_workspaces alone and going straight
// to a terminal action. Real-LLM runs were taking exactly that
// shortcut and emitting `question` after only `list_workspaces`,
// which fails TestRunTurn_RealLLM_VagueMentionAsksQuestion's
// requirePlannerTools=["get_workspace"] check.
func TestRenderPlannerPrompt_ListWorkspacesDoesNotCount(t *testing.T) {
	got, err := proposal.RenderPlannerPromptForTest(nil, "English")
	gt.NoError(t, err).Required()
	gt.S(t, got).Contains("`list_workspaces` does **NOT** count")
	gt.S(t, got).Contains("`list_workspaces` does NOT count toward this check")
}

// TestRenderPlannerPrompt_ListWorkspacesDiscouraged pins the "do not
// call list_workspaces in normal operation" guidance. Real-LLM runs
// were calling list_workspaces alone and then jumping to a terminal
// action; discouraging the call up front (instead of only after-the-
// fact rejecting via the Hard rule) shrinks the surface area where
// that mistake can occur.
func TestRenderPlannerPrompt_ListWorkspacesDiscouraged(t *testing.T) {
	got, err := proposal.RenderPlannerPromptForTest(nil, "English")
	gt.NoError(t, err).Required()
	gt.S(t, got).Contains("**Do not call this in normal operation.**")
	gt.S(t, got).Contains("**This is the tool you should be calling on round 0.**")
	gt.S(t, got).Contains("Concrete counter-example of what NOT to do")
}
