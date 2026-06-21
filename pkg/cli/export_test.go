package cli

import (
	"github.com/gollem-dev/gollem"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// BuildJobToolsForTest exposes buildJobTools so tests can assert the
// per-workspace-mode tool composition without standing up a full job runtime.
// Adapters are left zero-valued: buildJobTools only constructs tool structs
// (which hold their deps); the adapters are exercised at tool-call time, not at
// build time.
func BuildJobToolsForTest(c *model.Case, ws *model.WorkspaceEntry) []gollem.Tool {
	return buildJobTools(jobRuntimeDeps{}, jobToolAdapters{}, c, ws)
}
