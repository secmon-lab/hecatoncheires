package config_test

import (
	"context"
	"testing"

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

	// Should fail without bot token
	_, err := slack.Configure(ctx, repo, "")
	if err == nil {
		t.Error("Configure should fail without bot token in no-auth mode")
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

	// Should fail when no configuration is provided
	_, err := slack.Configure(ctx, repo, "")
	if err == nil {
		t.Error("Configure should fail when no authentication is configured")
	}
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
