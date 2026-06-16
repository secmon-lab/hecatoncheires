package env_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/env"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/scenario"
)

type fakeCompleter struct{}

func (fakeCompleter) Complete(_ context.Context, _, _ string, _ *gollem.Parameter) (string, error) {
	return "", nil
}

func loadScenario(t *testing.T) *scenario.Scenario {
	t.Helper()
	sc, err := scenario.Load(filepath.Join("..", "scenario", "testdata", "valid_thread_initial.toml"))
	gt.NoError(t, err)
	return sc
}

func fakeLLM() *mock.LLMClientMock {
	// Build never calls the LLM, so an empty mock suffices.
	return &mock.LLMClientMock{}
}

func TestBuild_OK(t *testing.T) {
	sc := loadScenario(t)
	e, err := env.Build(context.Background(), sc, env.Options{
		LLM:       fakeLLM(),
		Completer: fakeCompleter{},
	})
	gt.NoError(t, err)
	gt.V(t, e.AgentUC).NotNil()
	gt.V(t, e.Entry.Workspace.ID).Equal("support")
	gt.V(t, e.MonitorChannel).Equal("C0123456789")
	gt.V(t, e.Recorder).NotNil()
	gt.V(t, e.Trace).NotNil()
	gt.V(t, e.Slack).NotNil()
}

func TestBuild_SeedsSources(t *testing.T) {
	sc := loadScenario(t)
	e, err := env.Build(context.Background(), sc, env.Options{
		LLM:       fakeLLM(),
		Completer: fakeCompleter{},
	})
	gt.NoError(t, err)

	// Sources must be real repository entities (seeded via repo.Source().Create),
	// not just parsed config — read them back through the repository interface.
	sources, err := e.Repo.Source().List(context.Background(), e.Entry.Workspace.ID)
	gt.NoError(t, err)
	gt.A(t, sources).Length(2)

	byName := map[string]*model.Source{}
	for _, s := range sources {
		byName[s.Name] = s
	}

	nd := byName["Incident runbooks"]
	gt.V(t, nd).NotNil()
	gt.V(t, nd.SourceType).Equal(model.SourceTypeNotionDB)
	gt.B(t, nd.Enabled).True()
	gt.V(t, nd.NotionDBConfig).NotNil()
	gt.V(t, nd.NotionDBConfig.DatabaseID).Equal("11112222333344445555666677778888")
	gt.V(t, nd.ID).NotEqual(model.SourceID(""))

	sl := byName["Incident channel"]
	gt.V(t, sl).NotNil()
	gt.V(t, sl.SourceType).Equal(model.SourceTypeSlack)
	gt.V(t, sl.SlackConfig).NotNil()
	gt.A(t, sl.SlackConfig.Channels).Length(1)
	gt.V(t, sl.SlackConfig.Channels[0].ID).Equal("C0123456789")
}

func TestBuild_NilLLM(t *testing.T) {
	sc := loadScenario(t)
	_, err := env.Build(context.Background(), sc, env.Options{Completer: fakeCompleter{}})
	gt.Error(t, err)
}

func TestBuild_LiveSlackWithoutClient(t *testing.T) {
	sc := loadScenario(t)
	// Mark slack_search live but provide no live client.
	tool := sc.Tools["slack_search"]
	tool.Live = true
	sc.Tools["slack_search"] = tool

	_, err := env.Build(context.Background(), sc, env.Options{
		LLM:       fakeLLM(),
		Completer: fakeCompleter{},
	})
	gt.Error(t, err)
}
