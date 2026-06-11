package config_test

import (
	"context"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/urfave/cli/v3"
)

func runWebFetchFlags(t *testing.T, args []string) *config.WebFetch {
	t.Helper()
	var w config.WebFetch
	cmd := &cli.Command{
		Name:  "test",
		Flags: w.Flags(),
		Action: func(_ context.Context, _ *cli.Command) error {
			return nil
		},
	}
	err := cmd.Run(context.Background(), append([]string{"test"}, args...))
	gt.NoError(t, err).Required()
	return &w
}

func TestWebFetchDefaults(t *testing.T) {
	w := runWebFetchFlags(t, nil)
	gt.Bool(t, w.IsEnabled()).True()

	s := w.Settings()
	gt.Value(t, s.Timeout).Equal(10 * time.Second)
	gt.Number(t, s.MaxBytes).Equal(int64(262144))
	gt.String(t, s.UserAgent).NotEqual("")
	// The LLM client is injected by the usecase layer, not by config.
	gt.Value(t, s.LLM).Nil()
	gt.Bool(t, s.AllowPrivateIP).False()
}

func TestWebFetchExplicitFlags(t *testing.T) {
	w := runWebFetchFlags(t, []string{
		"--webfetch-enabled=false",
		"--webfetch-timeout=5",
		"--webfetch-max-size=2048",
	})
	gt.Bool(t, w.IsEnabled()).False()

	s := w.Settings()
	gt.Value(t, s.Timeout).Equal(5 * time.Second)
	gt.Number(t, s.MaxBytes).Equal(int64(2048))
}
