package wsmeta_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/wsmeta"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

// fixtureRegistry builds a small workspace registry the test cases share.
// It registers two workspaces: "ws-sec" (with a select field whose options
// have descriptions + metadata) and "ws-task" (with a free-form text field
// only). The order of List() must follow registration order.
func fixtureRegistry() *model.WorkspaceRegistry {
	r := model.NewWorkspaceRegistry()
	r.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{
			ID:          "ws-sec",
			Name:        "Security",
			Description: "Security risk management workspace",
		},
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{
					ID:          "severity",
					Name:        "Severity",
					Type:        types.FieldTypeSelect,
					Required:    true,
					Description: "Severity of the incident",
					Options: []config.FieldOption{
						{
							ID:          "low",
							Name:        "Low",
							Description: "Minor issue, no immediate action needed",
							Metadata:    map[string]any{"score": 1},
						},
						{
							ID:          "high",
							Name:        "High",
							Description: "Critical, escalate to on-call",
							// Intentionally no Metadata to verify the
							// "metadata key omitted when empty" branch.
						},
					},
				},
			},
		},
	})
	r.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{
			ID:          "ws-task",
			Name:        "Task",
			Description: "Free-form task tracking",
		},
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{
					ID:          "summary",
					Name:        "Summary",
					Type:        types.FieldTypeText,
					Required:    false,
					Description: "Short text summary",
				},
			},
		},
	})
	return r
}

func TestListWorkspaces_RegistryEmpty(t *testing.T) {
	tools := wsmeta.New(wsmeta.Deps{Registry: model.NewWorkspaceRegistry()})
	gt.Array(t, tools).Length(2).Required()

	out, err := tools[0].Run(context.Background(), nil)
	gt.NoError(t, err).Required()
	wsRaw, ok := out["workspaces"].([]map[string]any)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, wsRaw).Length(0)
}

func TestListWorkspaces_ReturnsRegistryInOrder(t *testing.T) {
	tools := wsmeta.New(wsmeta.Deps{Registry: fixtureRegistry()})
	out, err := tools[0].Run(context.Background(), nil)
	gt.NoError(t, err).Required()

	wsRaw, ok := out["workspaces"].([]map[string]any)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, wsRaw).Length(2).Required()

	gt.Value(t, wsRaw[0]["id"]).Equal("ws-sec")
	gt.Value(t, wsRaw[0]["name"]).Equal("Security")
	gt.Value(t, wsRaw[0]["description"]).Equal("Security risk management workspace")
	gt.Value(t, wsRaw[1]["id"]).Equal("ws-task")
	gt.Value(t, wsRaw[1]["name"]).Equal("Task")
	gt.Value(t, wsRaw[1]["description"]).Equal("Free-form task tracking")
}

func TestGetWorkspace_RequiresWorkspaceID(t *testing.T) {
	tools := wsmeta.New(wsmeta.Deps{Registry: fixtureRegistry()})
	_, err := tools[1].Run(context.Background(), map[string]any{})
	gt.Error(t, err)
}

func TestGetWorkspace_UnknownID(t *testing.T) {
	tools := wsmeta.New(wsmeta.Deps{Registry: fixtureRegistry()})
	_, err := tools[1].Run(context.Background(), map[string]any{"workspace_id": "nope"})
	gt.Error(t, err)
}

func TestGetWorkspace_ReturnsFieldsAndOptionDetails(t *testing.T) {
	tools := wsmeta.New(wsmeta.Deps{Registry: fixtureRegistry()})

	out, err := tools[1].Run(context.Background(), map[string]any{"workspace_id": "ws-sec"})
	gt.NoError(t, err).Required()

	gt.Value(t, out["id"]).Equal("ws-sec")
	gt.Value(t, out["name"]).Equal("Security")
	gt.Value(t, out["description"]).Equal("Security risk management workspace")

	fields, ok := out["fields"].([]map[string]any)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, fields).Length(1).Required()

	severity := fields[0]
	gt.Value(t, severity["id"]).Equal("severity")
	gt.Value(t, severity["name"]).Equal("Severity")
	gt.Value(t, severity["type"]).Equal("select")
	gt.Value(t, severity["required"]).Equal(true)
	gt.Value(t, severity["description"]).Equal("Severity of the incident")

	opts, ok := severity["options"].([]map[string]any)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, opts).Length(2).Required()

	gt.Value(t, opts[0]["id"]).Equal("low")
	gt.Value(t, opts[0]["name"]).Equal("Low")
	gt.Value(t, opts[0]["description"]).Equal("Minor issue, no immediate action needed")
	meta, ok := opts[0]["metadata"].(map[string]any)
	gt.Bool(t, ok).True().Required()
	gt.Value(t, meta["score"]).Equal(1)

	gt.Value(t, opts[1]["id"]).Equal("high")
	gt.Value(t, opts[1]["name"]).Equal("High")
	gt.Value(t, opts[1]["description"]).Equal("Critical, escalate to on-call")
	_, hasMeta := opts[1]["metadata"]
	gt.Bool(t, hasMeta).False()
}

// TestGetWorkspace_OptionDescriptionAlwaysPresent guards the contract called
// out in the spec: option.description is required, even when the source field
// definition has an empty description, so the planner can disambiguate "no
// description provided" from "key missing" without provider-specific JSON
// quirks.
func TestGetWorkspace_OptionDescriptionAlwaysPresent(t *testing.T) {
	r := model.NewWorkspaceRegistry()
	r.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws", Name: "WS"},
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{
					ID:   "status",
					Name: "Status",
					Type: types.FieldTypeSelect,
					Options: []config.FieldOption{
						{ID: "open", Name: "Open"}, // empty Description
					},
				},
			},
		},
	})

	tools := wsmeta.New(wsmeta.Deps{Registry: r})
	out, err := tools[1].Run(context.Background(), map[string]any{"workspace_id": "ws"})
	gt.NoError(t, err).Required()

	fields := out["fields"].([]map[string]any)
	gt.Array(t, fields).Length(1).Required()
	opts := fields[0]["options"].([]map[string]any)
	gt.Array(t, opts).Length(1).Required()

	desc, ok := opts[0]["description"]
	gt.Bool(t, ok).True().Required()
	gt.Value(t, desc).Equal("")
}

func TestGetWorkspace_FieldsWithoutOptions(t *testing.T) {
	tools := wsmeta.New(wsmeta.Deps{Registry: fixtureRegistry()})

	out, err := tools[1].Run(context.Background(), map[string]any{"workspace_id": "ws-task"})
	gt.NoError(t, err).Required()

	fields := out["fields"].([]map[string]any)
	gt.Array(t, fields).Length(1).Required()
	gt.Value(t, fields[0]["id"]).Equal("summary")
	gt.Value(t, fields[0]["type"]).Equal("text")
	gt.Value(t, fields[0]["required"]).Equal(false)
	_, hasOptions := fields[0]["options"]
	gt.Bool(t, hasOptions).False()
}

func TestGetWorkspace_NoSourceRepoReturnsEmptySources(t *testing.T) {
	tools := wsmeta.New(wsmeta.Deps{Registry: fixtureRegistry()})
	out, err := tools[1].Run(context.Background(), map[string]any{"workspace_id": "ws-sec"})
	gt.NoError(t, err).Required()

	srcs, ok := out["sources"].([]map[string]any)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, srcs).Length(0)
}

func TestGetWorkspace_SourcesNotionDB(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	created, err := repo.Source().Create(ctx, "ws-sec", &model.Source{
		ID:          model.NewSourceID(),
		Name:        "Security DB",
		SourceType:  model.SourceTypeNotionDB,
		Description: "Master incident log",
		Enabled:     true,
		NotionDBConfig: &model.NotionDBConfig{
			DatabaseID:    "abc-123",
			DatabaseTitle: "Incidents",
			DatabaseURL:   "https://notion.so/abc-123",
		},
	})
	gt.NoError(t, err).Required()

	tools := wsmeta.New(wsmeta.Deps{
		Registry:   fixtureRegistry(),
		SourceRepo: repo.Source(),
	})

	out, err := tools[1].Run(ctx, map[string]any{"workspace_id": "ws-sec"})
	gt.NoError(t, err).Required()

	srcs := out["sources"].([]map[string]any)
	gt.Array(t, srcs).Length(1).Required()
	gt.Value(t, srcs[0]["id"]).Equal(string(created.ID))
	gt.Value(t, srcs[0]["name"]).Equal("Security DB")
	gt.Value(t, srcs[0]["type"]).Equal("notion_db")
	gt.Value(t, srcs[0]["description"]).Equal("Master incident log")
	gt.Value(t, srcs[0]["enabled"]).Equal(true)

	cfg, ok := srcs[0]["config"].(map[string]any)
	gt.Bool(t, ok).True().Required()
	gt.Value(t, cfg["database_id"]).Equal("abc-123")
	gt.Value(t, cfg["database_title"]).Equal("Incidents")
	gt.Value(t, cfg["database_url"]).Equal("https://notion.so/abc-123")
}

func TestGetWorkspace_SourcesSlackAndGitHub(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	_, err := repo.Source().Create(ctx, "ws-sec", &model.Source{
		ID:         model.NewSourceID(),
		Name:       "#incidents",
		SourceType: model.SourceTypeSlack,
		Enabled:    true,
		SlackConfig: &model.SlackConfig{
			Channels: []model.SlackChannel{
				{ID: "C01", Name: "incidents"},
			},
		},
	})
	gt.NoError(t, err).Required()
	_, err = repo.Source().Create(ctx, "ws-sec", &model.Source{
		ID:         model.NewSourceID(),
		Name:       "infra-repo",
		SourceType: model.SourceTypeGitHub,
		Enabled:    true,
		GitHubConfig: &model.GitHubConfig{
			Repositories: []model.GitHubRepository{
				{Owner: "secmon-lab", Repo: "infra"},
			},
		},
	})
	gt.NoError(t, err).Required()

	tools := wsmeta.New(wsmeta.Deps{
		Registry:   fixtureRegistry(),
		SourceRepo: repo.Source(),
	})
	out, err := tools[1].Run(ctx, map[string]any{"workspace_id": "ws-sec"})
	gt.NoError(t, err).Required()

	srcs := out["sources"].([]map[string]any)
	gt.Array(t, srcs).Length(2).Required()

	byType := map[string]map[string]any{}
	for _, s := range srcs {
		byType[s["type"].(string)] = s
	}

	slack := byType["slack"]
	gt.Value(t, slack).NotNil()
	slackCfg := slack["config"].(map[string]any)
	channels := slackCfg["channels"].([]map[string]any)
	gt.Array(t, channels).Length(1).Required()
	gt.Value(t, channels[0]["id"]).Equal("C01")
	gt.Value(t, channels[0]["name"]).Equal("incidents")

	gh := byType["github"]
	gt.Value(t, gh).NotNil()
	ghCfg := gh["config"].(map[string]any)
	repos := ghCfg["repositories"].([]map[string]any)
	gt.Array(t, repos).Length(1).Required()
	gt.Value(t, repos[0]["owner"]).Equal("secmon-lab")
	gt.Value(t, repos[0]["repo"]).Equal("infra")
}

func TestToolSpecs(t *testing.T) {
	tools := wsmeta.New(wsmeta.Deps{Registry: fixtureRegistry()})

	listSpec := tools[0].Spec()
	gt.Value(t, listSpec.Name).Equal("list_workspaces")
	gt.Map(t, listSpec.Parameters).Length(0)

	getSpec := tools[1].Spec()
	gt.Value(t, getSpec.Name).Equal("get_workspace")
	wsParam, ok := getSpec.Parameters["workspace_id"]
	gt.Bool(t, ok).True().Required()
	gt.Bool(t, wsParam.Required).True()
}
