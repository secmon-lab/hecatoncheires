package config_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/urfave/cli/v3"
)

func runSlackToolFlags(t *testing.T, args []string) *config.SlackTool {
	t.Helper()
	var s config.SlackTool
	cmd := &cli.Command{
		Name:  "test",
		Flags: s.Flags(),
		Action: func(_ context.Context, _ *cli.Command) error {
			return nil
		},
	}
	err := cmd.Run(context.Background(), append([]string{"test"}, args...))
	gt.NoError(t, err).Required()
	return &s
}

func TestSlackToolDefaults(t *testing.T) {
	// A deployment that sets neither flag must still be bounded: the whole
	// point of the pair is that an operator does not have to opt in.
	s := runSlackToolFlags(t, nil)

	limits := s.Limits()
	gt.Number(t, limits.MaxTextBytes).Equal(4096)
	gt.Number(t, limits.MaxResultBytes).Equal(32768)
}

func TestSlackToolExplicitFlags(t *testing.T) {
	s := runSlackToolFlags(t, []string{
		"--slack-tool-max-text-size=512",
		"--slack-tool-max-result-size=2048",
	})

	limits := s.Limits()
	gt.Number(t, limits.MaxTextBytes).Equal(512)
	gt.Number(t, limits.MaxResultBytes).Equal(2048)
}

func TestSlackToolZeroDisablesTheBound(t *testing.T) {
	// 0 is the documented way to turn a bound off, so it must reach the tools
	// as 0 rather than being replaced by the default.
	s := runSlackToolFlags(t, []string{
		"--slack-tool-max-text-size=0",
		"--slack-tool-max-result-size=0",
	})

	limits := s.Limits()
	gt.Number(t, limits.MaxTextBytes).Equal(0)
	gt.Number(t, limits.MaxResultBytes).Equal(0)
}

func TestSlackToolRejectsANegativeBound(t *testing.T) {
	// The tools read any non-positive value as "this bound is off", so a `-1`
	// typo would start a deployment with Slack reads unbounded. It must fail at
	// startup instead.
	neg := runSlackToolFlags(t, []string{"--slack-tool-max-text-size=-1"})
	gt.Error(t, neg.Validate())

	neg = runSlackToolFlags(t, []string{"--slack-tool-max-result-size=-1"})
	gt.Error(t, neg.Validate())

	// 0 is the documented disable value and stays valid.
	zero := runSlackToolFlags(t, []string{
		"--slack-tool-max-text-size=0",
		"--slack-tool-max-result-size=0",
	})
	gt.NoError(t, zero.Validate())

	gt.NoError(t, runSlackToolFlags(t, nil).Validate())
}

func TestSlackToolEnvVars(t *testing.T) {
	t.Setenv("HECATONCHEIRES_SLACK_TOOL_MAX_TEXT_SIZE", "1024")
	s := runSlackToolFlags(t, nil)

	limits := s.Limits()
	gt.Number(t, limits.MaxTextBytes).Equal(1024)
	// The other bound keeps its default.
	gt.Number(t, limits.MaxResultBytes).Equal(32768)
}
