package knowledge_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"
	knowledgetool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/knowledge"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type fakeAccessor struct {
	searchCalls int
	getCalls    int
	tagsCalls   int
	lastQuery   string
	lastTags    []string
	lastLimit   int
	items       []*model.Knowledge
	tags        []string
}

func (f *fakeAccessor) SearchKnowledge(ctx context.Context, workspaceID, query string, tags []string, limit int) ([]*model.Knowledge, error) {
	f.searchCalls++
	f.lastQuery = query
	f.lastTags = tags
	f.lastLimit = limit
	return f.items, nil
}

func (f *fakeAccessor) GetKnowledge(ctx context.Context, workspaceID string, id model.KnowledgeID) (*model.Knowledge, error) {
	f.getCalls++
	return f.items[0], nil
}

func (f *fakeAccessor) ListTags(ctx context.Context, workspaceID string) ([]string, error) {
	f.tagsCalls++
	return f.tags, nil
}

type fakeMutator struct {
	created int
	updated int
}

func (f *fakeMutator) CreateKnowledge(ctx context.Context, workspaceID, title, claim string, tags []string) (*model.Knowledge, error) {
	f.created++
	return &model.Knowledge{ID: model.NewKnowledgeID(), WorkspaceID: workspaceID, Title: title, Claim: claim, Tags: tags, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}

func (f *fakeMutator) UpdateKnowledge(ctx context.Context, workspaceID string, id model.KnowledgeID, title, claim *string, tags *[]string) (*model.Knowledge, error) {
	f.updated++
	k := &model.Knowledge{ID: id, WorkspaceID: workspaceID, Tags: []string{"ops"}, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if title != nil {
		k.Title = *title
	}
	if claim != nil {
		k.Claim = *claim
	}
	if tags != nil {
		k.Tags = *tags
	}
	return k, nil
}

func toolNames(tools []gollem.Tool) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Spec().Name
	}
	return names
}

func findTool(t *testing.T, tools []gollem.Tool, name string) gollem.Tool {
	t.Helper()
	m := make(map[string]gollem.Tool, len(tools))
	for _, tl := range tools {
		m[tl.Spec().Name] = tl
	}
	gt.Map(t, m).HasKey(name).Required()
	return m[name]
}

func TestToolSets(t *testing.T) {
	acc := &fakeAccessor{}
	mut := &fakeMutator{}

	ro := knowledgetool.NewReadOnly(knowledgetool.Deps{WorkspaceID: "ws", Accessor: acc})
	roNames := toolNames(ro)
	gt.Array(t, ro).Length(3).Required()
	gt.Bool(t, slices.Contains(roNames, "knowledge__search_knowledge")).True()
	gt.Bool(t, slices.Contains(roNames, "knowledge__get_knowledge")).True()
	gt.Bool(t, slices.Contains(roNames, "knowledge__list_tags")).True()
	// Read-only set must NOT expose write tools.
	gt.Bool(t, slices.Contains(roNames, "knowledge__create_knowledge")).False()
	gt.Bool(t, slices.Contains(roNames, "knowledge__update_knowledge")).False()

	full := knowledgetool.New(knowledgetool.Deps{WorkspaceID: "ws", Accessor: acc, Mutator: mut})
	fullNames := toolNames(full)
	gt.Array(t, full).Length(5).Required()
	gt.Bool(t, slices.Contains(fullNames, "knowledge__create_knowledge")).True()
	gt.Bool(t, slices.Contains(fullNames, "knowledge__update_knowledge")).True()
}

func TestSearchToolRun(t *testing.T) {
	acc := &fakeAccessor{items: []*model.Knowledge{
		{ID: "k1", Title: "GitHub policy", Claim: "body", Tags: []string{"ops"}, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}}
	tools := knowledgetool.NewReadOnly(knowledgetool.Deps{WorkspaceID: "ws", Accessor: acc})
	tl := findTool(t, tools, "knowledge__search_knowledge")

	out, err := tl.Run(context.Background(), map[string]any{
		"query": "github",
		"tags":  []any{"ops"},
		"limit": float64(5),
	})
	gt.NoError(t, err).Required()
	gt.Number(t, acc.searchCalls).Equal(1)
	gt.String(t, acc.lastQuery).Equal("github")
	gt.Array(t, acc.lastTags).Length(1).Required()
	gt.Value(t, acc.lastTags[0]).Equal("ops")
	gt.Number(t, acc.lastLimit).Equal(5)
	items, ok := out["knowledge"].([]map[string]any)
	gt.Bool(t, ok).True()
	gt.Array(t, items).Length(1)
}

func TestSearchToolRequiresQuery(t *testing.T) {
	tools := knowledgetool.NewReadOnly(knowledgetool.Deps{WorkspaceID: "ws", Accessor: &fakeAccessor{}})
	tl := findTool(t, tools, "knowledge__search_knowledge")
	_, err := tl.Run(context.Background(), map[string]any{})
	gt.Error(t, err)
}

func TestCreateToolRun(t *testing.T) {
	mut := &fakeMutator{}
	tools := knowledgetool.New(knowledgetool.Deps{WorkspaceID: "ws", Accessor: &fakeAccessor{}, Mutator: mut})
	tl := findTool(t, tools, "knowledge__create_knowledge")

	out, err := tl.Run(context.Background(), map[string]any{
		"title": "GitHub policy",
		"claim": "## body",
		"tags":  []any{"ops", "github"},
	})
	gt.NoError(t, err).Required()
	gt.Number(t, mut.created).Equal(1)
	gt.String(t, out["title"].(string)).Equal("GitHub policy")
}

func TestCreateToolWithoutMutatorErrors(t *testing.T) {
	// New always includes the create tool; if it is somehow built without a
	// mutator (a wiring bug) the tool must fail loudly rather than silently.
	tools := knowledgetool.New(knowledgetool.Deps{WorkspaceID: "ws", Accessor: &fakeAccessor{}})
	tl := findTool(t, tools, "knowledge__create_knowledge")
	_, err := tl.Run(context.Background(), map[string]any{"title": "x", "tags": []any{"ops"}})
	gt.Error(t, err)
}

func TestUpdateToolRun(t *testing.T) {
	mut := &fakeMutator{}
	tools := knowledgetool.New(knowledgetool.Deps{WorkspaceID: "ws", Accessor: &fakeAccessor{}, Mutator: mut})
	tl := findTool(t, tools, "knowledge__update_knowledge")

	out, err := tl.Run(context.Background(), map[string]any{
		"knowledge_id": "k1",
		"title":        "new title",
		"tags":         []any{"ops", "updated"},
	})
	gt.NoError(t, err).Required()
	gt.Number(t, mut.updated).Equal(1)
	gt.String(t, out["title"].(string)).Equal("new title")
}

func TestListTagsToolRun(t *testing.T) {
	acc := &fakeAccessor{tags: []string{"github", "ops"}}
	tools := knowledgetool.NewReadOnly(knowledgetool.Deps{WorkspaceID: "ws", Accessor: acc})
	tl := findTool(t, tools, "knowledge__list_tags")
	out, err := tl.Run(context.Background(), map[string]any{})
	gt.NoError(t, err).Required()
	gt.Number(t, acc.tagsCalls).Equal(1)
	tags, ok := out["tags"].([]string)
	gt.Bool(t, ok).True()
	gt.Array(t, tags).Length(2)
}
