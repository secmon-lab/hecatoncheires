package planexec

import "github.com/gollem-dev/gollem"

// ToolResolver translates the symbolic Tools list inside a TaskPlan into a
// concrete []gollem.Tool slice for one sub-agent. The interface is the
// only piece of "what tools exist" planexec needs to know about; the
// concrete catalogue (workspace-scoped Slack / Notion / GitHub clients,
// the read-only core toolset, etc.) lives in the host.
type ToolResolver interface {
	// Resolve returns the concatenated tool list for the requested IDs.
	// Unknown IDs MUST be silently dropped — they should already have
	// been rejected at plan validation, and we never want to crash a
	// running sub-agent because the planner emitted a stray name.
	Resolve(ids []string) []gollem.Tool
}
