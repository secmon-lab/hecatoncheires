package memo_test

import (
	"bytes"
	"context"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	memotool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/memo"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

const testWS = "ws"
const testCaseID int64 = 7

const (
	testMemoIDA = "11111111-1111-7111-8111-111111111111"
	testMemoIDB = "22222222-2222-7222-8222-222222222222"
	testMemoIDC = "33333333-3333-7333-8333-333333333333"
)

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

type memoCreateCall struct {
	Title  string
	Fields map[string]model.FieldValue
}

type memoUpdateCall struct {
	ID     model.MemoID
	Title  *string
	Fields map[string]model.FieldValue
}

// fakeMutator records every call in the order it was made and can be told to
// fail a specific operation via errOn, so a batch's partial-failure behaviour
// is observable. A failed call is still recorded, which is what lets a test
// assert that later entries were attempted after an earlier one failed.
type fakeMutator struct {
	creates  []memoCreateCall
	updates  []memoUpdateCall
	archives []model.MemoID
	order    []string

	// errOn returns a non-nil error to fail the (kind, index) call, where
	// index is the 0-based position within that kind.
	errOn func(kind string, index int) error
}

func (f *fakeMutator) fail(kind string, index int) error {
	if f.errOn == nil {
		return nil
	}
	return f.errOn(kind, index)
}

func (f *fakeMutator) CreateMemo(_ context.Context, _ string, caseID int64, title string, fields map[string]model.FieldValue) (*model.Memo, error) {
	index := len(f.creates)
	f.creates = append(f.creates, memoCreateCall{Title: title, Fields: fields})
	f.order = append(f.order, "create")
	if err := f.fail("create", index); err != nil {
		return nil, err
	}
	return &model.Memo{ID: model.NewMemoID(), CaseID: caseID, Title: title, FieldValues: fields}, nil
}

func (f *fakeMutator) UpdateMemo(_ context.Context, _ string, caseID int64, id model.MemoID, title *string, fields map[string]model.FieldValue) (*model.Memo, error) {
	index := len(f.updates)
	f.updates = append(f.updates, memoUpdateCall{ID: id, Title: title, Fields: fields})
	f.order = append(f.order, "update")
	if err := f.fail("update", index); err != nil {
		return nil, err
	}
	return &model.Memo{ID: id, CaseID: caseID, Title: deref(title)}, nil
}

func (f *fakeMutator) ArchiveMemo(_ context.Context, _ string, caseID int64, id model.MemoID) (*model.Memo, error) {
	index := len(f.archives)
	f.archives = append(f.archives, id)
	f.order = append(f.order, "archive")
	if err := f.fail("archive", index); err != nil {
		return nil, err
	}
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
	gt.Bool(t, false).True().Required()
	return nil
}

func toolNames(tools []gollem.Tool) map[string]bool {
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Spec().Name] = true
	}
	return names
}

// applyTool builds the full memo tool set and returns the batch write tool.
func applyTool(t *testing.T, fake *fakeMutator, schema *config.FieldSchema) gollem.Tool {
	t.Helper()
	tools := memotool.New(memotool.Deps{
		Repo: memory.New(), WorkspaceID: testWS, CaseID: testCaseID, MemoUC: fake, Schema: schema,
	})
	return findTool(t, tools, "memo__apply_memo_changes")
}

func resultsOf(t *testing.T, res map[string]any) []map[string]any {
	t.Helper()
	return gt.Cast[[]map[string]any](t, res["results"])
}

func memoOf(t *testing.T, item map[string]any) map[string]any {
	t.Helper()
	return gt.Cast[map[string]any](t, item["memo"])
}

func fieldEntry(fieldID, value string) map[string]any {
	return map[string]any{"field_id": fieldID, "value": value}
}

func TestNew_ExposesReadToolsAndTheBatchWriteTool(t *testing.T) {
	tools := memotool.New(memotool.Deps{
		Repo: memory.New(), WorkspaceID: testWS, CaseID: testCaseID,
		MemoUC: &fakeMutator{}, Schema: memoSchema(),
	})
	names := toolNames(tools)

	gt.Array(t, tools).Length(3)
	gt.Bool(t, names["memo__list_memos"]).True()
	gt.Bool(t, names["memo__get_memo"]).True()
	gt.Bool(t, names["memo__apply_memo_changes"]).True()

	// The single-item write tools are gone: the model must not see two ways
	// to write a memo.
	gt.Bool(t, names["memo__create_memo"]).False()
	gt.Bool(t, names["memo__update_memo"]).False()
	gt.Bool(t, names["memo__archive_memo"]).False()
}

func TestApplyMemoChangesTool_AppliesAllKinds(t *testing.T) {
	fake := &fakeMutator{}
	apply := applyTool(t, fake, memoSchema())

	res, err := apply.Run(context.Background(), map[string]any{
		"creates": []any{
			map[string]any{
				"title": "first fact",
				"fields": []any{
					fieldEntry("memo_type", "fact"),
					map[string]any{"field_id": "tags", "values": []any{"a", "b"}},
				},
			},
			map[string]any{
				"title":  "second fact",
				"fields": []any{fieldEntry("memo_type", "hypothesis")},
			},
		},
		"updates": []any{
			map[string]any{
				"memo_id": testMemoIDA,
				"title":   "revised title",
				"fields":  []any{fieldEntry("memo_type", "fact")},
			},
		},
		"archives": []any{testMemoIDB},
	})
	gt.NoError(t, err).Required()

	// Applied in order: every create, then every update, then every archive.
	gt.Value(t, fake.order).Equal([]string{"create", "create", "update", "archive"})

	gt.Array(t, fake.creates).Length(2).Required()
	gt.String(t, fake.creates[0].Title).Equal("first fact")
	gt.Value(t, fake.creates[0].Fields["memo_type"].Value).Equal("fact")
	gt.Value(t, fake.creates[0].Fields["tags"].Value).Equal([]string{"a", "b"})
	gt.String(t, fake.creates[1].Title).Equal("second fact")
	gt.Value(t, fake.creates[1].Fields["memo_type"].Value).Equal("hypothesis")

	gt.Array(t, fake.updates).Length(1).Required()
	gt.Value(t, fake.updates[0].ID).Equal(model.MemoID(testMemoIDA))
	gt.String(t, deref(fake.updates[0].Title)).Equal("revised title")
	gt.Value(t, fake.updates[0].Fields["memo_type"].Value).Equal("fact")

	gt.Array(t, fake.archives).Length(1).Required()
	gt.Value(t, fake.archives[0]).Equal(model.MemoID(testMemoIDB))

	gt.Value(t, res["applied"]).Equal(4)
	gt.Value(t, res["failed"]).Equal(0)

	items := resultsOf(t, res)
	gt.Array(t, items).Length(4).Required()

	gt.Value(t, items[0]["op"]).Equal("create")
	gt.Value(t, items[0]["index"]).Equal(0)
	gt.Value(t, items[0]["ok"]).Equal(true)
	gt.Value(t, memoOf(t, items[0])["title"]).Equal("first fact")

	gt.Value(t, items[1]["op"]).Equal("create")
	gt.Value(t, items[1]["index"]).Equal(1)
	gt.Value(t, items[1]["ok"]).Equal(true)
	gt.Value(t, memoOf(t, items[1])["title"]).Equal("second fact")

	gt.Value(t, items[2]["op"]).Equal("update")
	gt.Value(t, items[2]["index"]).Equal(0)
	gt.Value(t, items[2]["ok"]).Equal(true)
	gt.Value(t, memoOf(t, items[2])["id"]).Equal(testMemoIDA)

	gt.Value(t, items[3]["op"]).Equal("archive")
	gt.Value(t, items[3]["index"]).Equal(0)
	gt.Value(t, items[3]["ok"]).Equal(true)
	gt.Value(t, memoOf(t, items[3])["id"]).Equal(testMemoIDB)
	gt.Value(t, memoOf(t, items[3])["archived"]).Equal(true)
}

func TestApplyMemoChangesTool_OnlyArchives(t *testing.T) {
	fake := &fakeMutator{}
	apply := applyTool(t, fake, memoSchema())

	res, err := apply.Run(context.Background(), map[string]any{
		"archives": []any{testMemoIDA, testMemoIDB},
	})
	gt.NoError(t, err).Required()

	gt.Value(t, fake.order).Equal([]string{"archive", "archive"})
	gt.Array(t, fake.creates).Length(0)
	gt.Array(t, fake.updates).Length(0)
	gt.Array(t, fake.archives).Length(2).Required()
	gt.Value(t, fake.archives[0]).Equal(model.MemoID(testMemoIDA))
	gt.Value(t, fake.archives[1]).Equal(model.MemoID(testMemoIDB))

	gt.Value(t, res["applied"]).Equal(2)
	gt.Value(t, res["failed"]).Equal(0)
}

func TestApplyMemoChangesTool_UpdateTitleOnlyAndFieldsOnly(t *testing.T) {
	fake := &fakeMutator{}
	apply := applyTool(t, fake, memoSchema())

	res, err := apply.Run(context.Background(), map[string]any{
		"updates": []any{
			map[string]any{"memo_id": testMemoIDA, "title": "title only"},
			map[string]any{"memo_id": testMemoIDB, "fields": []any{fieldEntry("memo_type", "fact")}},
		},
	})
	gt.NoError(t, err).Required()
	gt.Value(t, res["applied"]).Equal(2)
	gt.Value(t, res["failed"]).Equal(0)

	gt.Array(t, fake.updates).Length(2).Required()

	// Title only: fields stay nil so the usecase preserves every stored value.
	gt.Value(t, fake.updates[0].ID).Equal(model.MemoID(testMemoIDA))
	gt.String(t, deref(fake.updates[0].Title)).Equal("title only")
	gt.Value(t, fake.updates[0].Fields).Nil()

	// Fields only: title stays nil so the usecase preserves the stored title.
	gt.Value(t, fake.updates[1].ID).Equal(model.MemoID(testMemoIDB))
	gt.Value(t, fake.updates[1].Title).Nil()
	gt.Value(t, fake.updates[1].Fields["memo_type"].Value).Equal("fact")
}

func TestApplyMemoChangesTool_UpdateThenArchiveSameID(t *testing.T) {
	fake := &fakeMutator{}
	apply := applyTool(t, fake, memoSchema())

	res, err := apply.Run(context.Background(), map[string]any{
		"updates":  []any{map[string]any{"memo_id": testMemoIDA, "title": "final wording"}},
		"archives": []any{testMemoIDA},
	})
	gt.NoError(t, err).Required()

	// The update lands before the archive, which is what "revise, then retire"
	// requires.
	gt.Value(t, fake.order).Equal([]string{"update", "archive"})
	gt.Array(t, fake.updates).Length(1).Required()
	gt.Value(t, fake.updates[0].ID).Equal(model.MemoID(testMemoIDA))
	gt.Array(t, fake.archives).Length(1).Required()
	gt.Value(t, fake.archives[0]).Equal(model.MemoID(testMemoIDA))

	gt.Value(t, res["applied"]).Equal(2)
	gt.Value(t, res["failed"]).Equal(0)
}

func TestApplyMemoChangesTool_ContinuesAfterItemFailure(t *testing.T) {
	fake := &fakeMutator{errOn: func(kind string, index int) error {
		if kind == "create" && index == 1 {
			return goerr.New("second create rejected")
		}
		return nil
	}}
	apply := applyTool(t, fake, memoSchema())

	res, err := apply.Run(context.Background(), map[string]any{
		"creates": []any{
			map[string]any{"title": "one", "fields": []any{fieldEntry("memo_type", "fact")}},
			map[string]any{"title": "two", "fields": []any{fieldEntry("memo_type", "fact")}},
			map[string]any{"title": "three", "fields": []any{fieldEntry("memo_type", "fact")}},
		},
	})
	// The tool call itself succeeds; the failure is reported per item.
	gt.NoError(t, err).Required()

	// Every entry was attempted, including the ones after the failure.
	gt.Array(t, fake.creates).Length(3).Required()
	gt.String(t, fake.creates[2].Title).Equal("three")

	gt.Value(t, res["applied"]).Equal(2)
	gt.Value(t, res["failed"]).Equal(1)

	items := resultsOf(t, res)
	gt.Array(t, items).Length(3).Required()
	gt.Value(t, items[0]["ok"]).Equal(true)
	gt.Value(t, items[1]["ok"]).Equal(false)
	gt.String(t, gt.Cast[string](t, items[1]["error"])).Contains("second create rejected")
	gt.Value(t, items[1]["index"]).Equal(1)
	gt.Value(t, items[2]["ok"]).Equal(true)
	gt.Value(t, memoOf(t, items[2])["title"]).Equal("three")
}

func TestApplyMemoChangesTool_AllItemsFailStillReturnsResults(t *testing.T) {
	fake := &fakeMutator{errOn: func(_ string, _ int) error {
		return goerr.New("backend unavailable")
	}}
	apply := applyTool(t, fake, memoSchema())

	res, err := apply.Run(context.Background(), map[string]any{
		"creates":  []any{map[string]any{"title": "one", "fields": []any{fieldEntry("memo_type", "fact")}}},
		"archives": []any{testMemoIDA},
	})
	gt.NoError(t, err).Required()

	gt.Value(t, res["applied"]).Equal(0)
	gt.Value(t, res["failed"]).Equal(2)

	items := resultsOf(t, res)
	gt.Array(t, items).Length(2).Required()
	gt.Value(t, items[0]["ok"]).Equal(false)
	gt.String(t, gt.Cast[string](t, items[0]["error"])).Contains("backend unavailable")
	gt.Value(t, items[1]["ok"]).Equal(false)
	gt.String(t, gt.Cast[string](t, items[1]["error"])).Contains("backend unavailable")
}

func TestApplyMemoChangesTool_ParseErrorsArePerItem(t *testing.T) {
	fake := &fakeMutator{}
	apply := applyTool(t, fake, memoSchema())

	res, err := apply.Run(context.Background(), map[string]any{
		"creates": []any{
			map[string]any{"title": "good one", "fields": []any{fieldEntry("memo_type", "fact")}},
			map[string]any{"fields": []any{fieldEntry("memo_type", "fact")}}, // no title
			// A null entry. gollem's pre-Run ValidateArgs lets a null array item
			// through, so reporting it is this tool's job. A wrong-TYPED entry
			// never gets here — gollem rejects the whole call first. See
			// TestApplyMemoChangesTool_ValidateArgsBoundary.
			nil,
		},
		"updates": []any{
			map[string]any{"title": "no id here"},  // no memo_id
			map[string]any{"memo_id": testMemoIDA}, // neither title nor fields
			// field entry without a field_id
			map[string]any{"memo_id": testMemoIDB, "fields": []any{map[string]any{"value": "fact"}}},
		},
		"archives": []any{"", testMemoIDC},
	})
	gt.NoError(t, err).Required()

	// Only the two well-formed entries reached the mutator.
	gt.Value(t, fake.order).Equal([]string{"create", "archive"})
	gt.Array(t, fake.creates).Length(1).Required()
	gt.String(t, fake.creates[0].Title).Equal("good one")
	gt.Array(t, fake.updates).Length(0)
	gt.Array(t, fake.archives).Length(1).Required()
	gt.Value(t, fake.archives[0]).Equal(model.MemoID(testMemoIDC))

	gt.Value(t, res["applied"]).Equal(2)
	gt.Value(t, res["failed"]).Equal(6)

	items := resultsOf(t, res)
	gt.Array(t, items).Length(8).Required()

	gt.Value(t, items[0]["ok"]).Equal(true)

	gt.Value(t, items[1]["op"]).Equal("create")
	gt.Value(t, items[1]["ok"]).Equal(false)
	gt.String(t, gt.Cast[string](t, items[1]["error"])).Contains("title is required")

	gt.Value(t, items[2]["op"]).Equal("create")
	gt.Value(t, items[2]["ok"]).Equal(false)
	gt.String(t, gt.Cast[string](t, items[2]["error"])).Contains("must be an object")

	gt.Value(t, items[3]["op"]).Equal("update")
	gt.Value(t, items[3]["ok"]).Equal(false)
	gt.String(t, gt.Cast[string](t, items[3]["error"])).Contains("memo_id is required")

	gt.Value(t, items[4]["op"]).Equal("update")
	gt.Value(t, items[4]["ok"]).Equal(false)
	gt.String(t, gt.Cast[string](t, items[4]["error"])).Contains("at least one of title, fields")

	gt.Value(t, items[5]["op"]).Equal("update")
	gt.Value(t, items[5]["ok"]).Equal(false)
	gt.String(t, gt.Cast[string](t, items[5]["error"])).Contains("non-empty field_id")

	gt.Value(t, items[6]["op"]).Equal("archive")
	gt.Value(t, items[6]["index"]).Equal(0)
	gt.Value(t, items[6]["ok"]).Equal(false)
	gt.String(t, gt.Cast[string](t, items[6]["error"])).Contains("non-empty memo id")

	gt.Value(t, items[7]["op"]).Equal("archive")
	gt.Value(t, items[7]["index"]).Equal(1)
	gt.Value(t, items[7]["ok"]).Equal(true)
}

// TestApplyMemoChangesTool_UnknownFieldIDReachesTheUsecase pins where the
// "field is not defined in the workspace schema" rejection lives: coercion
// passes an unknown field id through with its type unset
// (model.CoerceFieldInputs), and MemoUseCase's field validator is what rejects
// it, so the message stays identical to the WebUI path. The tool must not
// duplicate that check.
func TestApplyMemoChangesTool_UnknownFieldIDReachesTheUsecase(t *testing.T) {
	fake := &fakeMutator{}
	apply := applyTool(t, fake, memoSchema())

	res, err := apply.Run(context.Background(), map[string]any{
		"creates": []any{map[string]any{
			"title":  "carries an unknown field",
			"fields": []any{fieldEntry("not_in_schema", "v")},
		}},
	})
	gt.NoError(t, err).Required()
	gt.Value(t, res["applied"]).Equal(1)
	gt.Value(t, res["failed"]).Equal(0)
	gt.Array(t, fake.creates).Length(1).Required()
	gt.Value(t, fake.creates[0].Fields["not_in_schema"].Value).Equal("v")
}

func TestApplyMemoChangesTool_FieldsWithoutSchemaFailPerItem(t *testing.T) {
	fake := &fakeMutator{}
	apply := applyTool(t, fake, nil)

	res, err := apply.Run(context.Background(), map[string]any{
		"creates":  []any{map[string]any{"title": "x", "fields": []any{fieldEntry("memo_type", "fact")}}},
		"archives": []any{testMemoIDA},
	})
	gt.NoError(t, err).Required()

	// The schema-less create fails on its own; the archive still lands.
	gt.Value(t, res["applied"]).Equal(1)
	gt.Value(t, res["failed"]).Equal(1)
	gt.Array(t, fake.creates).Length(0)
	gt.Array(t, fake.archives).Length(1).Required()

	items := resultsOf(t, res)
	gt.Array(t, items).Length(2).Required()
	gt.Value(t, items[0]["ok"]).Equal(false)
	gt.String(t, gt.Cast[string](t, items[0]["error"])).Contains("no memo fields")
	gt.Value(t, items[1]["ok"]).Equal(true)
}

func TestApplyMemoChangesTool_WholeCallErrors(t *testing.T) {
	validCreate := map[string]any{"title": "x", "fields": []any{fieldEntry("memo_type", "fact")}}

	t.Run("no arrays at all", func(t *testing.T) {
		fake := &fakeMutator{}
		apply := applyTool(t, fake, memoSchema())
		res, err := apply.Run(context.Background(), map[string]any{})
		gt.Value(t, err).NotNil()
		gt.Value(t, res).Nil()
		gt.Array(t, fake.order).Length(0)
	})

	t.Run("every array empty", func(t *testing.T) {
		fake := &fakeMutator{}
		apply := applyTool(t, fake, memoSchema())
		res, err := apply.Run(context.Background(), map[string]any{
			"creates": []any{}, "updates": []any{}, "archives": []any{},
		})
		gt.Value(t, err).NotNil()
		gt.Value(t, res).Nil()
		gt.Array(t, fake.order).Length(0)
	})

	t.Run("creates is not an array", func(t *testing.T) {
		fake := &fakeMutator{}
		apply := applyTool(t, fake, memoSchema())
		res, err := apply.Run(context.Background(), map[string]any{"creates": "nope"})
		gt.Value(t, err).NotNil()
		gt.Value(t, res).Nil()
		gt.Array(t, fake.order).Length(0)
	})

	t.Run("archives is not an array", func(t *testing.T) {
		fake := &fakeMutator{}
		apply := applyTool(t, fake, memoSchema())
		res, err := apply.Run(context.Background(), map[string]any{"archives": testMemoIDA})
		gt.Value(t, err).NotNil()
		gt.Value(t, res).Nil()
		gt.Array(t, fake.order).Length(0)
	})

	t.Run("over the per-call limit", func(t *testing.T) {
		fake := &fakeMutator{}
		apply := applyTool(t, fake, memoSchema())
		archives := make([]any, 0, 51)
		for i := range 51 {
			archives = append(archives, "id-"+strconv.Itoa(i))
		}
		res, err := apply.Run(context.Background(), map[string]any{"archives": archives})
		gt.Value(t, err).NotNil()
		gt.Value(t, res).Nil()
		gt.Array(t, fake.order).Length(0)
	})

	t.Run("exactly at the per-call limit", func(t *testing.T) {
		fake := &fakeMutator{}
		apply := applyTool(t, fake, memoSchema())
		archives := make([]any, 0, 50)
		for i := range 50 {
			archives = append(archives, "id-"+strconv.Itoa(i))
		}
		res, err := apply.Run(context.Background(), map[string]any{"archives": archives})
		gt.NoError(t, err).Required()
		gt.Value(t, res["applied"]).Equal(50)
		gt.Value(t, res["failed"]).Equal(0)
		gt.Array(t, fake.archives).Length(50)
	})

	t.Run("MemoUC not configured", func(t *testing.T) {
		tools := memotool.New(memotool.Deps{
			Repo: memory.New(), WorkspaceID: testWS, CaseID: testCaseID, MemoUC: nil, Schema: memoSchema(),
		})
		apply := findTool(t, tools, "memo__apply_memo_changes")
		res, err := apply.Run(context.Background(), map[string]any{"creates": []any{validCreate}})
		gt.Value(t, err).NotNil()
		gt.Value(t, res).Nil()
	})
}

// TestApplyMemoChangesTool_ValidateArgsBoundary pins which malformed inputs this
// tool reports per entry and which ones never reach it at all.
//
// gollem runs ToolSpec.ValidateArgs before Run and rejects the WHOLE call on the
// first violation, recursing into array items and object properties. A MISSING
// value is not a violation while the property is not Required, so enforcing
// requiredness stays this tool's job and a missing value costs only its own
// entry. A wrong TYPE is a violation, so a mistyped entry costs the whole batch
// and the model has to re-send it. That asymmetry is the real contract; the
// tool's own "must be an object" branch only ever sees a null entry.
func TestApplyMemoChangesTool_ValidateArgsBoundary(t *testing.T) {
	spec := applyTool(t, &fakeMutator{}, memoSchema()).Spec()

	t.Run("missing values reach Run and fail per entry", func(t *testing.T) {
		args := map[string]any{
			"creates":  []any{map[string]any{"fields": []any{map[string]any{"value": "fact"}}}},
			"updates":  []any{map[string]any{"title": "no id"}},
			"archives": []any{""},
		}
		gt.NoError(t, spec.ValidateArgs(args)).Required()

		fake := &fakeMutator{}
		res, err := applyTool(t, fake, memoSchema()).Run(context.Background(), args)
		gt.NoError(t, err).Required()
		gt.Value(t, res["applied"]).Equal(0)
		gt.Value(t, res["failed"]).Equal(3)
		gt.Array(t, fake.order).Length(0)
	})

	t.Run("a null entry reaches Run", func(t *testing.T) {
		args := map[string]any{"creates": []any{nil}}
		gt.NoError(t, spec.ValidateArgs(args)).Required()

		fake := &fakeMutator{}
		res, err := applyTool(t, fake, memoSchema()).Run(context.Background(), args)
		gt.NoError(t, err).Required()
		gt.Value(t, res["failed"]).Equal(1)
		gt.Array(t, fake.order).Length(0)
		items := resultsOf(t, res)
		gt.Array(t, items).Length(1).Required()
		gt.String(t, gt.Cast[string](t, items[0]["error"])).Contains("must be an object")
	})

	t.Run("a wrong-typed entry is rejected before Run", func(t *testing.T) {
		gt.Value(t, spec.ValidateArgs(map[string]any{"creates": []any{"not an object"}})).NotNil()
		gt.Value(t, spec.ValidateArgs(map[string]any{"creates": []any{map[string]any{"title": 123}}})).NotNil()
		gt.Value(t, spec.ValidateArgs(map[string]any{"updates": []any{map[string]any{"memo_id": 1}}})).NotNil()
		gt.Value(t, spec.ValidateArgs(map[string]any{"archives": []any{42}})).NotNil()
		gt.Value(t, spec.ValidateArgs(map[string]any{"creates": "nope"})).NotNil()
	})
}

// TestApplyMemoChangesTool_ReportsOnlyWriteFailures pins how the two kinds of
// per-entry failure are escalated. A write failure is the operator's problem and
// goes through errutil.Handle; a malformed argument is the model's own, is
// repairable by re-emitting the call, and must not reach the operator's error
// reporting.
func TestApplyMemoChangesTool_ReportsOnlyWriteFailures(t *testing.T) {
	ctxWithLog := func(buf *bytes.Buffer) context.Context {
		return logging.With(context.Background(),
			slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}

	t.Run("parse failures are not escalated", func(t *testing.T) {
		var buf bytes.Buffer
		fake := &fakeMutator{}
		res, err := applyTool(t, fake, memoSchema()).Run(ctxWithLog(&buf), map[string]any{
			"creates":  []any{map[string]any{"fields": []any{fieldEntry("memo_type", "fact")}}},
			"archives": []any{""},
		})
		gt.NoError(t, err).Required()
		gt.Value(t, res["failed"]).Equal(2)
		gt.Array(t, fake.order).Length(0)
		gt.String(t, buf.String()).Equal("")
	})

	t.Run("write failures are escalated with their context", func(t *testing.T) {
		var buf bytes.Buffer
		fake := &fakeMutator{errOn: func(_ string, _ int) error {
			return goerr.New("backend unavailable")
		}}
		res, err := applyTool(t, fake, memoSchema()).Run(ctxWithLog(&buf), map[string]any{
			"creates":  []any{map[string]any{"title": "one", "fields": []any{fieldEntry("memo_type", "fact")}}},
			"archives": []any{testMemoIDA},
		})
		gt.NoError(t, err).Required()
		gt.Value(t, res["failed"]).Equal(2)

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		gt.Array(t, lines).Length(2).Required()
		for _, line := range lines {
			gt.String(t, line).Contains("memo apply item failed")
			gt.String(t, line).Contains("backend unavailable")
			gt.String(t, line).Contains(testWS)
			gt.String(t, line).Contains("case_id")
			gt.String(t, line).Contains("index")
		}
		gt.String(t, lines[0]).Contains(`"op":"create"`)
		gt.String(t, lines[1]).Contains(`"op":"archive"`)
	})
}

// TestApplyMemoChangesTool_SpecHasNoNestedRequired pins the reason the batch
// tool can report failures per item at all: gollem runs ToolSpec.ValidateArgs
// before Run and rejects the WHOLE call on the first nested Required violation,
// which would discard every other entry in the batch. Requiredness is enforced
// while parsing each entry instead.
func TestApplyMemoChangesTool_SpecHasNoNestedRequired(t *testing.T) {
	apply := applyTool(t, &fakeMutator{}, memoSchema())
	spec := apply.Spec()

	gt.Map(t, spec.Parameters).HasKey("creates")
	gt.Map(t, spec.Parameters).HasKey("updates")
	gt.Map(t, spec.Parameters).HasKey("archives")

	for name, p := range spec.Parameters {
		gt.Bool(t, p.Required).False()
		gt.Value(t, p.Type).Equal(gollem.TypeArray)
		gt.Value(t, p.Items).NotNil()
		gt.Bool(t, p.Items.Required).False()
		if name == "archives" {
			gt.Value(t, p.Items.Type).Equal(gollem.TypeString)
			continue
		}
		gt.Value(t, p.Items.Type).Equal(gollem.TypeObject)
		for _, prop := range p.Items.Properties {
			gt.Bool(t, prop.Required).False()
		}
		fields := p.Items.Properties["fields"]
		gt.Value(t, fields).NotNil()
		gt.Bool(t, fields.Items.Properties["field_id"].Required).False()
	}

	gt.Map(t, spec.Parameters["creates"].Items.Properties).HasKey("title")
	gt.Map(t, spec.Parameters["updates"].Items.Properties).HasKey("memo_id")

	// An entry missing its required value survives gollem's pre-Run validation
	// and reaches Run, where it becomes a per-item failure.
	gt.NoError(t, spec.ValidateArgs(map[string]any{
		"creates": []any{map[string]any{"fields": []any{map[string]any{"value": "fact"}}}},
		"updates": []any{map[string]any{"title": "no id"}},
	}))
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
	names := toolNames(tools)
	gt.Bool(t, names["memo__list_memos"]).True()
	gt.Bool(t, names["memo__get_memo"]).True()
	gt.Bool(t, names["memo__apply_memo_changes"]).False()
	_ = interfaces.MemoArchiveScopeActiveOnly
}
