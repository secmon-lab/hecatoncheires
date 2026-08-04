package config_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/urfave/cli/v3"

	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
)

// runJobConcurrency parses args through a throwaway command so the flag's
// default and env-var wiring are exercised the same way the real commands do.
func runJobConcurrency(t *testing.T, args ...string) *config.JobConcurrency {
	t.Helper()
	var j config.JobConcurrency
	cmd := &cli.Command{
		Name:  "test",
		Flags: j.Flags(),
		Action: func(_ context.Context, _ *cli.Command) error {
			return nil
		},
	}
	gt.NoError(t, cmd.Run(context.Background(), append([]string{"test"}, args...))).Required()
	return &j
}

func TestJobConcurrency_DefaultsToSerialExecution(t *testing.T) {
	j := runJobConcurrency(t)
	gt.Value(t, j.Limit()).Equal(1)
	gt.NoError(t, j.Validate())
}

func TestJobConcurrency_ReadsFlag(t *testing.T) {
	j := runJobConcurrency(t, "--job-max-concurrency", "4")
	gt.Value(t, j.Limit()).Equal(4)
	gt.NoError(t, j.Validate())
}

func TestJobConcurrency_ReadsEnvVar(t *testing.T) {
	t.Setenv("HECATONCHEIRES_JOB_MAX_CONCURRENCY", "6")
	j := runJobConcurrency(t)
	gt.Value(t, j.Limit()).Equal(6)
}

func TestJobConcurrency_ZeroMeansNoLimit(t *testing.T) {
	j := runJobConcurrency(t, "--job-max-concurrency", "0")
	gt.Value(t, j.Limit()).Equal(0)
	// Zero is a valid, explicit opt-out; the wiring layer builds no limiter.
	gt.NoError(t, j.Validate())
}

func TestJobConcurrency_RejectsNegative(t *testing.T) {
	j := runJobConcurrency(t, "--job-max-concurrency", "-1")
	gt.Value(t, j.Limit()).Equal(-1)
	gt.Error(t, j.Validate())
}

func TestJobConcurrency_LogAttrs(t *testing.T) {
	j := runJobConcurrency(t, "--job-max-concurrency", "3")
	attrs := j.LogAttrs()
	gt.Array(t, attrs).Length(1).Required()
	gt.Value(t, attrs[0].Key).Equal("job_max_concurrency")
	gt.Value(t, attrs[0].Value.Int64()).Equal(int64(3))
}
