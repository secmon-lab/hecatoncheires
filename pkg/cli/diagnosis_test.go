package cli

import (
	"testing"

	"github.com/m-mizutani/gt"
)

func TestCmdDiagnosis(t *testing.T) {
	t.Run("registers fix-unsent-action subcommand", func(t *testing.T) {
		cmd := cmdDiagnosis()
		gt.Value(t, cmd.Name).Equal("diagnosis")

		// The diagnosis umbrella exists only to host repair / inspection
		// jobs; verify the fix-unsent-action sub-subcommand is wired so a
		// future refactor that drops it from the Commands slice fails the
		// test instead of silently disabling the repair entry point.
		var found bool
		for _, sub := range cmd.Commands {
			if sub.Name == "fix-unsent-action" {
				found = true
				break
			}
		}
		gt.Bool(t, found).True()
	})

	t.Run("fix-unsent-action declares its required flags", func(t *testing.T) {
		cmd := cmdFixUnsentAction()
		gt.Value(t, cmd.Name).Equal("fix-unsent-action")

		// Sanity check that flags are declared. The exact set comes from
		// the embedded config blocks (Repository / Slack / etc.), so we
		// only assert non-empty rather than enumerate them.
		gt.Number(t, len(cmd.Flags)).GreaterOrEqual(1)
	})
}
