package notiontool_test

import (
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
)

// testSchema is a data source whose columns cover the type groups the operator
// catalog distinguishes. The properties are in name order, as GetDataSource
// returns them.
func testSchema() *notiontool.DataSource {
	return &notiontool.DataSource{
		ID:   "ds-1",
		Name: "Active",
		Properties: []notiontool.PropertySchema{
			{ID: "aikw", Name: "Keywords", Type: "multi_select", Options: []string{"network"}},
			{ID: "edit", Name: "Last edited", Type: "last_edited_time"},
			{ID: "title", Name: "Name", Type: "title"},
			{ID: "ownr", Name: "Owner", Type: "people"},
			{ID: "stat", Name: "Status", Type: "select", Options: []string{"Published"}},
			{ID: "sumr", Name: "Summary", Type: "rich_text"},
			{ID: "vrfy", Name: "Verified", Type: "verification"},
		},
	}
}

func TestResolveProperty(t *testing.T) {
	ds := testSchema()

	t.Run("finds a property by its exact name", func(t *testing.T) {
		got, err := notiontool.ResolvePropertyForTest(ds, "Summary")
		gt.NoError(t, err).Required()
		gt.String(t, got.ID).Equal("sumr")
		gt.String(t, got.Type).Equal("rich_text")
	})

	// The agent reads these names out of property_schema but also retypes them,
	// and a case-only difference is unambiguous when one property matches.
	t.Run("finds a property when only the case differs", func(t *testing.T) {
		got, err := notiontool.ResolvePropertyForTest(ds, "summary")
		gt.NoError(t, err).Required()
		gt.String(t, got.ID).Equal("sumr")
	})

	t.Run("rejects a name that matches several properties when case is ignored", func(t *testing.T) {
		ambiguous := &notiontool.DataSource{Properties: []notiontool.PropertySchema{
			{ID: "a", Name: "Owner", Type: "people"},
			{ID: "b", Name: "owner", Type: "rich_text"},
		}}

		_, err := notiontool.ResolvePropertyForTest(ambiguous, "OWNER")
		gt.Value(t, err).NotNil().Required()
		gt.Bool(t, notiontool.IsRejectionForTest(err)).True()
		gt.String(t, err.Error()).Contains("Owner")
		gt.String(t, err.Error()).Contains("owner")
		gt.String(t, err.Error()).Contains("exact name")
	})

	t.Run("rejects an unknown name and lists the alternatives", func(t *testing.T) {
		_, err := notiontool.ResolvePropertyForTest(ds, "Assignee")
		gt.Value(t, err).NotNil().Required()
		gt.Bool(t, notiontool.IsRejectionForTest(err)).True()
		gt.String(t, err.Error()).Contains("Assignee")
		gt.String(t, err.Error()).Contains("available properties")
		gt.String(t, err.Error()).Contains("Keywords")
		gt.String(t, err.Error()).Contains("Status")
	})

	t.Run("rejects an empty name", func(t *testing.T) {
		_, err := notiontool.ResolvePropertyForTest(ds, "")
		gt.Value(t, err).NotNil().Required()
		gt.Bool(t, notiontool.IsRejectionForTest(err)).True()
	})

	t.Run("says so when the data source has no properties at all", func(t *testing.T) {
		_, err := notiontool.ResolvePropertyForTest(&notiontool.DataSource{}, "Status")
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("no properties")
	})

	// A model that has to read 60 property names to find out its own mistake has
	// no context left for the answer.
	t.Run("caps how many names one rejection lists", func(t *testing.T) {
		wide := &notiontool.DataSource{}
		for i := 0; i < 40; i++ {
			wide.Properties = append(wide.Properties, notiontool.PropertySchema{
				ID:   string(rune('a' + i%26)),
				Name: "Column " + strings.Repeat("x", i+1),
				Type: "rich_text",
			})
		}

		_, err := notiontool.ResolvePropertyForTest(wide, "Missing")
		gt.Value(t, err).NotNil().Required()
		gt.Number(t, strings.Count(err.Error(), "Column ")).Equal(30)
		gt.String(t, err.Error()).Contains("and 10 more")
	})
}

func TestOperatorsFor(t *testing.T) {
	t.Run("gives a title property the text operators", func(t *testing.T) {
		gt.Array(t, notiontool.OperatorsForTest("title")).Equal([]string{
			"equals", "does_not_equal", "contains", "does_not_contain",
			"starts_with", "ends_with", "is_empty", "is_not_empty",
		})
	})

	t.Run("gives a select property the choice operators", func(t *testing.T) {
		gt.Array(t, notiontool.OperatorsForTest("select")).Equal([]string{
			"equals", "does_not_equal", "is_empty", "is_not_empty",
		})
	})

	// A multi_select holds several choices, so Notion matches it with contains
	// rather than equals — and that contains is an exact choice name, not a
	// substring.
	t.Run("gives a multi_select property the containment operators", func(t *testing.T) {
		gt.Array(t, notiontool.OperatorsForTest("multi_select")).Equal([]string{
			"contains", "does_not_contain", "is_empty", "is_not_empty",
		})
	})

	t.Run("gives a date property the relative operators", func(t *testing.T) {
		ops := notiontool.OperatorsForTest("date")
		for _, want := range []string{"on_or_before", "past_week", "this_week", "next_year", "is_empty"} {
			gt.Bool(t, containsOperator(ops, want)).True()
		}
	})

	// A created_time / last_edited_time property always holds a value.
	t.Run("gives a timestamp property no emptiness operators", func(t *testing.T) {
		for _, propType := range []string{"created_time", "last_edited_time"} {
			ops := notiontool.OperatorsForTest(propType)
			gt.Bool(t, containsOperator(ops, "before")).True()
			gt.Bool(t, containsOperator(ops, "is_empty")).False()
			gt.Bool(t, containsOperator(ops, "is_not_empty")).False()
		}
	})

	t.Run("gives created_by and last_edited_by only containment", func(t *testing.T) {
		for _, propType := range []string{"created_by", "last_edited_by"} {
			gt.Array(t, notiontool.OperatorsForTest(propType)).Equal([]string{"contains", "does_not_contain"})
		}
	})

	t.Run("gives a files property only emptiness", func(t *testing.T) {
		gt.Array(t, notiontool.OperatorsForTest("files")).Equal([]string{"is_empty", "is_not_empty"})
	})

	// Notion's schema carries a formula's expression and a rollup's aggregation,
	// never their result type, so which of these apply depends on the result
	// type the caller declares.
	t.Run("gives formula and rollup the union of the value-type operators", func(t *testing.T) {
		for _, propType := range []string{"formula", "rollup"} {
			ops := notiontool.OperatorsForTest(propType)
			for _, want := range []string{"contains", "greater_than", "before", "equals"} {
				gt.Bool(t, containsOperator(ops, want)).True()
			}
			// The union is deduplicated: equals belongs to four of the sets.
			gt.Number(t, countOperator(ops, "equals")).Equal(1)
		}
	})

	t.Run("gives an unfilterable type no operators", func(t *testing.T) {
		for _, propType := range []string{"verification", "place", ""} {
			gt.Array(t, notiontool.OperatorsForTest(propType)).Length(0)
		}
	})
}

func TestParseNameList(t *testing.T) {
	t.Run("reads an array of names", func(t *testing.T) {
		got, err := notiontool.ParseNameListForTest(map[string]any{
			"properties": []any{"Status", "Summary"},
		}, "properties", 5)
		gt.NoError(t, err).Required()
		gt.Array(t, got).Equal([]string{"Status", "Summary"})
	})

	t.Run("treats an absent or null argument as unset", func(t *testing.T) {
		for _, args := range []map[string]any{{}, {"properties": nil}} {
			got, err := notiontool.ParseNameListForTest(args, "properties", 5)
			gt.NoError(t, err).Required()
			gt.Array(t, got).Length(0)
		}
	})

	t.Run("drops blank entries", func(t *testing.T) {
		got, err := notiontool.ParseNameListForTest(map[string]any{
			"properties": []any{"Status", "  ", ""},
		}, "properties", 5)
		gt.NoError(t, err).Required()
		gt.Array(t, got).Equal([]string{"Status"})
	})

	t.Run("rejects a value that is not an array", func(t *testing.T) {
		_, err := notiontool.ParseNameListForTest(map[string]any{"properties": "Status"}, "properties", 5)
		gt.Value(t, err).NotNil().Required()
		gt.Bool(t, notiontool.IsRejectionForTest(err)).True()
		gt.String(t, err.Error()).Contains("must be an array")
	})

	t.Run("rejects an entry that is not text", func(t *testing.T) {
		_, err := notiontool.ParseNameListForTest(map[string]any{
			"properties": []any{"Status", float64(3)},
		}, "properties", 5)
		gt.Value(t, err).NotNil().Required()
		gt.Bool(t, notiontool.IsRejectionForTest(err)).True()
		gt.String(t, err.Error()).Contains("properties[1]")
	})

	t.Run("rejects more entries than the cap", func(t *testing.T) {
		_, err := notiontool.ParseNameListForTest(map[string]any{
			"properties": []any{"a", "b", "c"},
		}, "properties", 2)
		gt.Value(t, err).NotNil().Required()
		gt.Bool(t, notiontool.IsRejectionForTest(err)).True()
		gt.String(t, err.Error()).Contains("3")
		gt.String(t, err.Error()).Contains("2")
	})
}

func containsOperator(ops []string, want string) bool {
	return countOperator(ops, want) > 0
}

func countOperator(ops []string, want string) int {
	n := 0
	for _, op := range ops {
		if op == want {
			n++
		}
	}
	return n
}
