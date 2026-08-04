package config

import (
	"log/slog"

	"github.com/m-mizutani/goerr/v2"
	"github.com/urfave/cli/v3"
)

// defaultJobMaxConcurrency is the out-of-the-box limit on concurrently running
// scheduled Jobs across the whole deployment. It defaults to serial execution:
// an unconfigured deployment must not be able to fan a single tick out into
// dozens of simultaneous LLM runs and blow through the provider's rate limit.
// Operators raise it once they know their quota; 0 disables the limit.
const defaultJobMaxConcurrency = 1

// JobConcurrency holds the CLI flag bounding how many scheduled Agent Job runs
// execute at the same time across every instance of the deployment. Shared by
// `serve` and `tick` because both dispatch Job runs.
type JobConcurrency struct {
	limit int
}

// Flags returns the CLI flags for the Job concurrency limit.
func (j *JobConcurrency) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.IntFlag{
			Name:        "job-max-concurrency",
			Usage:       "Maximum number of scheduled Agent Job runs executing concurrently across the whole deployment (0 disables the limit)",
			Value:       defaultJobMaxConcurrency,
			Sources:     cli.EnvVars("HECATONCHEIRES_JOB_MAX_CONCURRENCY"),
			Destination: &j.limit,
		},
	}
}

// Limit returns the configured limit. 0 means no limit.
func (j *JobConcurrency) Limit() int { return j.limit }

// Validate rejects a negative limit. Called at startup so a typo fails fast
// instead of silently behaving like "no limit".
func (j *JobConcurrency) Validate() error {
	if j.limit < 0 {
		return goerr.New("--job-max-concurrency must not be negative",
			goerr.V("job_max_concurrency", j.limit))
	}
	return nil
}

// LogAttrs returns log attributes describing the configuration.
func (j *JobConcurrency) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.Int("job_max_concurrency", j.limit),
	}
}
