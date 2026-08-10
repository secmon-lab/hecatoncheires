package kernel_test

import (
	"context"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gollem-dev/agentkit"
	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
)

func testRegistry(entries ...*model.WorkspaceEntry) *model.WorkspaceRegistry {
	r := model.NewWorkspaceRegistry()
	for _, e := range entries {
		r.Register(e)
	}
	return r
}

func channelWorkspace() *model.WorkspaceEntry {
	return &model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws-1", Name: "Workspace One"}}
}

func toolNames(tools []gollem.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tl := range tools {
		names = append(names, tl.Spec().Name)
	}
	sort.Strings(names)
	return names
}

func hasPrefixIn(names []string, prefix string) bool {
	for _, n := range names {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}

func newProcess(name agentkit.AgentName, sc kernel.Scope) *agentkit.Process {
	return &agentkit.Process{
		ID:       "proc-1",
		Agent:    name,
		Status:   agentkit.ProcessPending,
		RootID:   "proc-1",
		Metadata: sc.Metadata(),
	}
}

func TestNewToolFactoryValidatesDeps(t *testing.T) {
	testCases := map[string]kernel.ToolDeps{
		"no repository": {Registry: testRegistry()},
		"no registry":   {Repo: memory.New()},
	}
	for name, deps := range testCases {
		t.Run(name, func(t *testing.T) {
			factory, err := kernel.NewToolFactory(deps)
			gt.Value(t, err).NotNil()
			gt.Value(t, factory).Nil()
		})
	}
}

// TestToolFactoryExpandsTheAgentPalette pins that "*" resolves to the palette of
// the agent kind, not to one shared list. A wrong expansion is silent: the agent
// simply has tools it should not, or lacks tools its prompt tells it to use.
func TestToolFactoryExpandsTheAgentPalette(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	factory, err := kernel.NewToolFactory(kernel.ToolDeps{
		Repo:     repo,
		Registry: testRegistry(channelWorkspace()),
	})
	gt.NoError(t, err).Required()

	sc := kernel.Scope{WorkspaceID: "ws-1", ToolSets: []string{kernel.ToolSetsAll}}

	t.Run("the channel case agent gets the mutating action tools", func(t *testing.T) {
		tools, err := factory(ctx, newProcess(kernel.AgentCaseChannel, sc))
		gt.NoError(t, err).Required()
		names := toolNames(tools)
		gt.Bool(t, slices.Contains(names, "core__create_action")).True()
		gt.Bool(t, slices.Contains(names, "core__list_actions")).True()
	})

	t.Run("the thread case agent gets no action tools at all", func(t *testing.T) {
		tools, err := factory(ctx, newProcess(kernel.AgentCaseThread, sc))
		gt.NoError(t, err).Required()
		names := toolNames(tools)
		gt.Bool(t, hasPrefixIn(names, "core__")).False()
	})

	t.Run("the create agent gets neither action nor case-write tools", func(t *testing.T) {
		tools, err := factory(ctx, newProcess(kernel.AgentCaseThreadCreate, sc))
		gt.NoError(t, err).Required()
		names := toolNames(tools)
		gt.Bool(t, hasPrefixIn(names, "core__")).False()
		gt.Bool(t, hasPrefixIn(names, "case__")).False()
	})
}

// TestToolFactoryHonoursAnExplicitSubset pins that a sub-agent gets exactly the
// toolsets its task was planned with, not the whole palette.
func TestToolFactoryHonoursAnExplicitSubset(t *testing.T) {
	ctx := context.Background()
	factory, err := kernel.NewToolFactory(kernel.ToolDeps{
		Repo:     memory.New(),
		Registry: testRegistry(channelWorkspace()),
	})
	gt.NoError(t, err).Required()

	sc := kernel.Scope{WorkspaceID: "ws-1", ToolSets: []string{agent.ToolSetCoreRO}}
	tools, err := factory(ctx, newProcess(kernel.AgentTask, sc))
	gt.NoError(t, err).Required()

	gt.Array(t, toolNames(tools)).Equal([]string{"core__get_action", "core__list_actions"})
}

// TestToolFactoryWithdrawsActionToolsForAThreadBoundCase pins the rule that a
// thread-mode case has no Actions: offering the tools would hand the agent
// something that can only return an error.
func TestToolFactoryWithdrawsActionToolsForAThreadBoundCase(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	ctx = seedCase(t, ctx, repo, &model.Case{
		Title:         "thread bound",
		SlackThreadTS: "1700000000.000100",
	})

	factory, err := kernel.NewToolFactory(kernel.ToolDeps{
		Repo:     repo,
		Registry: testRegistry(channelWorkspace()),
	})
	gt.NoError(t, err).Required()

	sc := kernel.Scope{WorkspaceID: "ws-1", CaseID: 1, ToolSets: []string{agent.ToolSetCore, agent.ToolSetCoreRO}}
	tools, err := factory(ctx, newProcess(kernel.AgentCaseChannel, sc))
	gt.NoError(t, err).Required()

	gt.Bool(t, hasPrefixIn(toolNames(tools), "core__")).False()
}

// TestToolFactoryFailsOnAnUnknownWorkspace pins that a Process naming a
// workspace this deployment does not configure fails the claim. Handing the
// agent an empty tool set instead would let it run to a confident, tool-less
// conclusion and hide the misconfiguration.
func TestToolFactoryFailsOnAnUnknownWorkspace(t *testing.T) {
	ctx := context.Background()
	factory, err := kernel.NewToolFactory(kernel.ToolDeps{
		Repo:     memory.New(),
		Registry: testRegistry(),
	})
	gt.NoError(t, err).Required()

	sc := kernel.Scope{WorkspaceID: "missing", ToolSets: []string{kernel.ToolSetsAll}}
	tools, err := factory(ctx, newProcess(kernel.AgentCaseChannel, sc))
	gt.Value(t, err).NotNil()
	gt.Array(t, tools).Length(0)
}

// seedCase persists one case so the factory can load it, and returns the ctx
// unchanged so the caller reads naturally.
func seedCase(t *testing.T, ctx context.Context, repo interfaces.Repository, c *model.Case) context.Context {
	t.Helper()
	now := time.Now().UTC()
	c.ReporterID = "U1"
	c.CreatedAt = now
	c.UpdatedAt = now
	_, err := repo.Case().Create(ctx, "ws-1", c)
	gt.NoError(t, err).Required()
	return ctx
}
