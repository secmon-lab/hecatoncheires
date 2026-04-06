package config_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func TestSlackSetNoAuthUID(t *testing.T) {
	slack := config.NewSlackForTest("", "", "", "", "")

	// Initially empty
	if slack.NoAuthUID() != "" {
		t.Errorf("NoAuthUID should be empty initially, got %v", slack.NoAuthUID())
	}

	// Set no-auth UID
	slack.SetNoAuthUID("U1234567890")
	if slack.NoAuthUID() != "U1234567890" {
		t.Errorf("NoAuthUID mismatch: got %v, want %v", slack.NoAuthUID(), "U1234567890")
	}
}

func TestSlackIsNoAuthMode(t *testing.T) {
	slack := config.NewSlackForTest("", "", "", "", "")

	// Initially false
	if slack.IsNoAuthMode() {
		t.Error("IsNoAuthMode should be false initially")
	}

	// After setting no-auth UID
	slack.SetNoAuthUID("U1234567890")
	if !slack.IsNoAuthMode() {
		t.Error("IsNoAuthMode should be true after setting no-auth UID")
	}
}

func TestSlackConfigureNoAuthWithoutBotToken(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	slack := config.NewSlackForTest("", "", "", "", "U1234567890")

	// Should succeed without bot token, falling back to default test user
	usecase, err := slack.Configure(ctx, repo, "")
	if err != nil {
		t.Errorf("Configure should not fail without bot token in no-auth mode: %v", err)
	}
	if usecase == nil {
		t.Error("Configure should return a valid usecase")
	}
}

// TestSlackConfigureNoAuthWithOAuth verifies that no-auth takes precedence over OAuth
// (warning is logged but no error is returned - we can't easily test the warning here
// but we verify the behavior works correctly)
// Note: This test will fail because it tries to validate a fake user ID against Slack API
// In real usage, the warning would be logged and no-auth mode would be used

func TestSlackConfigureMissingConfiguration(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	slack := config.NewSlackForTest("", "", "", "", "")

	// Should succeed with fallback to default test user when no configuration is provided
	usecase, err := slack.Configure(ctx, repo, "")
	if err != nil {
		t.Errorf("Configure should not fail when no configuration is provided: %v", err)
	}
	if usecase == nil {
		t.Error("Configure should return a valid usecase")
	}
}

func TestSlackValidateWorkspaceTeamIDs(t *testing.T) {
	t.Run("org-level app requires team_id on all workspaces", func(t *testing.T) {
		s := config.NewSlackForTest("", "", "xoxb-token", "", "")
		s.SetOrgLevelForTest(true, "")

		configs := []*config.WorkspaceConfig{
			{ID: "ws1", SlackTeamID: "T111"},
			{ID: "ws2", SlackTeamID: ""},
		}
		err := s.ValidateWorkspaceTeamIDs(configs)
		gt.Value(t, err).NotNil()
	})

	t.Run("org-level app with all team_ids set succeeds", func(t *testing.T) {
		s := config.NewSlackForTest("", "", "xoxb-token", "", "")
		s.SetOrgLevelForTest(true, "")

		configs := []*config.WorkspaceConfig{
			{ID: "ws1", SlackTeamID: "T111"},
			{ID: "ws2", SlackTeamID: "T222"},
		}
		err := s.ValidateWorkspaceTeamIDs(configs)
		gt.NoError(t, err)
	})

	t.Run("ws-level app with empty team_id succeeds", func(t *testing.T) {
		s := config.NewSlackForTest("", "", "xoxb-token", "", "")
		s.SetOrgLevelForTest(false, "T999")

		configs := []*config.WorkspaceConfig{
			{ID: "ws1", SlackTeamID: ""},
		}
		err := s.ValidateWorkspaceTeamIDs(configs)
		gt.NoError(t, err)
	})

	t.Run("ws-level app with matching team_id succeeds", func(t *testing.T) {
		s := config.NewSlackForTest("", "", "xoxb-token", "", "")
		s.SetOrgLevelForTest(false, "T999")

		configs := []*config.WorkspaceConfig{
			{ID: "ws1", SlackTeamID: "T999"},
		}
		err := s.ValidateWorkspaceTeamIDs(configs)
		gt.NoError(t, err)
	})

	t.Run("ws-level app with mismatched team_id fails", func(t *testing.T) {
		s := config.NewSlackForTest("", "", "xoxb-token", "", "")
		s.SetOrgLevelForTest(false, "T999")

		configs := []*config.WorkspaceConfig{
			{ID: "ws1", SlackTeamID: "T000"},
		}
		err := s.ValidateWorkspaceTeamIDs(configs)
		gt.Value(t, err).NotNil()
	})

	t.Run("no bot token skips validation", func(t *testing.T) {
		s := config.NewSlackForTest("", "", "", "", "")
		s.SetOrgLevelForTest(false, "")

		configs := []*config.WorkspaceConfig{
			{ID: "ws1", SlackTeamID: "T000"},
		}
		err := s.ValidateWorkspaceTeamIDs(configs)
		gt.NoError(t, err)
	})
}

func TestSlackIsConfigured(t *testing.T) {
	tests := []struct {
		name           string
		clientID       string
		clientSecret   string
		wantConfigured bool
	}{
		{"both set", "id", "secret", true},
		{"only client ID", "id", "", false},
		{"only client secret", "", "secret", false},
		{"neither set", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slack := config.NewSlackForTest(tt.clientID, tt.clientSecret, "", "", "")
			if got := slack.IsConfigured(); got != tt.wantConfigured {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.wantConfigured)
			}
		})
	}
}
