package memo_test

import (
	"context"
	"testing"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"
	memotool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/memo"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

const testWS = "ws"
const testCaseID int64 = 7

func memoSchema() *config.FieldSchema {
	return &config.FieldSchema{Fields: []config.FieldDefinition{
		{ID: "memo_type", Name: "Type", Type: types.FieldTypeSelect, Required: true, Options: []config.FieldOption{
			{ID: "fact", Name: "Fact"}, {ID: "hypothesis", Name: "Hypothesis"},
		}},
		{ID: "tags", Name: "Tags", Type: types.FieldTypeMultiSelect, Options: []config.FieldOption{
			{ID: "a", Name: "A"}, {ID: "b", Name: "B"},
		}},
	}}
}

// fakeMutator records calls and returns a canned memo.
type fakeMutator struct {
	createTitle  string
	createFields map[string]model.FieldValue
	updateID     model.MemoID
	updateTitle  *string
	updateFields map[string]model.FieldValue
	archiveID    model.MemoID
}

func (f *fakeMutator) CreateMemo(_ context.Context, _ string, caseID int64, title string, fields map[string]model.FieldValue) (*model.Memo, error) {
	f.createTitle = title
	f.createFields = fields
	return &model.Memo{ID: model.NewMemoID(), CaseID: caseID, Title: title, FieldValues: fields}, nil
}

func (f *fakeMutator) UpdateMemo(_ context.Context, _ string, caseID int64, id model.MemoID, title *string, fields map[string]model.FieldValue) (*model.Memo, error) {
	f.updateID = id
	f.updateTitle = title
	f.updateFields = fields
	return &model.Memo{ID: id, CaseID: caseID, Title: deref(title)}, nil
}

func (f *fakeMutator) ArchiveMemo(_ context.Context, _ string, caseID int64, id model.MemoID) (*model.Memo, error) {
	f.archiveID = id
	at := time.Now()
	return &model.Memo{ID: id, CaseID: caseID, Title: "archived", ArchivedAt: &at}, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func findTool(t *testing.T, tools []gollem.Tool, name string) gollem.Tool {
	t.Helper()
	for _, tl := range tools {
		if tl.Spec().Name == name {
			return tl
		}
	}
	t.Fatalf("tool %q not found", name)
	return nil
}

func TestCreateMemoTool(t *testing.T) {
	fake := &fakeMutator{}
	repo := memory.New()
	tools := memotool.New(memotool.Deps{Repo: repo, WorkspaceID: testWS, CaseID: testCaseID, MemoUC: fake, Schema: memoSchema()})
	create := findTool(t, tools, "memo__create_memo")

	res, err := create.Run(context.Background(), map[string]any{
		"title": "a memo",
		"fields": []any{
			map[string]any{"field_id": "memo_type", "value": "fact"},
			map[string]any{"field_id": "tags", "values": []any{"a", "b"}},
		},
	})
	gt.NoError(t, err).Required()
	gt.String(t, fake.createTitle).Equal("a memo")
	gt.Value(t, fake.createFields["memo_type"].Value).Equal("fact")
	gt.Value(t, fake.createFields["tags"].Value).Equal([]string{"a", "b"})
	gt.Value(t, res["title"]).Equal("a memo")
}

func TestCreateMemoTool_RequiresTitle(t *testing.T) {
	fake := &fakeMutator{}
	tools := memotool.New(memotool.Deps{Repo: memory.New(), WorkspaceID: testWS, CaseID: testCaseID, MemoUC: fake, Schema: memoSchema()})
	create := findTool(t, tools, "memo__create_memo")
	_, err := create.Run(context.Background(), map[string]any{})
	gt.Error(t, err)
}

func TestCreateMemoTool_FieldsWithoutSchema(t *testing.T) {
	fake := &fakeMutator{}
	tools := memotool.New(memotool.Deps{Repo: memory.New(), WorkspaceID: testWS, CaseID: testCaseID, MemoUC: fake, Schema: nil})
	create := findTool(t, tools, "memo__create_memo")
	_, err := create.Run(context.Background(), map[string]any{
		"title":  "x",
		"fields": []any{map[string]any{"field_id": "memo_type", "value": "fact"}},
	})
	gt.Error(t, err)
}

func TestUpdateMemoTool(t *testing.T) {
	fake := &fakeMutator{}
	tools := memotool.New(memotool.Deps{Repo: memory.New(), WorkspaceID: testWS, CaseID: testCaseID, MemoUC: fake, Schema: memoSchema()})
	update := findTool(t, tools, "memo__update_memo")

	_, err := update.Run(context.Background(), map[string]any{
		"memo_id": "11111111-1111-7111-8111-111111111111",
		"title":   "new title",
		"fields":  []any{map[string]any{"field_id": "memo_type", "value": "hypothesis"}},
	})
	gt.NoError(t, err).Required()
	gt.Value(t, fake.updateID).Equal(model.MemoID("11111111-1111-7111-8111-111111111111"))
	gt.String(t, deref(fake.updateTitle)).Equal("new title")
	gt.Value(t, fake.updateFields["memo_type"].Value).Equal("hypothesis")
}

func TestUpdateMemoTool_RequiresMemoID(t *testing.T) {
	fake := &fakeMutator{}
	tools := memotool.New(memotool.Deps{Repo: memory.New(), WorkspaceID: testWS, CaseID: testCaseID, MemoUC: fake, Schema: memoSchema()})
	update := findTool(t, tools, "memo__update_memo")
	_, err := update.Run(context.Background(), map[string]any{"title": "x"})
	gt.Error(t, err)
}

func TestArchiveMemoTool(t *testing.T) {
	fake := &fakeMutator{}
	tools := memotool.New(memotool.Deps{Repo: memory.New(), WorkspaceID: testWS, CaseID: testCaseID, MemoUC: fake, Schema: memoSchema()})
	archive := findTool(t, tools, "memo__archive_memo")
	res, err := archive.Run(context.Background(), map[string]any{"memo_id": "22222222-2222-7222-8222-222222222222"})
	gt.NoError(t, err).Required()
	gt.Value(t, fake.archiveID).Equal(model.MemoID("22222222-2222-7222-8222-222222222222"))
	gt.Value(t, res["archived"]).Equal(true)
}

func TestListMemosTool_ExcludesArchivedByDefault(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	now := time.Now()
	active := &model.Memo{ID: model.NewMemoID(), WorkspaceID: testWS, CaseID: testCaseID, Title: "active", CreatedAt: now, UpdatedAt: now}
	at := now
	archived := &model.Memo{ID: model.NewMemoID(), WorkspaceID: testWS, CaseID: testCaseID, Title: "archived", ArchivedAt: &at, CreatedAt: now, UpdatedAt: now}
	_, err := repo.Memo().Create(ctx, testWS, active)
	gt.NoError(t, err).Required()
	_, err = repo.Memo().Create(ctx, testWS, archived)
	gt.NoError(t, err).Required()

	tools := memotool.New(memotool.Deps{Repo: repo, WorkspaceID: testWS, CaseID: testCaseID, MemoUC: &fakeMutator{}, Schema: memoSchema()})
	list := findTool(t, tools, "memo__list_memos")

	res, err := list.Run(ctx, map[string]any{})
	gt.NoError(t, err).Required()
	items := res["memos"].([]map[string]any)
	gt.Array(t, items).Length(1).Required()
	gt.Value(t, items[0]["title"]).Equal("active")

	resAll, err := list.Run(ctx, map[string]any{"include_archived": true})
	gt.NoError(t, err).Required()
	gt.Array(t, resAll["memos"].([]map[string]any)).Length(2)
}

// listToolWithMemos seeds a memory repository with the given memos and returns
// the memo__list_memos tool bound to it.
func listToolWithMemos(t *testing.T, memos ...*model.Memo) gollem.Tool {
	t.Helper()
	repo := memory.New()
	ctx := context.Background()
	for _, m := range memos {
		_, err := repo.Memo().Create(ctx, testWS, m)
		gt.NoError(t, err).Required()
	}
	tools := memotool.New(memotool.Deps{Repo: repo, WorkspaceID: testWS, CaseID: testCaseID, MemoUC: &fakeMutator{}, Schema: memoSchema()})
	return findTool(t, tools, "memo__list_memos")
}

func memoCreatedAt(title string, createdAt time.Time) *model.Memo {
	return &model.Memo{
		ID: model.NewMemoID(), WorkspaceID: testWS, CaseID: testCaseID,
		Title: title, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func TestListMemosTool_FiltersByCreatedAt(t *testing.T) {
	now := time.Now().UTC()
	old := memoCreatedAt("ten days ago", now.Add(-10*24*time.Hour))
	recent := memoCreatedAt("three days ago", now.Add(-3*24*time.Hour))
	newest := memoCreatedAt("one hour ago", now.Add(-time.Hour))
	list := listToolWithMemos(t, old, recent, newest)
	ctx := context.Background()

	sevenDaysAgo := now.Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	res, err := list.Run(ctx, map[string]any{"created_after": sevenDaysAgo})
	gt.NoError(t, err).Required()
	items := res["memos"].([]map[string]any)
	gt.Array(t, items).Length(2).Required()
	gt.Value(t, items[0]["title"]).Equal("three days ago")
	gt.Value(t, items[1]["title"]).Equal("one hour ago")

	twoDaysAgo := now.Add(-2 * 24 * time.Hour).Format(time.RFC3339)
	windowed, err := list.Run(ctx, map[string]any{
		"created_after":  sevenDaysAgo,
		"created_before": twoDaysAgo,
	})
	gt.NoError(t, err).Required()
	windowedItems := windowed["memos"].([]map[string]any)
	gt.Array(t, windowedItems).Length(1).Required()
	gt.Value(t, windowedItems[0]["title"]).Equal("three days ago")

	onlyBefore, err := list.Run(ctx, map[string]any{"created_before": sevenDaysAgo})
	gt.NoError(t, err).Required()
	beforeItems := onlyBefore["memos"].([]map[string]any)
	gt.Array(t, beforeItems).Length(1).Required()
	gt.Value(t, beforeItems[0]["title"]).Equal("ten days ago")
}

func TestListMemosTool_CreatedAtBoundaries(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second)
	onBoundary := memoCreatedAt("on boundary", base)
	later := memoCreatedAt("one hour later", base.Add(time.Hour))
	list := listToolWithMemos(t, onBoundary, later)
	ctx := context.Background()

	// created_after includes a memo created exactly at the bound.
	after, err := list.Run(ctx, map[string]any{"created_after": base.Format(time.RFC3339)})
	gt.NoError(t, err).Required()
	afterItems := after["memos"].([]map[string]any)
	gt.Array(t, afterItems).Length(2).Required()
	gt.Value(t, afterItems[0]["title"]).Equal("on boundary")
	gt.Value(t, afterItems[1]["title"]).Equal("one hour later")

	// created_before excludes a memo created exactly at the bound.
	before, err := list.Run(ctx, map[string]any{"created_before": base.Add(time.Hour).Format(time.RFC3339)})
	gt.NoError(t, err).Required()
	beforeItems := before["memos"].([]map[string]any)
	gt.Array(t, beforeItems).Length(1).Required()
	gt.Value(t, beforeItems[0]["title"]).Equal("on boundary")
}

func TestListMemosTool_CreatedAtEmptyStringIsUnset(t *testing.T) {
	now := time.Now().UTC()
	list := listToolWithMemos(t,
		memoCreatedAt("older", now.Add(-48*time.Hour)),
		memoCreatedAt("newer", now.Add(-time.Hour)),
	)

	res, err := list.Run(context.Background(), map[string]any{"created_after": "", "created_before": ""})
	gt.NoError(t, err).Required()
	items := res["memos"].([]map[string]any)
	gt.Array(t, items).Length(2).Required()
	gt.Value(t, items[0]["title"]).Equal("older")
	gt.Value(t, items[1]["title"]).Equal("newer")
}

func TestListMemosTool_RejectsInvalidCreatedAt(t *testing.T) {
	now := time.Now().UTC()
	list := listToolWithMemos(t, memoCreatedAt("only memo", now))
	ctx := context.Background()

	res, err := list.Run(ctx, map[string]any{"created_after": "7 days ago"})
	gt.Value(t, err).NotNil()
	gt.Value(t, res).Nil()

	res, err = list.Run(ctx, map[string]any{"created_before": "2026-08-04"})
	gt.Value(t, err).NotNil()
	gt.Value(t, res).Nil()

	res, err = list.Run(ctx, map[string]any{"created_after": float64(7)})
	gt.Value(t, err).NotNil()
	gt.Value(t, res).Nil()
}

func TestListMemosTool_RejectsInvertedWindow(t *testing.T) {
	now := time.Now().UTC()
	list := listToolWithMemos(t, memoCreatedAt("only memo", now))

	res, err := list.Run(context.Background(), map[string]any{
		"created_after":  now.Format(time.RFC3339),
		"created_before": now.Add(-time.Hour).Format(time.RFC3339),
	})
	gt.Error(t, err).Is(interfaces.ErrMemoListOptions)
	gt.Value(t, res).Nil()
}

func TestListMemosTool_SpecAdvertisesCreatedAtWindow(t *testing.T) {
	list := listToolWithMemos(t)
	spec := list.Spec()
	gt.Map(t, spec.Parameters).HasKey("created_after")
	gt.Map(t, spec.Parameters).HasKey("created_before")
	gt.Value(t, spec.Parameters["created_after"].Type).Equal(gollem.TypeString)
	gt.Value(t, spec.Parameters["created_before"].Type).Equal(gollem.TypeString)
	gt.String(t, spec.Parameters["created_after"].Description).Contains("inclusive")
	gt.String(t, spec.Parameters["created_before"].Description).Contains("exclusive")
}

func TestGetMemoTool(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()
	now := time.Now()
	m := &model.Memo{ID: model.NewMemoID(), WorkspaceID: testWS, CaseID: testCaseID, Title: "the memo", CreatedAt: now, UpdatedAt: now}
	_, err := repo.Memo().Create(ctx, testWS, m)
	gt.NoError(t, err).Required()

	tools := memotool.New(memotool.Deps{Repo: repo, WorkspaceID: testWS, CaseID: testCaseID, MemoUC: &fakeMutator{}, Schema: memoSchema()})
	get := findTool(t, tools, "memo__get_memo")
	res, err := get.Run(ctx, map[string]any{"memo_id": string(m.ID)})
	gt.NoError(t, err).Required()
	gt.Value(t, res["title"]).Equal("the memo")
	gt.Value(t, res["id"]).Equal(string(m.ID))
}

func TestNewReadOnly_OmitsWriters(t *testing.T) {
	tools := memotool.NewReadOnly(memotool.Deps{Repo: memory.New(), WorkspaceID: testWS, CaseID: testCaseID})
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Spec().Name] = true
	}
	gt.Bool(t, names["memo__list_memos"]).True()
	gt.Bool(t, names["memo__get_memo"]).True()
	gt.Bool(t, names["memo__create_memo"]).False()
	gt.Bool(t, names["memo__archive_memo"]).False()
	_ = interfaces.MemoArchiveScopeActiveOnly
}
