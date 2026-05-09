package agent_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
)

func TestIsKnownToolSetID(t *testing.T) {
	t.Run("known IDs return true", func(t *testing.T) {
		for _, id := range agent.KnownToolSetIDs {
			gt.Bool(t, agent.IsKnownToolSetID(id)).True()
		}
	})
	t.Run("unknown ID returns false", func(t *testing.T) {
		gt.Bool(t, agent.IsKnownToolSetID("not_a_toolset")).False()
		gt.Bool(t, agent.IsKnownToolSetID("")).False()
	})
}

func TestToolSetResolver_ResolveCore(t *testing.T) {
	r := agent.NewToolSetResolver(agent.ToolSetDeps{
		Core:   core.Deps{Repo: nil, WorkspaceID: "ws", CaseID: 1},
		Slack:  slacktool.Deps{},
		Notion: notiontool.Deps{},
		GitHub: nil,
	})
	tools := r.Resolve([]string{agent.ToolSetCoreRO})
	// core_ro is read-only: list_actions + get_action (no step UC wired here).
	gt.Array(t, tools).Length(2)
}

func TestToolSetResolver_ResolveMultipleAndUnknown(t *testing.T) {
	r := agent.NewToolSetResolver(agent.ToolSetDeps{
		Core:   core.Deps{Repo: nil, WorkspaceID: "ws", CaseID: 1},
		Slack:  slacktool.Deps{},
		Notion: notiontool.Deps{},
		GitHub: nil,
	})
	tools := r.Resolve([]string{agent.ToolSetCoreRO, "ghost_set", agent.ToolSetSlackRO})
	// Only core (2 tools) survives — ghost_set is silently skipped, slack_ro is empty
	// because Slack Deps has no Bot/Search wired.
	gt.Array(t, tools).Length(2)
}

func TestToolSetResolver_ResolveEmpty(t *testing.T) {
	r := agent.NewToolSetResolver(agent.ToolSetDeps{})
	gt.Array(t, r.Resolve(nil)).Length(0)
	gt.Array(t, r.Resolve([]string{})).Length(0)
}

func TestToolSetResolver_NilReceiver(t *testing.T) {
	var r *agent.ToolSetResolver
	gt.Array(t, r.Resolve([]string{agent.ToolSetCoreRO})).Length(0)
}
