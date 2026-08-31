package kernel_test

import (
	"testing"

	"github.com/gollem-dev/agentkit"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/budget"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
)

// TestRequiresActor pins which agents may only run with an identified person
// behind them. A context with no auth token is read by the usecase layer as a
// system context and BYPASSES private-case access control, so a missing actor
// widens access rather than narrowing it: a private case becomes readable by
// someone who is not in its channel.
func TestRequiresActor(t *testing.T) {
	t.Run("every human-triggered agent needs one", func(t *testing.T) {
		for _, name := range []agentkit.AgentName{
			kernel.AgentCaseChannel,
			kernel.AgentCaseThread,
			kernel.AgentWorkspace,
			kernel.AgentProposal,
		} {
			gt.Bool(t, kernel.RequiresActor(name)).True()
		}
	})

	// The thread-mode CREATE turn is the scoped exception: a thread-mode case may
	// be raised by an integration bot's intake post that names no human, so
	// demanding an actor would refuse a legitimate creation. It is safe because a
	// create turn's palette carries no case-reading tool — there is no private case
	// for the missing actor to widen access to.
	t.Run("the thread-mode create turn does not", func(t *testing.T) {
		gt.Bool(t, kernel.RequiresActor(kernel.AgentCaseThreadCreate)).False()
	})

	// A Job and the assist batch run on a schedule with nobody behind them, so
	// there is no actor to name. A sub-agent inherits its parent's metadata and
	// carries whatever actor the parent was given.
	t.Run("the unattended agents and sub-agents do not", func(t *testing.T) {
		for _, name := range []agentkit.AgentName{
			kernel.AgentJob,
			kernel.AgentJobSimple,
			kernel.AgentAssist,
			kernel.AgentTask,
		} {
			gt.Bool(t, kernel.RequiresActor(name)).False()
		}
	})
}

// TestValidateSpawn pins where the actor rule is enforced. Spawn is the last
// point a bad scope can be reported to a caller that can act on it: once the
// Process exists, a claim that refuses to run it is put back as pending with a
// backoff and never consumes the retry budget, so the row would requeue forever
// and hold its Subject with it.
func TestValidateSpawn(t *testing.T) {
	valid := kernel.Scope{
		WorkspaceID: "ws-1",
		ChannelID:   "C1",
		ThreadTS:    "1.1",
		ToolSets:    []string{kernel.ToolSetsAll},
	}

	t.Run("an agent that needs an actor is rejected without one", func(t *testing.T) {
		gt.Value(t, kernel.ValidateSpawn(kernel.AgentWorkspace, valid)).NotNil()
	})

	t.Run("the same agent is accepted with one", func(t *testing.T) {
		sc := valid
		sc.ActorUserID = "U1"
		gt.NoError(t, kernel.ValidateSpawn(kernel.AgentWorkspace, sc))
	})

	t.Run("an unattended agent is accepted without one", func(t *testing.T) {
		gt.NoError(t, kernel.ValidateSpawn(kernel.AgentJob, valid))
	})

	t.Run("an invalid scope is rejected whatever the agent", func(t *testing.T) {
		sc := valid
		sc.ToolSets = nil
		gt.Value(t, kernel.ValidateSpawn(kernel.AgentJob, sc)).NotNil()
	})
}

// The per-task sub-agent is registered ONCE and shared by every plan-execute
// host, because agentkit keys a Process on the agent name: a second registration
// under the same name is an error, and giving each host its own name would mean
// maintaining a separate tool palette for sub-agents that do the same thing.
func TestRegisterTaskAgentIsRegisteredOnce(t *testing.T) {
	reg := agentkit.NewRegistry()
	cfg := budget.Config{MaxSteps: 8, MaxInputTokens: 1000, MaxOutputTokens: 1000, NoticeRatio: 0.8}
	store := agentarchive.NewMemoryHistoryStore()

	handle, err := kernel.RegisterTaskAgent(reg, cfg.Limiter(testSpend()), store)
	gt.NoError(t, err).Required()
	gt.Value(t, handle.Name()).Equal(kernel.AgentTask)

	// A second registration under the same name is refused rather than silently
	// replacing the first.
	_, err = kernel.RegisterTaskAgent(reg, cfg.Limiter(testSpend()), store)
	gt.Error(t, err).Required()
}
