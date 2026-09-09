package notiontool_test

import (
	"encoding/json"
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

// asJSON renders a built filter so a test can pin the exact object Notion
// receives, including which key each condition took.
func asJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	gt.NoError(t, err).Required()
	return string(raw)
}

func TestBuildFilterFromKeywords(t *testing.T) {
	ds := testSchema()

	build := func(t *testing.T, args map[string]any) map[string]any {
		t.Helper()
		got, err := notiontool.BuildFilterForTest(ds, args)
		gt.NoError(t, err).Required()
		return got
	}

	t.Run("matches one keyword against the title by default", func(t *testing.T) {
		got := build(t, map[string]any{"query": "outage"})
		// A title column is filtered under the rich_text key: Notion has no
		// "title" condition key.
		gt.String(t, asJSON(t, got)).Equal(`{"property":"title","rich_text":{"contains":"outage"}}`)
	})

	t.Run("requires every keyword of a multi-word query", func(t *testing.T) {
		got := build(t, map[string]any{"query": "network outage"})
		gt.String(t, asJSON(t, got)).Equal(
			`{"and":[{"property":"title","rich_text":{"contains":"network"}},` +
				`{"property":"title","rich_text":{"contains":"outage"}}]}`)
	})

	// Notion cannot search page bodies, so a term that is only in a summary or a
	// keyword column is reachable only by naming those columns.
	t.Run("matches each keyword against any of the named properties", func(t *testing.T) {
		got := build(t, map[string]any{
			"query":             "network outage",
			"search_properties": []any{"Name", "Summary", "Keywords", "Status"},
		})
		gt.String(t, asJSON(t, got)).Equal(
			`{"and":[` +
				`{"or":[{"property":"title","rich_text":{"contains":"network"}},` +
				`{"property":"sumr","rich_text":{"contains":"network"}},` +
				`{"multi_select":{"contains":"network"},"property":"aikw"},` +
				`{"property":"stat","select":{"equals":"network"}}]},` +
				`{"or":[{"property":"title","rich_text":{"contains":"outage"}},` +
				`{"property":"sumr","rich_text":{"contains":"outage"}},` +
				`{"multi_select":{"contains":"outage"},"property":"aikw"},` +
				`{"property":"stat","select":{"equals":"outage"}}]}]}`)
	})

	t.Run("splits on ideographic and repeated whitespace", func(t *testing.T) {
		got := build(t, map[string]any{"query": "  network　 outage "})
		gt.String(t, asJSON(t, got)).Equal(
			`{"and":[{"property":"title","rich_text":{"contains":"network"}},` +
				`{"property":"title","rich_text":{"contains":"outage"}}]}`)
	})

	t.Run("builds nothing from a blank query", func(t *testing.T) {
		for _, query := range []any{"", "   ", nil} {
			got, err := notiontool.BuildFilterForTest(ds, map[string]any{"query": query})
			gt.NoError(t, err).Required()
			gt.Value(t, got).Nil()
		}
	})

	t.Run("builds nothing when neither keywords nor conditions are given", func(t *testing.T) {
		got, err := notiontool.BuildFilterForTest(ds, map[string]any{})
		gt.NoError(t, err).Required()
		gt.Value(t, got).Nil()
	})

	t.Run("rejects a keyword target that cannot be matched by keyword", func(t *testing.T) {
		_, err := notiontool.BuildFilterForTest(ds, map[string]any{
			"query":             "alice",
			"search_properties": []any{"Owner"},
		})
		gt.Value(t, err).NotNil().Required()
		gt.Bool(t, notiontool.IsRejectionForTest(err)).True()
		gt.String(t, err.Error()).Contains("Owner")
		gt.String(t, err.Error()).Contains("people")
		gt.String(t, err.Error()).Contains("search_properties")
	})

	t.Run("rejects a keyword search on a data source with no title column", func(t *testing.T) {
		titleless := &notiontool.DataSource{Properties: []notiontool.PropertySchema{
			{ID: "sumr", Name: "Summary", Type: "rich_text"},
		}}
		_, err := notiontool.BuildFilterForTest(titleless, map[string]any{"query": "outage"})
		gt.Value(t, err).NotNil().Required()
		gt.Bool(t, notiontool.IsRejectionForTest(err)).True()
		gt.String(t, err.Error()).Contains("search_properties")
	})

	t.Run("rejects more keywords than the cap", func(t *testing.T) {
		_, err := notiontool.BuildFilterForTest(ds, map[string]any{"query": "a b c d e f"})
		gt.Value(t, err).NotNil().Required()
		gt.Bool(t, notiontool.IsRejectionForTest(err)).True()
		gt.String(t, err.Error()).Contains("6")
		gt.String(t, err.Error()).Contains("5")
	})

	t.Run("rejects a query that is not text", func(t *testing.T) {
		_, err := notiontool.BuildFilterForTest(ds, map[string]any{"query": []any{"outage"}})
		gt.Value(t, err).NotNil().Required()
		gt.Bool(t, notiontool.IsRejectionForTest(err)).True()
	})
}

func TestBuildFilterFromConditions(t *testing.T) {
	ds := &notiontool.DataSource{
		ID: "ds-1",
		Properties: []notiontool.PropertySchema{
			{ID: "cby", Name: "Created by", Type: "created_by"},
			{ID: "crt", Name: "Created on", Type: "created_time"},
			{ID: "due", Name: "Due", Type: "date"},
			{ID: "att", Name: "Files", Type: "files"},
			{ID: "aikw", Name: "Keywords", Type: "multi_select"},
			{ID: "edit", Name: "Last edited", Type: "last_edited_time"},
			{ID: "title", Name: "Name", Type: "title"},
			{ID: "ownr", Name: "Owner", Type: "people"},
			{ID: "rank", Name: "Rank", Type: "number"},
			{ID: "rel", Name: "Related", Type: "relation"},
			{ID: "roll", Name: "Rolled up", Type: "rollup"},
			{ID: "uniq", Name: "Row id", Type: "unique_id"},
			{ID: "stat", Name: "Status", Type: "select"},
			{ID: "sumr", Name: "Summary", Type: "rich_text"},
			{ID: "vrfy", Name: "Verified", Type: "verification"},
			{ID: "fml", Name: "Window", Type: "formula"},
			{ID: "done", Name: "Done", Type: "checkbox"},
		},
	}

	condition := func(fields map[string]any) map[string]any {
		return map[string]any{"filter": map[string]any{"conditions": []any{fields}}}
	}

	build := func(t *testing.T, args map[string]any) string {
		t.Helper()
		got, err := notiontool.BuildFilterForTest(ds, args)
		gt.NoError(t, err).Required()
		return asJSON(t, got)
	}

	t.Run("writes each property type's own condition key", func(t *testing.T) {
		cases := []struct {
			name   string
			fields map[string]any
			want   string
		}{
			{
				name:   "text uses rich_text",
				fields: map[string]any{"property": "Summary", "operator": "contains", "value": "outage"},
				want:   `{"property":"sumr","rich_text":{"contains":"outage"}}`,
			},
			{
				name:   "number takes a JSON number",
				fields: map[string]any{"property": "Rank", "operator": "greater_than", "value": "42"},
				want:   `{"number":{"greater_than":42},"property":"rank"}`,
			},
			{
				name:   "checkbox takes a JSON boolean",
				fields: map[string]any{"property": "Done", "operator": "equals", "value": "true"},
				want:   `{"checkbox":{"equals":true},"property":"done"}`,
			},
			{
				name:   "select takes an exact choice",
				fields: map[string]any{"property": "Status", "operator": "equals", "value": "Published"},
				want:   `{"property":"stat","select":{"equals":"Published"}}`,
			},
			{
				name:   "multi_select takes a choice it holds",
				fields: map[string]any{"property": "Keywords", "operator": "contains", "value": "network"},
				want:   `{"multi_select":{"contains":"network"},"property":"aikw"}`,
			},
			{
				// Notion's unique_id is a counter, and its conditions take an
				// integer.
				name:   "unique_id takes a whole number",
				fields: map[string]any{"property": "Row id", "operator": "greater_than", "value": "17"},
				want:   `{"property":"uniq","unique_id":{"greater_than":17}}`,
			},
			{
				name:   "date takes an ISO day",
				fields: map[string]any{"property": "Due", "operator": "on_or_before", "value": "2026-01-31"},
				want:   `{"date":{"on_or_before":"2026-01-31"},"property":"due"}`,
			},
			{
				name:   "date takes an ISO timestamp",
				fields: map[string]any{"property": "Due", "operator": "after", "value": "2026-01-31T09:00:00Z"},
				want:   `{"date":{"after":"2026-01-31T09:00:00Z"},"property":"due"}`,
			},
			{
				name:   "a relative date takes an empty object",
				fields: map[string]any{"property": "Due", "operator": "past_week"},
				want:   `{"date":{"past_week":{}},"property":"due"}`,
			},
			{
				name:   "an emptiness check takes a boolean",
				fields: map[string]any{"property": "Files", "operator": "is_not_empty"},
				want:   `{"files":{"is_not_empty":true},"property":"att"}`,
			},
			{
				name:   "an emptiness check ignores a value it was given",
				fields: map[string]any{"property": "Summary", "operator": "is_empty", "value": "ignored"},
				want:   `{"property":"sumr","rich_text":{"is_empty":true}}`,
			},
			{
				name:   "people takes a user id",
				fields: map[string]any{"property": "Owner", "operator": "contains", "value": "user-1"},
				want:   `{"people":{"contains":"user-1"},"property":"ownr"}`,
			},
			{
				// created_by and last_edited_by are filtered under the people
				// key, not under their own type name.
				name:   "created_by uses the people key",
				fields: map[string]any{"property": "Created by", "operator": "contains", "value": "user-1"},
				want:   `{"people":{"contains":"user-1"},"property":"cby"}`,
			},
			{
				name:   "relation takes a page id",
				fields: map[string]any{"property": "Related", "operator": "contains", "value": "page-1"},
				want:   `{"property":"rel","relation":{"contains":"page-1"}}`,
			},
			{
				// A created_time / last_edited_time property takes Notion's
				// top-level timestamp filter, which names no property at all.
				name:   "created_time uses the timestamp filter",
				fields: map[string]any{"property": "Created on", "operator": "before", "value": "2026-01-01"},
				want:   `{"created_time":{"before":"2026-01-01"},"timestamp":"created_time"}`,
			},
			{
				name:   "last_edited_time uses the timestamp filter",
				fields: map[string]any{"property": "Last edited", "operator": "past_month"},
				want:   `{"last_edited_time":{"past_month":{}},"timestamp":"last_edited_time"}`,
			},
			{
				// Notion keys a formula's text filter "string".
				name:   "a formula's text result is keyed string",
				fields: map[string]any{"property": "Window", "operator": "contains", "value": "late", "value_type": "text"},
				want:   `{"formula":{"string":{"contains":"late"}},"property":"fml"}`,
			},
			{
				name:   "a formula's date result is keyed date",
				fields: map[string]any{"property": "Window", "operator": "after", "value": "2026-01-01", "value_type": "date"},
				want:   `{"formula":{"date":{"after":"2026-01-01"}},"property":"fml"}`,
			},
			{
				// A rollup's text filter is keyed rich_text, unlike a formula's.
				name: "a rollup over a list nests the aggregation",
				fields: map[string]any{
					"property": "Rolled up", "operator": "contains", "value": "late",
					"value_type": "text", "aggregation": "any",
				},
				want: `{"property":"roll","rollup":{"any":{"rich_text":{"contains":"late"}}}}`,
			},
			{
				name: "a rollup that computes a number takes the condition directly",
				fields: map[string]any{
					"property": "Rolled up", "operator": "does_not_equal", "value": "42", "value_type": "number",
				},
				want: `{"property":"roll","rollup":{"number":{"does_not_equal":42}}}`,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				gt.String(t, build(t, condition(tc.fields))).Equal(tc.want)
			})
		}
	})

	t.Run("accepts a number or a boolean written as JSON rather than text", func(t *testing.T) {
		gt.String(t, build(t, condition(map[string]any{
			"property": "Rank", "operator": "equals", "value": float64(7),
		}))).Equal(`{"number":{"equals":7},"property":"rank"}`)

		gt.String(t, build(t, condition(map[string]any{
			"property": "Done", "operator": "equals", "value": true,
		}))).Equal(`{"checkbox":{"equals":true},"property":"done"}`)
	})

	t.Run("combines conditions and groups two levels deep", func(t *testing.T) {
		got := build(t, map[string]any{"filter": map[string]any{
			"operator": "and",
			"conditions": []any{
				map[string]any{"property": "Status", "operator": "equals", "value": "Published"},
			},
			"groups": []any{
				map[string]any{
					"operator": "or",
					"conditions": []any{
						map[string]any{"property": "Keywords", "operator": "contains", "value": "network"},
						map[string]any{"property": "Keywords", "operator": "contains", "value": "storage"},
					},
				},
			},
		}})
		gt.String(t, got).Equal(
			`{"and":[{"property":"stat","select":{"equals":"Published"}},` +
				`{"or":[{"multi_select":{"contains":"network"},"property":"aikw"},` +
				`{"multi_select":{"contains":"storage"},"property":"aikw"}]}]}`)
	})

	t.Run("defaults the top-level operator to and", func(t *testing.T) {
		got := build(t, map[string]any{"filter": map[string]any{"conditions": []any{
			map[string]any{"property": "Status", "operator": "equals", "value": "Published"},
			map[string]any{"property": "Rank", "operator": "greater_than", "value": "1"},
		}}})
		gt.String(t, got).Equal(
			`{"and":[{"property":"stat","select":{"equals":"Published"}},` +
				`{"number":{"greater_than":1},"property":"rank"}]}`)
	})

	t.Run("merges keywords into an and-filter without adding a level", func(t *testing.T) {
		got := build(t, map[string]any{
			"query": "outage",
			"filter": map[string]any{
				"operator":   "and",
				"conditions": []any{map[string]any{"property": "Status", "operator": "equals", "value": "Published"}},
				"groups": []any{map[string]any{
					"operator":   "or",
					"conditions": []any{map[string]any{"property": "Keywords", "operator": "contains", "value": "network"}},
				}},
			},
		})
		gt.String(t, got).Equal(
			`{"and":[{"property":"title","rich_text":{"contains":"outage"}},` +
				`{"property":"stat","select":{"equals":"Published"}},` +
				`{"or":[{"multi_select":{"contains":"network"},"property":"aikw"}]}]}`)
	})

	t.Run("wraps an or-filter that has no groups", func(t *testing.T) {
		got := build(t, map[string]any{
			"query": "outage",
			"filter": map[string]any{
				"operator": "or",
				"conditions": []any{
					map[string]any{"property": "Status", "operator": "equals", "value": "Published"},
					map[string]any{"property": "Status", "operator": "equals", "value": "Draft"},
				},
			},
		})
		gt.String(t, got).Equal(
			`{"and":[{"property":"title","rich_text":{"contains":"outage"}},` +
				`{"or":[{"property":"stat","select":{"equals":"Published"}},` +
				`{"property":"stat","select":{"equals":"Draft"}}]}]}`)
	})

	// The keyword parts already occupy the top "and" and the group occupies the
	// level below it, so wrapping the or would be a third level.
	t.Run("rejects keywords alongside an or-filter that has groups", func(t *testing.T) {
		_, err := notiontool.BuildFilterForTest(ds, map[string]any{
			"query": "outage",
			"filter": map[string]any{
				"operator":   "or",
				"conditions": []any{map[string]any{"property": "Status", "operator": "equals", "value": "Published"}},
				"groups": []any{map[string]any{
					"operator":   "and",
					"conditions": []any{map[string]any{"property": "Rank", "operator": "greater_than", "value": "1"}},
				}},
			},
		})
		gt.Value(t, err).NotNil().Required()
		gt.Bool(t, notiontool.IsRejectionForTest(err)).True()
		gt.String(t, err.Error()).Contains("two levels")
		gt.String(t, err.Error()).Contains("conditions inside filter")
	})

	t.Run("rejects a condition Notion cannot express", func(t *testing.T) {
		cases := []struct {
			name   string
			fields map[string]any
			want   string
		}{
			{
				name:   "an operator the type does not accept",
				fields: map[string]any{"property": "Status", "operator": "contains", "value": "Pub"},
				want:   "equals, does_not_equal, is_empty, is_not_empty",
			},
			{
				name:   "an operator that does not exist",
				fields: map[string]any{"property": "Summary", "operator": "matches", "value": "x"},
				want:   "matches",
			},
			{
				name:   "a missing operator",
				fields: map[string]any{"property": "Summary", "value": "x"},
				want:   "cannot be used",
			},
			{
				name:   "a value that is not a number",
				fields: map[string]any{"property": "Rank", "operator": "equals", "value": "forty-two"},
				want:   "digits",
			},
			{
				// strconv.ParseFloat accepts these three, and none of them can
				// be encoded as JSON: letting one through turns a repairable
				// argument into a request that fails to serialise.
				name:   "a number that is NaN",
				fields: map[string]any{"property": "Rank", "operator": "equals", "value": "NaN"},
				want:   "digits",
			},
			{
				name:   "a number that is positive infinity",
				fields: map[string]any{"property": "Rank", "operator": "greater_than", "value": "Inf"},
				want:   "digits",
			},
			{
				name:   "a number that is negative infinity",
				fields: map[string]any{"property": "Rank", "operator": "less_than", "value": "-Inf"},
				want:   "digits",
			},
			{
				name:   "a unique_id that is a fraction",
				fields: map[string]any{"property": "Row id", "operator": "equals", "value": "1.5"},
				want:   "no decimal point",
			},
			{
				name:   "a value that is not a boolean",
				fields: map[string]any{"property": "Done", "operator": "equals", "value": "yes please"},
				want:   `"true"`,
			},
			{
				name:   "a value that is not an ISO date",
				fields: map[string]any{"property": "Due", "operator": "before", "value": "next Tuesday"},
				want:   "ISO 8601",
			},
			{
				name:   "a missing value",
				fields: map[string]any{"property": "Summary", "operator": "contains"},
				want:   "needs a value",
			},
			{
				name:   "a formula without a declared result type",
				fields: map[string]any{"property": "Window", "operator": "contains", "value": "late"},
				want:   "value_type is required",
			},
			{
				name: "a rollup over a list without an aggregation",
				fields: map[string]any{
					"property": "Rolled up", "operator": "contains", "value": "late", "value_type": "text",
				},
				want: "needs an aggregation",
			},
			{
				name: "an aggregation that does not exist",
				fields: map[string]any{
					"property": "Rolled up", "operator": "contains", "value": "late",
					"value_type": "text", "aggregation": "some",
				},
				want: "any, every, none",
			},
			{
				name: "a result type that does not exist",
				fields: map[string]any{
					"property": "Window", "operator": "contains", "value": "late", "value_type": "duration",
				},
				want: "value_type",
			},
			{
				name:   "a property type Notion cannot filter on",
				fields: map[string]any{"property": "Verified", "operator": "equals", "value": "verified"},
				want:   "cannot filter on",
			},
			{
				name:   "a property that does not exist",
				fields: map[string]any{"property": "Assignee", "operator": "equals", "value": "x"},
				want:   "available properties",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := notiontool.BuildFilterForTest(ds, condition(tc.fields))
				gt.Value(t, err).NotNil().Required()
				gt.Bool(t, notiontool.IsRejectionForTest(err)).True()
				gt.String(t, err.Error()).Contains(tc.want)
			})
		}
	})

	t.Run("rejects a filter shape that is not the documented one", func(t *testing.T) {
		cases := []struct {
			name string
			args map[string]any
			want string
		}{
			{
				name: "filter is not an object",
				args: map[string]any{"filter": "Status = Published"},
				want: "must be an object",
			},
			{
				name: "conditions is not an array",
				args: map[string]any{"filter": map[string]any{"conditions": "Status"}},
				want: "filter.conditions must be an array",
			},
			{
				name: "a condition is not an object",
				args: map[string]any{"filter": map[string]any{"conditions": []any{"Status"}}},
				want: "filter.conditions[0] must be an object",
			},
			{
				name: "a condition field is not text",
				args: map[string]any{"filter": map[string]any{"conditions": []any{
					map[string]any{"property": []any{"Status"}, "operator": "equals"},
				}}},
				want: "filter.conditions[0].property must be text",
			},
			{
				name: "groups is not an array",
				args: map[string]any{"filter": map[string]any{"groups": "or"}},
				want: "filter.groups must be an array",
			},
			{
				name: "a group holds no conditions",
				args: map[string]any{"filter": map[string]any{"groups": []any{
					map[string]any{"operator": "or", "conditions": []any{}},
				}}},
				want: "filter.groups[0] holds no conditions",
			},
			{
				name: "a group operator is not and or or",
				args: map[string]any{"filter": map[string]any{"groups": []any{
					map[string]any{"operator": "either", "conditions": []any{
						map[string]any{"property": "Rank", "operator": "equals", "value": "1"},
					}},
				}}},
				want: "filter.groups[0].operator",
			},
			{
				name: "the top-level operator is not and or or",
				args: map[string]any{"filter": map[string]any{
					"operator":   "either",
					"conditions": []any{map[string]any{"property": "Rank", "operator": "equals", "value": "1"}},
				}},
				want: "filter.operator",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := notiontool.BuildFilterForTest(ds, tc.args)
				gt.Value(t, err).NotNil().Required()
				gt.Bool(t, notiontool.IsRejectionForTest(err)).True()
				gt.String(t, err.Error()).Contains(tc.want)
			})
		}
	})

	t.Run("rejects more conditions or groups than the caps", func(t *testing.T) {
		many := make([]any, 0, 26)
		for i := 0; i < 26; i++ {
			many = append(many, map[string]any{"property": "Rank", "operator": "equals", "value": "1"})
		}
		_, err := notiontool.BuildFilterForTest(ds, map[string]any{"filter": map[string]any{"conditions": many}})
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("26")
		gt.String(t, err.Error()).Contains("25")

		groups := make([]any, 0, 6)
		for i := 0; i < 6; i++ {
			groups = append(groups, map[string]any{"operator": "or", "conditions": []any{
				map[string]any{"property": "Rank", "operator": "equals", "value": "1"},
			}})
		}
		_, err = notiontool.BuildFilterForTest(ds, map[string]any{"filter": map[string]any{"groups": groups}})
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("6 groups")
	})
}

func TestBuildSorts(t *testing.T) {
	ds := testSchema()

	t.Run("orders by a property and by a timestamp", func(t *testing.T) {
		got, err := notiontool.BuildSortsForTest(ds, map[string]any{"sorts": []any{
			map[string]any{"timestamp": "last_edited_time", "direction": "descending"},
			map[string]any{"property": "Status", "direction": "ascending"},
		}})
		gt.NoError(t, err).Required()
		gt.String(t, asJSON(t, got)).Equal(
			`[{"direction":"descending","timestamp":"last_edited_time"},` +
				`{"direction":"ascending","property":"stat"}]`)
	})

	// Notion's own default is ascending, so an unstated direction is not guessed
	// at.
	t.Run("defaults the direction to ascending", func(t *testing.T) {
		got, err := notiontool.BuildSortsForTest(ds, map[string]any{"sorts": []any{
			map[string]any{"property": "Name"},
		}})
		gt.NoError(t, err).Required()
		gt.String(t, asJSON(t, got)).Equal(`[{"direction":"ascending","property":"title"}]`)
	})

	t.Run("builds nothing when no ordering is asked for", func(t *testing.T) {
		got, err := notiontool.BuildSortsForTest(ds, map[string]any{})
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(0)
	})

	t.Run("rejects an ordering Notion cannot express", func(t *testing.T) {
		cases := []struct {
			name string
			args map[string]any
			want string
		}{
			{
				name: "both a property and a timestamp",
				args: map[string]any{"sorts": []any{map[string]any{"property": "Name", "timestamp": "created_time"}}},
				want: "sets both",
			},
			{
				name: "neither a property nor a timestamp",
				args: map[string]any{"sorts": []any{map[string]any{"direction": "ascending"}}},
				want: "sets neither",
			},
			{
				name: "a direction that does not exist",
				args: map[string]any{"sorts": []any{map[string]any{"property": "Name", "direction": "down"}}},
				want: "sorts[0].direction",
			},
			{
				name: "a timestamp that does not exist",
				args: map[string]any{"sorts": []any{map[string]any{"timestamp": "opened_time"}}},
				want: "sorts[0].timestamp",
			},
			{
				name: "a property that does not exist",
				args: map[string]any{"sorts": []any{map[string]any{"property": "Assignee"}}},
				want: "available properties",
			},
			{
				name: "sorts is not an array",
				args: map[string]any{"sorts": "Name"},
				want: "sorts must be an array",
			},
			{
				name: "an entry is not an object",
				args: map[string]any{"sorts": []any{"Name"}},
				want: "sorts[0] must be an object",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := notiontool.BuildSortsForTest(ds, tc.args)
				gt.Value(t, err).NotNil().Required()
				gt.Bool(t, notiontool.IsRejectionForTest(err)).True()
				gt.String(t, err.Error()).Contains(tc.want)
			})
		}
	})

	t.Run("rejects more ordering keys than the cap", func(t *testing.T) {
		entries := make([]any, 0, 6)
		for i := 0; i < 6; i++ {
			entries = append(entries, map[string]any{"property": "Name"})
		}
		_, err := notiontool.BuildSortsForTest(ds, map[string]any{"sorts": entries})
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("6")
		gt.String(t, err.Error()).Contains("5")
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
