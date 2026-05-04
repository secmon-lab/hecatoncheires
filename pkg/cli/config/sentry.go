package config

import (
	"context"
	"log/slog"

	"github.com/urfave/cli/v3"

	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	obssentry "github.com/secmon-lab/hecatoncheires/pkg/utils/observability/sentry"
)

// Sentry binds the HECATONCHEIRES_SENTRY_* CLI flags / env vars and drives
// the obssentry package's lifecycle. An empty DSN keeps Sentry disabled and
// the rest of the values are ignored.
type Sentry struct {
	dsn         string
	environment string
	release     string
}

// Flags returns CLI flags for Sentry configuration.
func (x *Sentry) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "sentry-dsn",
			Category:    "Observability",
			Usage:       "Sentry DSN. Setting a non-empty value enables Sentry error reporting.",
			Sources:     cli.EnvVars("HECATONCHEIRES_SENTRY_DSN"),
			Destination: &x.dsn,
		},
		&cli.StringFlag{
			Name:        "sentry-env",
			Category:    "Observability",
			Usage:       "Sentry environment tag (e.g., production, staging).",
			Sources:     cli.EnvVars("HECATONCHEIRES_SENTRY_ENV"),
			Destination: &x.environment,
		},
		&cli.StringFlag{
			Name:        "sentry-release",
			Category:    "Observability",
			Usage:       "Sentry release identifier (e.g., commit SHA).",
			Sources:     cli.EnvVars("HECATONCHEIRES_SENTRY_RELEASE"),
			Destination: &x.release,
		},
	}
}

// LogValue masks the DSN so it never lands in operational logs while still
// surfacing the rest of the Sentry configuration for diagnostics.
func (x Sentry) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Bool("dsn_set", x.dsn != ""),
		slog.String("environment", x.environment),
		slog.String("release", x.release),
	)
}

// Configure initializes the Sentry SDK from the bound flags. When the DSN
// is empty Sentry stays disabled. SDK-level init errors are reported via
// errutil.Handle but never fail startup — losing Sentry must not block the
// service from coming up.
func (x *Sentry) Configure(ctx context.Context) {
	if x.dsn == "" {
		logging.From(ctx).Info("Sentry disabled (no DSN configured)")
		return
	}

	if err := obssentry.Init(obssentry.Config{
		DSN:         x.dsn,
		Environment: x.environment,
		Release:     x.release,
	}); err != nil {
		errutil.Handle(ctx, err, "failed to initialize Sentry; continuing without it")
		return
	}

	logging.From(ctx).Info("Sentry enabled",
		slog.String("environment", x.environment),
		slog.String("release", x.release),
	)
}
