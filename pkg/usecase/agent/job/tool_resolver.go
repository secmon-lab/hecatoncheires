package job

import (
	"slices"

	"github.com/m-mizutani/gollem"

	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
)

// jobToolID is the only ToolResolver ID exposed to the planner in the
// planexec-strategy Job path. Jobs hand the executor a single resolved
// tool slice (the Case-scoped read+write set the ToolBuilder produced),
// so there is no per-task sub-selection to make — every task gets the
// same bucket of tools. The planner emits `"tools": ["default"]` on
// every TaskPlan, and Resolve returns the full slice.
const jobToolID = "default"

// jobToolResolver is the planexec.ToolResolver implementation used when
// JobStrategy == planexec. It wraps the resolved tool list produced by
// JobRunner.ToolBuilder.Build and exposes a single named bucket.
type jobToolResolver struct {
	tools []gollem.Tool
}

// newJobToolResolver wraps the supplied tool list. nil is fine — Resolve
// then returns nil for every call.
func newJobToolResolver(tools []gollem.Tool) *jobToolResolver {
	return &jobToolResolver{tools: tools}
}

// Resolve returns the wrapped tools whenever the requested IDs include
// jobToolID. Unknown IDs are silently dropped per the planexec contract.
func (r *jobToolResolver) Resolve(ids []string) []gollem.Tool {
	if r == nil || len(ids) == 0 {
		return nil
	}
	if slices.Contains(ids, jobToolID) {
		return r.tools
	}
	return nil
}

// KnownIDs returns the single allowed ID; used by Job wiring to populate
// RunRequest.KnownToolIDs.
func (r *jobToolResolver) KnownIDs() []string {
	return []string{jobToolID}
}

// Compile-time assertion: jobToolResolver satisfies planexec.ToolResolver.
var _ planexec.ToolResolver = (*jobToolResolver)(nil)
