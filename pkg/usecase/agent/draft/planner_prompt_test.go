package draft_test

import (
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/draft"
)

func TestRenderPlannerPrompt_NoWorkspaces(t *testing.T) {
	got, err := draft.RenderPlannerPromptForTest(nil, "English")
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

	got, err := draft.RenderPlannerPromptForTest(r, "Japanese")
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
	got, err := draft.RenderPlannerPromptForTest(nil, "")
	gt.NoError(t, err).Required()
	gt.Bool(t, strings.Contains(got, "## Language")).False()
}
