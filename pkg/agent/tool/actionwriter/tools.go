// Package actionwriter exposes the action-mutation gollem tools available
// to event-driven Agent Jobs. It is a thin selector over pkg/agent/tool/core:
// the writer-but-not-destructive tools (create / update / step add+done+rename)
// are forwarded, while archive / unarchive / delete_action_step are
// intentionally withheld. Jobs operate unattended and an auto-archive from a
// misjudgement is strictly worse than leaving the row in place for a human.
package actionwriter

import (
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
)

// Deps groups the dependencies the actionwriter tools need.
type Deps = core.Deps

// New returns the writer subset of action tools available to Jobs.
func New(deps Deps) []gollem.Tool {
	return core.NewWriterForJob(deps)
}
