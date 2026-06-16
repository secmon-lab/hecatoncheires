package agent_test

import (
	"context"
	"testing"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/webfetch"
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

func TestToolSetResolver_ResolveWebFetch(t *testing.T) {
	llm := &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{}, nil
		},
	}
	wfClient := webfetch.NewClient(webfetch.ClientConfig{
		Timeout:   10 * time.Second,
		MaxBytes:  1024,
		UserAgent: "test",
		LLM:       llm,
	})

	t.Run("webfetch ID resolves to the single tool", func(t *testing.T) {
		r := agent.NewToolSetResolver(agent.ToolSetDeps{WebFetch: wfClient})
		tools := r.Resolve([]string{agent.ToolSetWebFetch})
		gt.Array(t, tools).Length(1).Required()
		gt.String(t, tools[0].Spec().Name).Equal("webfetch")
	})

	t.Run("combined with core returns both sets", func(t *testing.T) {
		r := agent.NewToolSetResolver(agent.ToolSetDeps{
			Core:     core.Deps{WorkspaceID: "ws", CaseID: 1},
			WebFetch: wfClient,
		})
		tools := r.Resolve([]string{agent.ToolSetCoreRO, agent.ToolSetWebFetch})
		// core_ro (2) + webfetch (1).
		gt.Array(t, tools).Length(3)
	})

	t.Run("nil webfetch client yields no tools", func(t *testing.T) {
		r := agent.NewToolSetResolver(agent.ToolSetDeps{WebFetch: nil})
		gt.Array(t, r.Resolve([]string{agent.ToolSetWebFetch})).Length(0)
	})
}
