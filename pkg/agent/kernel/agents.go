package kernel

import "github.com/gollem-dev/agentkit"

// Agent names. These values are persisted on every Process row, so a running
// deployment always has in-flight Processes referring to them by name. Renaming
// one strands those Processes with ErrUnknownAgent and no way to finish, so the
// names are fixed for good; a change in what an agent DOES is expressed by
// bumping its strategy version, which DecodeState migrates.
const (
	// AgentCaseChannel is the ReAct agent behind a mention in a channel-mode
	// case channel.
	AgentCaseChannel agentkit.AgentName = "case-channel"
	// AgentCaseThread is the plan-execute agent behind a mention in a
	// thread-mode case thread.
	AgentCaseThread agentkit.AgentName = "case-thread"
	// AgentCaseThreadCreate is the plan-execute agent that materialises a new
	// thread-mode case from the conversation that triggered it.
	AgentCaseThreadCreate agentkit.AgentName = "case-thread-create"
	// AgentWorkspace is the cross-case plan-execute agent in a workspace
	// channel.
	AgentWorkspace agentkit.AgentName = "workspace"
	// AgentProposal is the plan-execute agent that drafts a case before one
	// exists.
	AgentProposal agentkit.AgentName = "proposal"
	// AgentJob is the plan-execute agent behind a Job configured with the
	// planexec strategy.
	AgentJob agentkit.AgentName = "job"
	// AgentJobSimple is the ReAct agent behind a Job configured with the simple
	// strategy.
	AgentJobSimple agentkit.AgentName = "job-simple"
	// AgentAssist is the ReAct agent behind the assist batch command.
	AgentAssist agentkit.AgentName = "assist"
	// AgentTask is the ReAct sub-agent a plan-execute agent spawns per planned
	// task.
	AgentTask agentkit.AgentName = "task"
)

// RequiresActor reports whether an agent may only run with an identified Slack
// user behind it.
//
// This is not a formality. A context with no auth token is read by the usecase
// layer as a system context and BYPASSES private-case access control entirely
// (see tokenActor in pkg/usecase/case_access.go). So for an agent whose tools
// reach across cases on a person's behalf, a missing actor is not "reduced
// access" — it is full access. The workspace agent is that agent: its
// cross-case tools are gated solely by the requesting user's membership, and
// its pre-agentkit host required the actor for exactly this reason.
//
// The per-case agents are deliberately NOT listed. Their pre-agentkit hosts ran
// without an auth token as well, and adding one here would silently tighten who
// the mention agent can act for. Tightening them is a separate decision from
// moving them onto this runtime.
func RequiresActor(name agentkit.AgentName) bool {
	return name == AgentWorkspace
}
