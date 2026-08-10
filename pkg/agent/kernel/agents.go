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
