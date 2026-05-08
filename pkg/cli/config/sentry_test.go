package config_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
)

func TestSentry_Configure_DSNEmptyIsNoop(t *testing.T) {
	cfg := config.NewSentryForTest("", "production", "rel-1")
	cfg.Configure(context.Background())
	// Configure with empty DSN must return without error or panic.
}

func TestSentry_Configure_FailedInitDoesNotPanic(t *testing.T) {
	// Sentry SDK rejects malformed DSNs; Configure must absorb the error
	// rather than aborting startup.
	cfg := config.NewSentryForTest("not-a-valid-dsn", "production", "rel-1")
	cfg.Configure(context.Background())
}

func TestSentry_Flags_Bind(t *testing.T) {
	var s config.Sentry
	flags := s.Flags()

	gt.Array(t, flags).Length(3).Required()

	wantNames := []string{
		"sentry-dsn",
		"sentry-env",
		"sentry-release",
	}
	for i, want := range wantNames {
		gt.Array(t, flags[i].Names()).Length(1).Required()
		gt.String(t, flags[i].Names()[0]).Equal(want)
	}
}

func TestSentry_LogValue_DSNIsMaskedAsBool(t *testing.T) {
	cfg := config.NewSentryForTest("https://abc@sentry.example/1", "production", "rel-1")
	v := cfg.LogValue()
	rendered := v.String()
	// LogValue must not leak the raw DSN.
	gt.Bool(t, contains(rendered, "abc@sentry")).False()
	// But environment / release should appear.
	gt.Bool(t, contains(rendered, "production")).True()
	gt.Bool(t, contains(rendered, "rel-1")).True()
}

func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
