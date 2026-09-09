package notiontool_test

import (
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
)

func TestRenderPropertyValue(t *testing.T) {
	render := func(t *testing.T, raw string) string {
		t.Helper()
		text, ok := notiontool.RenderPropertyValueForTest([]byte(raw))
		gt.Bool(t, ok).True().Required()
		return text
	}

	t.Run("renders each property type as one line", func(t *testing.T) {
		cases := []struct {
			name string
			raw  string
			want string
		}{
			{
				name: "title joins its rich text runs",
				raw:  `{"type":"title","title":[{"plain_text":"Reset a "},{"plain_text":"stuck job"}]}`,
				want: "Reset a stuck job",
			},
			{
				name: "rich_text joins its runs",
				raw:  `{"type":"rich_text","rich_text":[{"plain_text":"cannot "},{"plain_text":"connect"}]}`,
				want: "cannot connect",
			},
			{
				name: "number keeps an integer integral",
				raw:  `{"type":"number","number":42}`,
				want: "42",
			},
			{
				name: "number keeps a fraction",
				raw:  `{"type":"number","number":1.5}`,
				want: "1.5",
			},
			{
				name: "select renders its choice name",
				raw:  `{"type":"select","select":{"name":"Published"}}`,
				want: "Published",
			},
			{
				name: "status renders its choice name",
				raw:  `{"type":"status","status":{"name":"In review"}}`,
				want: "In review",
			},
			{
				name: "multi_select joins its choice names",
				raw:  `{"type":"multi_select","multi_select":[{"name":"network"},{"name":"storage"}]}`,
				want: "network, storage",
			},
			{
				name: "date renders a single day",
				raw:  `{"type":"date","date":{"start":"2026-01-31","end":null}}`,
				want: "2026-01-31",
			},
			{
				name: "date renders a range",
				raw:  `{"type":"date","date":{"start":"2026-01-31","end":"2026-02-02"}}`,
				want: "2026-01-31 -> 2026-02-02",
			},
			{
				name: "checkbox renders a boolean",
				raw:  `{"type":"checkbox","checkbox":true}`,
				want: "true",
			},
			{
				name: "url renders its text",
				raw:  `{"type":"url","url":"https://example.com/a"}`,
				want: "https://example.com/a",
			},
			{
				name: "email renders its text",
				raw:  `{"type":"email","email":"alice@example.com"}`,
				want: "alice@example.com",
			},
			{
				name: "phone_number renders its text",
				raw:  `{"type":"phone_number","phone_number":"+81-3-0000-0000"}`,
				want: "+81-3-0000-0000",
			},
			{
				name: "people prefer their names",
				raw:  `{"type":"people","people":[{"id":"u1","name":"Alice"},{"id":"u2","name":"Bob"}]}`,
				want: "Alice, Bob",
			},
			{
				// This is what an integration allowed no user information sees.
				name: "people fall back to their ids",
				raw:  `{"type":"people","people":[{"id":"u1"},{"id":"u2","name":"Bob"}]}`,
				want: "u1, Bob",
			},
			{
				name: "files render their names",
				raw:  `{"type":"files","files":[{"name":"runbook.pdf"}]}`,
				want: "runbook.pdf",
			},
			{
				name: "relation renders the related page ids",
				raw:  `{"type":"relation","relation":[{"id":"page-1"},{"id":"page-2"}]}`,
				want: "page-1, page-2",
			},
			{
				name: "created_time renders its timestamp",
				raw:  `{"type":"created_time","created_time":"2026-01-01T00:00:00.000Z"}`,
				want: "2026-01-01T00:00:00.000Z",
			},
			{
				name: "last_edited_time renders its timestamp",
				raw:  `{"type":"last_edited_time","last_edited_time":"2026-05-01T08:00:00.000Z"}`,
				want: "2026-05-01T08:00:00.000Z",
			},
			{
				name: "created_by renders the user name",
				raw:  `{"type":"created_by","created_by":{"id":"u1","name":"Alice"}}`,
				want: "Alice",
			},
			{
				name: "last_edited_by falls back to the user id",
				raw:  `{"type":"last_edited_by","last_edited_by":{"id":"u9"}}`,
				want: "u9",
			},
			{
				name: "unique_id renders its prefix",
				raw:  `{"type":"unique_id","unique_id":{"prefix":"KB","number":17}}`,
				want: "KB-17",
			},
			{
				name: "unique_id without a prefix renders the number",
				raw:  `{"type":"unique_id","unique_id":{"prefix":null,"number":17}}`,
				want: "17",
			},
			{
				// Notion keys a formula's text result under "string", not
				// "rich_text".
				name: "formula renders a string result",
				raw:  `{"type":"formula","formula":{"type":"string","string":"overdue"}}`,
				want: "overdue",
			},
			{
				name: "formula renders a number result",
				raw:  `{"type":"formula","formula":{"type":"number","number":3}}`,
				want: "3",
			},
			{
				name: "formula renders a boolean result",
				raw:  `{"type":"formula","formula":{"type":"boolean","boolean":false}}`,
				want: "false",
			},
			{
				name: "formula renders a date result",
				raw:  `{"type":"formula","formula":{"type":"date","date":{"start":"2026-03-01"}}}`,
				want: "2026-03-01",
			},
			{
				name: "rollup renders a number result",
				raw:  `{"type":"rollup","rollup":{"type":"number","number":8}}`,
				want: "8",
			},
			{
				name: "rollup renders a date result",
				raw:  `{"type":"rollup","rollup":{"type":"date","date":{"start":"2026-02-08"}}}`,
				want: "2026-02-08",
			},
			{
				name: "rollup renders each entry of an array result",
				raw:  `{"type":"rollup","rollup":{"type":"array","array":[{"type":"rich_text","rich_text":[{"plain_text":"one"}]},{"type":"number","number":2}]}}`,
				want: "one, 2",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				gt.String(t, render(t, tc.raw)).Equal(tc.want)
			})
		}
	})

	// A row with no value in a column is a different thing from a row with no
	// such column, and a caller reading the evidence has to be able to tell.
	t.Run("renders a null value as empty text and keeps it", func(t *testing.T) {
		for _, raw := range []string{
			`{"type":"select","select":null}`,
			`{"type":"number","number":null}`,
			`{"type":"date","date":null}`,
			`{"type":"rich_text","rich_text":[]}`,
			`{"type":"formula","formula":null}`,
			`{"type":"rollup","rollup":null}`,
		} {
			text, ok := notiontool.RenderPropertyValueForTest([]byte(raw))
			gt.Bool(t, ok).True()
			gt.String(t, text).Equal("")
		}
	})

	// Notion adds property types over time — a `place` property in production is
	// what made the search decoder narrow in the first place — so an unknown one
	// is dropped rather than allowed to fail the row.
	t.Run("declines a property type it does not model", func(t *testing.T) {
		for _, raw := range []string{
			`{"type":"place","place":{"name":"HQ"}}`,
			`{"type":"verification","verification":{"state":"verified"}}`,
			`{"type":"formula","formula":{"type":"place","place":{}}}`,
			`{"type":"rollup","rollup":{"type":"incomplete"}}`,
		} {
			_, ok := notiontool.RenderPropertyValueForTest([]byte(raw))
			gt.Bool(t, ok).False()
		}
	})

	t.Run("declines a payload that does not decode", func(t *testing.T) {
		_, ok := notiontool.RenderPropertyValueForTest([]byte(`{"type":"number","number":"forty-two"}`))
		gt.Bool(t, ok).False()
	})

	t.Run("caps a long value on a rune boundary", func(t *testing.T) {
		// Two-byte runes: a byte-offset cut would split one and put an invalid
		// rune in front of the model, the run timeline and Sentry alike.
		long := strings.Repeat("é", 600)
		text := render(t, `{"type":"rich_text","rich_text":[{"plain_text":"`+long+`"}]}`)

		gt.Bool(t, strings.HasSuffix(text, "...")).True()
		gt.Number(t, len([]rune(strings.TrimSuffix(text, "...")))).Equal(500)
		gt.Bool(t, strings.ContainsRune(text, '�')).False()
	})

	t.Run("caps a long list and marks the cut", func(t *testing.T) {
		entries := make([]string, 0, 12)
		for i := 0; i < 12; i++ {
			entries = append(entries, `{"name":"tag"}`)
		}
		text := render(t, `{"type":"multi_select","multi_select":[`+strings.Join(entries, ",")+`]}`)

		gt.Number(t, strings.Count(text, "tag")).Equal(10)
		gt.Bool(t, strings.HasSuffix(text, ", ...")).True()
	})
}
