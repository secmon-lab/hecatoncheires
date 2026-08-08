package memo_test

import (
	"context"
	"fmt"
	"math"
	"strings"
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

func TestListMemosTool_NeverReturnsArchived(t *testing.T) {
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

	// The archived memo is out of reach whether or not the caller asks for it:
	// include_archived is no longer part of the tool spec and is ignored.
	for _, args := range []map[string]any{{}, {"include_archived": true}} {
		res, err := list.Run(ctx, args)
		gt.NoError(t, err).Required()
		items := res["memos"].([]map[string]any)
		gt.Array(t, items).Length(1).Required()
		gt.Value(t, items[0]["title"]).Equal("active")
		gt.Value(t, res["total_count"]).Equal(1)
		gt.Value(t, res["returned_count"]).Equal(1)
		gt.Value(t, res["has_more"]).Equal(false)
	}
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

// listToolWithSeries seeds n memos one minute apart, titled "memo-0" (oldest)
// through "memo-(n-1)" (newest), and returns the memo__list_memos tool bound to
// them. The titles encode the age order so a page can be asserted by name.
func listToolWithSeries(t *testing.T, n int) gollem.Tool {
	t.Helper()
	base := time.Now().UTC().Add(-time.Duration(n) * time.Minute)
	memos := make([]*model.Memo, 0, n)
	for i := range n {
		memos = append(memos, memoCreatedAt(fmt.Sprintf("memo-%d", i), base.Add(time.Duration(i)*time.Minute)))
	}
	return listToolWithMemos(t, memos...)
}

// descTitles builds the titles a newest-first page starting at series index
// `from` should contain, e.g. descTitles(24, 3) == memo-24, memo-23, memo-22.
func descTitles(from, count int) []string {
	out := make([]string, 0, count)
	for i := range count {
		out = append(out, fmt.Sprintf("memo-%d", from-i))
	}
	return out
}

func memoTitles(items []map[string]any) []string {
	out := make([]string, 0, len(items))
	for _, m := range items {
		title, _ := m["title"].(string)
		out = append(out, title)
	}
	return out
}

func memoIDs(items []map[string]any) []string {
	out := make([]string, 0, len(items))
	for _, m := range items {
		id, _ := m["id"].(string)
		out = append(out, id)
	}
	return out
}

// runList runs the tool and returns the page items alongside the raw response
// so a caller can assert both the contents and the paging counters.
func runList(t *testing.T, list gollem.Tool, args map[string]any) (map[string]any, []map[string]any) {
	t.Helper()
	res, err := list.Run(context.Background(), args)
	gt.NoError(t, err).Required()
	items, ok := res["memos"].([]map[string]any)
	gt.Bool(t, ok).True().Required()
	return res, items
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
	gt.Value(t, items[0]["title"]).Equal("one hour ago")
	gt.Value(t, items[1]["title"]).Equal("three days ago")

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
	gt.Value(t, afterItems[0]["title"]).Equal("one hour later")
	gt.Value(t, afterItems[1]["title"]).Equal("on boundary")

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
	gt.Value(t, items[0]["title"]).Equal("newer")
	gt.Value(t, items[1]["title"]).Equal("older")
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

func TestListMemosTool_DefaultLimit(t *testing.T) {
	list := listToolWithSeries(t, 25)

	res, items := runList(t, list, map[string]any{})
	gt.Array(t, items).Length(10).Required()
	gt.Value(t, memoTitles(items)).Equal(descTitles(24, 10))
	gt.Value(t, res["offset"]).Equal(0)
	gt.Value(t, res["total_count"]).Equal(25)
	gt.Value(t, res["returned_count"]).Equal(10)
	gt.Value(t, res["has_more"]).Equal(true)
}

func TestListMemosTool_ExplicitLimit(t *testing.T) {
	list := listToolWithSeries(t, 25)

	res, items := runList(t, list, map[string]any{"limit": 3})
	gt.Array(t, items).Length(3).Required()
	gt.Value(t, memoTitles(items)).Equal(descTitles(24, 3))
	gt.Value(t, res["total_count"]).Equal(25)
	gt.Value(t, res["returned_count"]).Equal(3)
	gt.Value(t, res["has_more"]).Equal(true)
}

func TestListMemosTool_Paging(t *testing.T) {
	list := listToolWithSeries(t, 25)

	first, firstItems := runList(t, list, map[string]any{})
	gt.Value(t, memoTitles(firstItems)).Equal(descTitles(24, 10))
	gt.Value(t, first["has_more"]).Equal(true)

	second, secondItems := runList(t, list, map[string]any{"offset": 10})
	gt.Array(t, secondItems).Length(10).Required()
	gt.Value(t, memoTitles(secondItems)).Equal(descTitles(14, 10))
	gt.Value(t, second["offset"]).Equal(10)
	gt.Value(t, second["total_count"]).Equal(25)
	gt.Value(t, second["has_more"]).Equal(true)

	// The tail page is shorter than the limit and closes the sequence.
	third, thirdItems := runList(t, list, map[string]any{"offset": 20})
	gt.Array(t, thirdItems).Length(5).Required()
	gt.Value(t, memoTitles(thirdItems)).Equal(descTitles(4, 5))
	gt.Value(t, third["offset"]).Equal(20)
	gt.Value(t, third["returned_count"]).Equal(5)
	gt.Value(t, third["has_more"]).Equal(false)

	// Consecutive pages must not overlap: the three pages cover all 25 ids once.
	seen := map[string]bool{}
	for _, id := range append(append(memoIDs(firstItems), memoIDs(secondItems)...), memoIDs(thirdItems)...) {
		gt.Bool(t, seen[id]).False()
		seen[id] = true
	}
	gt.Number(t, len(seen)).Equal(25)
}

func TestListMemosTool_NoMoreUnderLimit(t *testing.T) {
	list := listToolWithSeries(t, 3)

	res, items := runList(t, list, map[string]any{})
	gt.Array(t, items).Length(3).Required()
	gt.Value(t, memoTitles(items)).Equal(descTitles(2, 3))
	gt.Value(t, res["offset"]).Equal(0)
	gt.Value(t, res["total_count"]).Equal(3)
	gt.Value(t, res["returned_count"]).Equal(3)
	gt.Value(t, res["has_more"]).Equal(false)
}

func TestListMemosTool_LimitCappedAtMax(t *testing.T) {
	list := listToolWithSeries(t, 55)

	res, items := runList(t, list, map[string]any{"limit": 1000})
	gt.Array(t, items).Length(50).Required()
	gt.Value(t, memoTitles(items)).Equal(descTitles(54, 50))
	gt.Value(t, res["total_count"]).Equal(55)
	gt.Value(t, res["returned_count"]).Equal(50)
	gt.Value(t, res["has_more"]).Equal(true)
}

func TestListMemosTool_LimitBelowOneFallsBackToDefault(t *testing.T) {
	list := listToolWithSeries(t, 25)

	for _, limit := range []any{0, -5} {
		res, items := runList(t, list, map[string]any{"limit": limit})
		gt.Array(t, items).Length(10).Required()
		gt.Value(t, memoTitles(items)).Equal(descTitles(24, 10))
		gt.Value(t, res["has_more"]).Equal(true)
	}
}

func TestListMemosTool_LimitOffsetEmptyStringIsUnset(t *testing.T) {
	list := listToolWithSeries(t, 25)

	res, items := runList(t, list, map[string]any{"limit": "", "offset": ""})
	gt.Array(t, items).Length(10).Required()
	gt.Value(t, memoTitles(items)).Equal(descTitles(24, 10))
	gt.Value(t, res["offset"]).Equal(0)
}

func TestListMemosTool_LimitTruncatesFloat(t *testing.T) {
	list := listToolWithSeries(t, 25)

	_, items := runList(t, list, map[string]any{"limit": 3.7})
	gt.Array(t, items).Length(3).Required()
	gt.Value(t, memoTitles(items)).Equal(descTitles(24, 3))
}

func TestListMemosTool_NegativeOffsetIsZero(t *testing.T) {
	list := listToolWithSeries(t, 25)

	res, items := runList(t, list, map[string]any{"offset": -3})
	gt.Array(t, items).Length(10).Required()
	gt.Value(t, memoTitles(items)).Equal(descTitles(24, 10))
	gt.Value(t, res["offset"]).Equal(0)
}

func TestListMemosTool_OffsetBeyondTotal(t *testing.T) {
	list := listToolWithSeries(t, 5)

	res, items := runList(t, list, map[string]any{"offset": 100})
	gt.Array(t, items).Length(0)
	gt.Value(t, res["offset"]).Equal(5)
	gt.Value(t, res["total_count"]).Equal(5)
	gt.Value(t, res["returned_count"]).Equal(0)
	gt.Value(t, res["has_more"]).Equal(false)
}

// TestListMemosTool_OffsetOutOfRange pins the narrowing rule: an offset far past
// the end must yield an empty page, never wrap around to the first one. The
// float64 cases are the ones a model can actually produce — gollem decodes JSON
// numbers as float64 (see tool.ExtractInt64), and converting an out-of-range
// float64 to int64 is implementation-defined, so an unchecked conversion can
// land on a negative value and silently return page 1.
func TestListMemosTool_OffsetOutOfRange(t *testing.T) {
	list := listToolWithSeries(t, 5)

	for _, offset := range []any{int64(math.MaxInt64), float64(math.MaxInt64), 1e300} {
		res, items := runList(t, list, map[string]any{"offset": offset})
		gt.Array(t, items).Length(0)
		gt.Value(t, res["offset"]).Equal(5)
		gt.Value(t, res["total_count"]).Equal(5)
		gt.Value(t, res["has_more"]).Equal(false)
	}
}

// TestListMemosTool_LimitOutOfRange is the limit-side counterpart: an
// out-of-range request is capped at the maximum, not folded back to the default.
func TestListMemosTool_LimitOutOfRange(t *testing.T) {
	list := listToolWithSeries(t, 55)

	for _, limit := range []any{int64(math.MaxInt64), float64(math.MaxInt64), 1e300} {
		_, items := runList(t, list, map[string]any{"limit": limit})
		gt.Array(t, items).Length(50).Required()
		gt.Value(t, memoTitles(items)).Equal(descTitles(54, 50))
	}
}

func TestListMemosTool_RejectsInvalidLimit(t *testing.T) {
	list := listToolWithSeries(t, 3)
	ctx := context.Background()

	for _, limit := range []any{"many", true, []any{1}} {
		res, err := list.Run(ctx, map[string]any{"limit": limit})
		gt.Value(t, err).NotNil()
		gt.Value(t, res).Nil()
	}
}

func TestListMemosTool_RejectsInvalidOffset(t *testing.T) {
	list := listToolWithSeries(t, 3)

	res, err := list.Run(context.Background(), map[string]any{"offset": "second page"})
	gt.Value(t, err).NotNil()
	gt.Value(t, res).Nil()
}

func TestListMemosTool_EmptyResult(t *testing.T) {
	list := listToolWithMemos(t)

	res, items := runList(t, list, map[string]any{})
	gt.Array(t, items).Length(0)
	gt.Value(t, res["offset"]).Equal(0)
	gt.Value(t, res["total_count"]).Equal(0)
	gt.Value(t, res["returned_count"]).Equal(0)
	gt.Value(t, res["has_more"]).Equal(false)
}

func TestListMemosTool_TotalCountReflectsWindow(t *testing.T) {
	now := time.Now().UTC()
	list := listToolWithMemos(t,
		memoCreatedAt("twenty days ago", now.Add(-20*24*time.Hour)),
		memoCreatedAt("fifteen days ago", now.Add(-15*24*time.Hour)),
		memoCreatedAt("ten days ago", now.Add(-10*24*time.Hour)),
		memoCreatedAt("three days ago", now.Add(-3*24*time.Hour)),
		memoCreatedAt("one hour ago", now.Add(-time.Hour)),
	)

	res, items := runList(t, list, map[string]any{
		"created_after": now.Add(-7 * 24 * time.Hour).Format(time.RFC3339),
	})
	gt.Array(t, items).Length(2).Required()
	gt.Value(t, memoTitles(items)).Equal([]string{"one hour ago", "three days ago"})
	// total_count counts the window, not the whole case.
	gt.Value(t, res["total_count"]).Equal(2)
	gt.Value(t, res["returned_count"]).Equal(2)
	gt.Value(t, res["has_more"]).Equal(false)
}

// TestListMemosTool_ReturnsFullFieldValues pins the payload contract: paging
// bounds how many memos come back, not how much of each one. The list response
// carries every field value verbatim — no summary, no excerpt, no truncation.
func TestListMemosTool_ReturnsFullFieldValues(t *testing.T) {
	now := time.Now().UTC()
	longText := strings.Repeat("resolver logs show NXDOMAIN for the whole window. ", 40)
	m := memoCreatedAt("with fields", now)
	m.FieldValues = map[string]model.FieldValue{
		"memo_type": {Value: "fact"},
		"tags":      {Value: []string{"a", "b"}},
		"note":      {Value: longText},
	}
	list := listToolWithMemos(t, m)

	_, items := runList(t, list, map[string]any{})
	gt.Array(t, items).Length(1).Required()
	values, ok := items[0]["field_values"].(map[string]any)
	gt.Bool(t, ok).True().Required()
	gt.Value(t, values["memo_type"]).Equal("fact")
	gt.Value(t, values["tags"]).Equal([]string{"a", "b"})
	gt.Value(t, values["note"]).Equal(longText)
}

func TestListMemosTool_SpecAdvertisesPaging(t *testing.T) {
	spec := listToolWithMemos(t).Spec()

	gt.Map(t, spec.Parameters).HasKey("limit")
	gt.Map(t, spec.Parameters).HasKey("offset")
	gt.Value(t, spec.Parameters["limit"].Type).Equal(gollem.TypeInteger)
	gt.Value(t, spec.Parameters["offset"].Type).Equal(gollem.TypeInteger)
	gt.Bool(t, spec.Parameters["limit"].Required).False()
	gt.Bool(t, spec.Parameters["offset"].Required).False()

	// include_archived is gone: this tool never reaches archived memos.
	_, hasIncludeArchived := spec.Parameters["include_archived"]
	gt.Bool(t, hasIncludeArchived).False()
	gt.String(t, spec.Description).Contains("Archived memos are never returned")
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
