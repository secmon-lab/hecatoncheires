package config_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	obssentry "github.com/secmon-lab/hecatoncheires/pkg/utils/observability/sentry"
)

func TestSentry_Configure_DisabledWhenDSNEmpty(t *testing.T) {
	cfg := config.NewSentryForTest("", "production", "rel-1")
	cfg.Configure(context.Background())
	gt.Bool(t, obssentry.Enabled()).False()
}

func TestSentry_Configure_FailedInitDoesNotPanic(t *testing.T) {
	// Sentry SDK rejects malformed DSNs; Configure must absorb the error
	// rather than aborting startup. A panic here would surface as a Go
	// test failure automatically (no explicit recover needed).
	cfg := config.NewSentryForTest("not-a-valid-dsn", "production", "rel-1")
	cfg.Configure(context.Background())
	// Either init succeeds (Enabled true) or it fails silently (Enabled
	// false). We don't pin the SDK's parsing rules; we only assert that
	// the bad input did not crash the process.
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
