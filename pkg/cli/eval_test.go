package cli_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/cli"
)

func scenarioFixture() string {
	return filepath.Join("..", "usecase", "eval", "scenario", "testdata", "valid_thread_initial.toml")
}

func TestCmdEval_DryRun(t *testing.T) {
	cmd := cli.CmdEvalForTest()
	// --dryrun must not require an LLM or touch the network.
	err := cmd.Run(context.Background(), []string{"eval", "--dryrun", scenarioFixture()})
	gt.NoError(t, err)
}

func TestCmdEval_ListTools(t *testing.T) {
	cmd := cli.CmdEvalForTest()
	err := cmd.Run(context.Background(), []string{"eval", "--list-tools"})
	gt.NoError(t, err)
}

func TestCmdEval_NoPaths(t *testing.T) {
	cmd := cli.CmdEvalForTest()
	err := cmd.Run(context.Background(), []string{"eval", "--dryrun"})
	gt.Error(t, err)
}

func TestCmdEval_RunRequiresLLM(t *testing.T) {
	// Neutralise any ambient HECATONCHEIRES_LLM_PROVIDER (e.g. injected by the
	// developer's env loader) so this stays a hermetic assertion about the CLI
	// surface, not a 5-minute live run against whatever provider happens to be
	// configured. IsEnabled() keys solely on the provider being non-empty.
	t.Setenv("HECATONCHEIRES_LLM_PROVIDER", "")
	cmd := cli.CmdEvalForTest()
	// Without --dryrun and without an LLM provider, running must error.
	err := cmd.Run(context.Background(), []string{"eval", scenarioFixture()})
	gt.Error(t, err)
}
