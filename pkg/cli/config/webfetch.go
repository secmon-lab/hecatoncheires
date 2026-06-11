package config

import (
	"log/slog"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/webfetch"
	"github.com/urfave/cli/v3"
)

// webFetchUserAgent is the User-Agent sent on every webfetch request. It is a
// caller-level default (the CLI owns it) rather than a hidden default inside
// the webfetch package.
const webFetchUserAgent = "hecatoncheires-webfetch/1.0 (+https://github.com/secmon-lab/hecatoncheires)"

// WebFetch holds configuration for the agent webfetch tool. The shared LLM
// client (used for injection screening + Markdown formatting) is NOT held here:
// it lives in the usecase layer and is injected when the client is built.
type WebFetch struct {
	enabled    bool
	timeoutSec int
	maxBytes   int
}

// Flags returns CLI flags for the webfetch tool.
func (w *WebFetch) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:        "webfetch-enabled",
			Usage:       "Enable the agent webfetch tool (requires an LLM client for injection screening)",
			Value:       true,
			Sources:     cli.EnvVars("HECATONCHEIRES_WEBFETCH_ENABLED"),
			Destination: &w.enabled,
		},
		&cli.IntFlag{
			Name:        "webfetch-timeout",
			Usage:       "webfetch HTTP request timeout in seconds",
			Value:       10,
			Sources:     cli.EnvVars("HECATONCHEIRES_WEBFETCH_TIMEOUT"),
			Destination: &w.timeoutSec,
		},
		&cli.IntFlag{
			Name:        "webfetch-max-size",
			Usage:       "webfetch maximum response body size in bytes",
			Value:       1048576,
			Sources:     cli.EnvVars("HECATONCHEIRES_WEBFETCH_MAX_SIZE"),
			Destination: &w.maxBytes,
		},
	}
}

// LogAttrs returns log attributes for the webfetch configuration.
func (w *WebFetch) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.Bool("enabled", w.enabled),
		slog.Int("timeout_sec", w.timeoutSec),
		slog.Int("max_bytes", w.maxBytes),
	}
}

// IsEnabled reports whether the webfetch tool should be wired.
func (w *WebFetch) IsEnabled() bool {
	return w.enabled
}

// Settings returns the HTTP-side client config. ClientConfig.LLM is left nil:
// the usecase layer injects the shared LLM client before building the client.
func (w *WebFetch) Settings() webfetch.ClientConfig {
	return webfetch.ClientConfig{
		Timeout:   time.Duration(w.timeoutSec) * time.Second,
		MaxBytes:  int64(w.maxBytes),
		UserAgent: webFetchUserAgent,
	}
}
