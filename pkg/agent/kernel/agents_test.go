package kernel_test

import (
	"testing"

	"github.com/gollem-dev/agentkit"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
)

// TestRequiresActor pins which agents may only run with an identified person
// behind them. A context with no auth token is read by the usecase layer as a
// system context and BYPASSES private-case access control, so a missing actor
// widens access rather than narrowing it — but the per-case agents ran without a
// token before this runtime, and tightening them is a separate decision from
// moving them onto it.
func TestRequiresActor(t *testing.T) {
	gt.Bool(t, kernel.RequiresActor(kernel.AgentWorkspace)).True()

	for _, name := range []agentkit.AgentName{
		kernel.AgentCaseChannel,
		kernel.AgentCaseThread,
		kernel.AgentCaseThreadCreate,
		kernel.AgentProposal,
		kernel.AgentJob,
		kernel.AgentJobSimple,
		kernel.AgentAssist,
		kernel.AgentTask,
	} {
		gt.Bool(t, kernel.RequiresActor(name)).False()
	}
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

	t.Run("an agent that does not need an actor is accepted without one", func(t *testing.T) {
		gt.NoError(t, kernel.ValidateSpawn(kernel.AgentCaseChannel, valid))
	})

	t.Run("an invalid scope is rejected whatever the agent", func(t *testing.T) {
		sc := valid
		sc.ToolSets = nil
		gt.Value(t, kernel.ValidateSpawn(kernel.AgentCaseChannel, sc)).NotNil()
	})
}
