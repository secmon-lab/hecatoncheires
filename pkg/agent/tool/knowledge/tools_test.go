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

// fakeAccessor records calls and returns canned results.
type fakeAccessor struct {
	searchCalls  int
	getCalls     int
	listTagCalls int
	lastQuery    string
	lastTagIDs   []model.TagID
	lastLimit    int
	items        []*model.Knowledge
	tags         []*model.Tag
}

func (f *fakeAccessor) SearchKnowledge(ctx context.Context, workspaceID, query string, tagIDs []model.TagID, limit int) ([]*model.Knowledge, error) {
	f.searchCalls++
	f.lastQuery = query
	f.lastTagIDs = tagIDs
	f.lastLimit = limit
	return f.items, nil
}

func (f *fakeAccessor) GetKnowledge(ctx context.Context, workspaceID string, id model.KnowledgeID) (*model.Knowledge, error) {
	f.getCalls++
	return f.items[0], nil
}

func (f *fakeAccessor) ListTags(ctx context.Context, workspaceID string) ([]*model.Tag, error) {
	f.listTagCalls++
	return f.tags, nil
}

// fakeMutator records write calls and returns minimal synthetic results.
type fakeMutator struct {
	createTagCalls    int
	updateTagCalls    int
	deleteTagCalls    int
	createKnowCalls   int
	updateKnowCalls   int
	lastCreateTagName string
}

func (f *fakeMutator) CreateTag(ctx context.Context, workspaceID, name string) (*model.Tag, error) {
	f.createTagCalls++
	f.lastCreateTagName = name
	return &model.Tag{
		ID:          model.NewTagID(),
		WorkspaceID: workspaceID,
		Name:        name,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}, nil
}

func (f *fakeMutator) UpdateTag(ctx context.Context, workspaceID string, id model.TagID, name string) (*model.Tag, error) {
	f.updateTagCalls++
	return &model.Tag{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        name,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}, nil
}

func (f *fakeMutator) DeleteTag(ctx context.Context, workspaceID string, id model.TagID) error {
	f.deleteTagCalls++
	return nil
}

func (f *fakeMutator) CreateKnowledge(ctx context.Context, workspaceID, title, claim string, tagIDs []model.TagID) (*model.Knowledge, error) {
	f.createKnowCalls++
	return &model.Knowledge{
		ID:          model.NewKnowledgeID(),
		WorkspaceID: workspaceID,
		Title:       title,
		Claim:       claim,
		TagIDs:      tagIDs,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}, nil
}

func (f *fakeMutator) UpdateKnowledge(ctx context.Context, workspaceID string, id model.KnowledgeID, title, claim *string, tagIDs *[]model.TagID) (*model.Knowledge, error) {
	f.updateKnowCalls++
	k := &model.Knowledge{
		ID:          id,
		WorkspaceID: workspaceID,
		TagIDs:      []model.TagID{model.NewTagID()},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if title != nil {
		k.Title = *title
	}
	if claim != nil {
		k.Claim = *claim
	}
	if tagIDs != nil {
		k.TagIDs = *tagIDs
	}
	return k, nil
}

func toolNames(tools []gollem.Tool) []string {
	names := make([]string, len(tools))
	for i, tl := range tools {
		names[i] = tl.Spec().Name
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

	// Read-only: exactly 3 tools (search / get / list_tags).
	ro := knowledgetool.NewReadOnly(knowledgetool.Deps{WorkspaceID: "ws", Accessor: acc})
	roNames := toolNames(ro)
	gt.Array(t, ro).Length(3).Required()
	gt.Bool(t, slices.Contains(roNames, "knowledge__search_knowledge")).True()
	gt.Bool(t, slices.Contains(roNames, "knowledge__get_knowledge")).True()
	gt.Bool(t, slices.Contains(roNames, "knowledge__list_tags")).True()

	// Read-only must NOT expose any write tools.
	gt.Bool(t, slices.Contains(roNames, "knowledge__create_tag")).False()
	gt.Bool(t, slices.Contains(roNames, "knowledge__update_tag")).False()
	gt.Bool(t, slices.Contains(roNames, "knowledge__delete_tag")).False()
	gt.Bool(t, slices.Contains(roNames, "knowledge__create_knowledge")).False()
	gt.Bool(t, slices.Contains(roNames, "knowledge__update_knowledge")).False()

	// Full set: 3 read + 5 write = 8 tools.
	full := knowledgetool.New(knowledgetool.Deps{WorkspaceID: "ws", Accessor: acc, Mutator: mut})
	fullNames := toolNames(full)
	gt.Array(t, full).Length(8).Required()
	gt.Bool(t, slices.Contains(fullNames, "knowledge__search_knowledge")).True()
	gt.Bool(t, slices.Contains(fullNames, "knowledge__get_knowledge")).True()
	gt.Bool(t, slices.Contains(fullNames, "knowledge__list_tags")).True()
	gt.Bool(t, slices.Contains(fullNames, "knowledge__create_tag")).True()
	gt.Bool(t, slices.Contains(fullNames, "knowledge__update_tag")).True()
	gt.Bool(t, slices.Contains(fullNames, "knowledge__delete_tag")).True()
	gt.Bool(t, slices.Contains(fullNames, "knowledge__create_knowledge")).True()
	gt.Bool(t, slices.Contains(fullNames, "knowledge__update_knowledge")).True()
}

func TestSearchToolRun(t *testing.T) {
	tagID1 := model.NewTagID()
	acc := &fakeAccessor{items: []*model.Knowledge{
		{
			ID:        "k1",
			Title:     "GitHub policy",
			Claim:     "body",
			TagIDs:    []model.TagID{tagID1},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}}
	tools := knowledgetool.NewReadOnly(knowledgetool.Deps{WorkspaceID: "ws", Accessor: acc})
	tl := findTool(t, tools, "knowledge__search_knowledge")

	out, err := tl.Run(context.Background(), map[string]any{
		"query":   "github",
		"tag_ids": []any{string(tagID1)},
		"limit":   float64(5),
	})
	gt.NoError(t, err).Required()
	gt.Number(t, acc.searchCalls).Equal(1)
	gt.String(t, acc.lastQuery).Equal("github")
	gt.Array(t, acc.lastTagIDs).Length(1).Required()
	gt.Value(t, acc.lastTagIDs[0]).Equal(tagID1)
	gt.Number(t, acc.lastLimit).Equal(5)

	knowledge, ok := out["knowledge"].([]map[string]any)
	gt.Bool(t, ok).True()
	gt.Array(t, knowledge).Length(1).Required()

	// knowledgeToMap emits tag_ids, not tags.
	item := knowledge[0]
	tagIDs, ok := item["tag_ids"].([]string)
	gt.Bool(t, ok).True()
	gt.Array(t, tagIDs).Length(1).Required()
	gt.String(t, tagIDs[0]).Equal(string(tagID1))
}

func TestSearchToolRequiresQuery(t *testing.T) {
	tools := knowledgetool.NewReadOnly(knowledgetool.Deps{WorkspaceID: "ws", Accessor: &fakeAccessor{}})
	tl := findTool(t, tools, "knowledge__search_knowledge")
	_, err := tl.Run(context.Background(), map[string]any{})
	gt.Error(t, err)
}

func TestListTagsToolRun(t *testing.T) {
	now := time.Now()
	acc := &fakeAccessor{tags: []*model.Tag{
		{ID: model.NewTagID(), Name: "github", WorkspaceID: "ws", CreatedAt: now, UpdatedAt: now},
		{ID: model.NewTagID(), Name: "ops", WorkspaceID: "ws", CreatedAt: now, UpdatedAt: now},
	}}
	tools := knowledgetool.NewReadOnly(knowledgetool.Deps{WorkspaceID: "ws", Accessor: acc})
	tl := findTool(t, tools, "knowledge__list_tags")
	out, err := tl.Run(context.Background(), map[string]any{})
	gt.NoError(t, err).Required()
	gt.Number(t, acc.listTagCalls).Equal(1)

	tags, ok := out["tags"].([]map[string]any)
	gt.Bool(t, ok).True()
	gt.Array(t, tags).Length(2).Required()

	// Each tag map must have id, name, created_at, updated_at.
	gt.Map(t, tags[0]).HasKey("id")
	gt.Map(t, tags[0]).HasKey("name")
	gt.Map(t, tags[0]).HasKey("created_at")
	gt.Map(t, tags[0]).HasKey("updated_at")
}

func TestCreateTagToolRun(t *testing.T) {
	mut := &fakeMutator{}
	tools := knowledgetool.New(knowledgetool.Deps{WorkspaceID: "ws", Accessor: &fakeAccessor{}, Mutator: mut})
	tl := findTool(t, tools, "knowledge__create_tag")

	out, err := tl.Run(context.Background(), map[string]any{
		"name": "ops",
	})
	gt.NoError(t, err).Required()
	gt.Number(t, mut.createTagCalls).Equal(1)
	gt.String(t, mut.lastCreateTagName).Equal("ops")

	// Response map has tag fields.
	gt.Map(t, out).HasKey("id")
	gt.Map(t, out).HasKey("name")
}

func TestCreateTagToolWithoutMutatorErrors(t *testing.T) {
	tools := knowledgetool.New(knowledgetool.Deps{WorkspaceID: "ws", Accessor: &fakeAccessor{}})
	tl := findTool(t, tools, "knowledge__create_tag")
	_, err := tl.Run(context.Background(), map[string]any{"name": "ops"})
	gt.Error(t, err)
}

func TestUpdateTagToolRun(t *testing.T) {
	mut := &fakeMutator{}
	tools := knowledgetool.New(knowledgetool.Deps{WorkspaceID: "ws", Accessor: &fakeAccessor{}, Mutator: mut})
	tl := findTool(t, tools, "knowledge__update_tag")

	tagID := model.NewTagID()
	out, err := tl.Run(context.Background(), map[string]any{
		"tag_id": string(tagID),
		"name":   "renamed",
	})
	gt.NoError(t, err).Required()
	gt.Number(t, mut.updateTagCalls).Equal(1)
	gt.Map(t, out).HasKey("id")
	gt.Map(t, out).HasKey("name")
}

func TestDeleteTagToolRun(t *testing.T) {
	mut := &fakeMutator{}
	tools := knowledgetool.New(knowledgetool.Deps{WorkspaceID: "ws", Accessor: &fakeAccessor{}, Mutator: mut})
	tl := findTool(t, tools, "knowledge__delete_tag")

	tagID := model.NewTagID()
	out, err := tl.Run(context.Background(), map[string]any{
		"tag_id": string(tagID),
	})
	gt.NoError(t, err).Required()
	gt.Number(t, mut.deleteTagCalls).Equal(1)
	deleted, ok := out["deleted"].(bool)
	gt.Bool(t, ok).True()
	gt.Bool(t, deleted).True()
}

func TestCreateKnowledgeToolRun(t *testing.T) {
	mut := &fakeMutator{}
	tools := knowledgetool.New(knowledgetool.Deps{WorkspaceID: "ws", Accessor: &fakeAccessor{}, Mutator: mut})
	tl := findTool(t, tools, "knowledge__create_knowledge")

	tagID := model.NewTagID()
	out, err := tl.Run(context.Background(), map[string]any{
		"title":   "GitHub policy",
		"claim":   "## body",
		"tag_ids": []any{string(tagID)},
	})
	gt.NoError(t, err).Required()
	gt.Number(t, mut.createKnowCalls).Equal(1)
	gt.String(t, out["title"].(string)).Equal("GitHub policy")

	// knowledgeToMap emits tag_ids key.
	gt.Map(t, out).HasKey("tag_ids")
}

func TestCreateKnowledgeToolWithoutMutatorErrors(t *testing.T) {
	tools := knowledgetool.New(knowledgetool.Deps{WorkspaceID: "ws", Accessor: &fakeAccessor{}})
	tl := findTool(t, tools, "knowledge__create_knowledge")
	tagID := model.NewTagID()
	_, err := tl.Run(context.Background(), map[string]any{"title": "x", "tag_ids": []any{string(tagID)}})
	gt.Error(t, err)
}

func TestUpdateKnowledgeToolRun(t *testing.T) {
	mut := &fakeMutator{}
	tools := knowledgetool.New(knowledgetool.Deps{WorkspaceID: "ws", Accessor: &fakeAccessor{}, Mutator: mut})
	tl := findTool(t, tools, "knowledge__update_knowledge")

	tagID := model.NewTagID()
	out, err := tl.Run(context.Background(), map[string]any{
		"knowledge_id": "k1",
		"title":        "new title",
		"tag_ids":      []any{string(tagID)},
	})
	gt.NoError(t, err).Required()
	gt.Number(t, mut.updateKnowCalls).Equal(1)
	gt.String(t, out["title"].(string)).Equal("new title")
	gt.Map(t, out).HasKey("tag_ids")
}
