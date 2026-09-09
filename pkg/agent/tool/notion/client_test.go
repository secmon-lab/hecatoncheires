package notiontool_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

func TestNewClient(t *testing.T) {
	t.Run("returns error when token is empty", func(t *testing.T) {
		_, err := notiontool.NewClient("")
		gt.Value(t, err).NotNil()
	})

	t.Run("creates client when token is provided", func(t *testing.T) {
		c, err := notiontool.NewClient("secret-token")
		gt.NoError(t, err).Required()
		gt.Value(t, c).NotNil()
	})
}

func TestSearch(t *testing.T) {
	t.Run("converts API response into SearchResult", func(t *testing.T) {
		var capturedAuth, capturedNotionVersion, capturedBody string
		var capturedMethod, capturedPath string

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/search", func(w http.ResponseWriter, r *http.Request) {
			capturedMethod = r.Method
			capturedPath = r.URL.Path
			capturedAuth = r.Header.Get("Authorization")
			capturedNotionVersion = r.Header.Get("Notion-Version")

			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			capturedBody = string(body)

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"object": "list",
				"has_more": true,
				"next_cursor": "cursor-xyz",
				"results": [
					{
						"object": "page",
						"id": "00000000-0000-0000-0000-000000000001",
						"created_time": "2026-01-01T00:00:00Z",
						"last_edited_time": "2026-04-01T12:00:00Z",
						"archived": false,
						"properties": {
							"title": {
								"id": "title",
								"type": "title",
								"title": [{"type": "text", "text": {"content": "Incident Playbook"}, "plain_text": "Incident Playbook"}]
							}
						},
						"parent": {"type": "workspace", "workspace": true},
						"url": "https://www.notion.so/Incident-Playbook-0001"
					},
					{
						"object": "database",
						"id": "00000000-0000-0000-0000-000000000002",
						"created_time": "2026-01-01T00:00:00Z",
						"last_edited_time": "2026-04-02T09:00:00Z",
						"title": [{"type": "text", "text": {"content": "Runbooks"}, "plain_text": "Runbooks"}],
						"description": [],
						"properties": {
							"Name": {"id": "title", "type": "title", "title": {}},
							"Tags": {"id": "abcd", "type": "multi_select", "multi_select": {"options": [{"id": "id", "name": "tag", "color": "blue"}]}},
							"Owner": {"id": "efgh", "type": "people", "people": {}}
						},
						"parent": {"type": "workspace", "workspace": true},
						"url": "https://www.notion.so/Runbooks-0002",
						"archived": false,
						"is_inline": false
					}
				]
			}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)

		got, err := c.Search(context.Background(), "incident", notiontool.SearchOptions{
			PageSize:   50,
			FilterType: "page",
			SortByEdit: "descending",
		})
		gt.NoError(t, err).Required()

		gt.String(t, capturedMethod).Equal(http.MethodPost)
		gt.String(t, capturedPath).Equal("/v1/search")
		gt.String(t, capturedAuth).Equal("Bearer secret-token")
		gt.String(t, capturedNotionVersion).Equal("2022-06-28")
		gt.Bool(t, strings.Contains(capturedBody, `"query":"incident"`)).True()
		gt.Bool(t, strings.Contains(capturedBody, `"page_size":50`)).True()
		gt.Bool(t, strings.Contains(capturedBody, `"property":"object"`)).True()
		gt.Bool(t, strings.Contains(capturedBody, `"value":"page"`)).True()
		gt.Bool(t, strings.Contains(capturedBody, `"timestamp":"last_edited_time"`)).True()
		gt.Bool(t, strings.Contains(capturedBody, `"direction":"descending"`)).True()

		gt.Bool(t, got.HasMore).True()
		gt.String(t, got.NextCursor).Equal("cursor-xyz")
		gt.Array(t, got.Items).Length(2).Required()

		gt.String(t, got.Items[0].ID).Equal("00000000-0000-0000-0000-000000000001")
		gt.String(t, got.Items[0].Type).Equal("page")
		gt.String(t, got.Items[0].Title).Equal("Incident Playbook")
		gt.String(t, got.Items[0].URL).Equal("https://www.notion.so/Incident-Playbook-0001")
		gt.Bool(t, got.Items[0].LastEdited.Equal(time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC))).True()

		// The database fixture carries a real schema, where "properties" holds
		// column DEFINITIONS ("title": {}) rather than a page's values
		// ("title": [...]). Decoding both shapes with one type fails the whole
		// response, so the database title must still resolve here.
		gt.String(t, got.Items[1].ID).Equal("00000000-0000-0000-0000-000000000002")
		gt.String(t, got.Items[1].Type).Equal("database")
		gt.String(t, got.Items[1].Title).Equal("Runbooks")
		gt.String(t, got.Items[1].URL).Equal("https://www.notion.so/Runbooks-0002")
	})

	t.Run("skips a result whose object type is neither page nor database", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/search", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"object": "list",
				"has_more": false,
				"next_cursor": null,
				"results": [
					{"object": "block", "id": "00000000-0000-0000-0000-000000000005", "url": "https://www.notion.so/0005"},
					{"object": "page", "id": "00000000-0000-0000-0000-000000000006", "url": "https://www.notion.so/0006"}
				]
			}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)

		// The unrecognised entry is reported through errutil.Handle and dropped;
		// the rest of the search still reaches the caller.
		got, err := c.Search(context.Background(), "q", notiontool.SearchOptions{})
		gt.NoError(t, err).Required()
		gt.Array(t, got.Items).Length(1).Required()
		gt.String(t, got.Items[0].ID).Equal("00000000-0000-0000-0000-000000000006")
	})

	t.Run("omits the filter key when no object type is requested", func(t *testing.T) {
		var capturedBody string

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/search", func(w http.ResponseWriter, r *http.Request) {
			raw, err := io.ReadAll(r.Body)
			gt.NoError(t, err)
			capturedBody = string(raw)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","has_more":false,"next_cursor":null,"results":[]}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)

		_, err := c.Search(context.Background(), "closed network", notiontool.SearchOptions{})
		gt.NoError(t, err).Required()

		// Notion rejects the whole request with 400 when filter is present but
		// filter.property is not "object", so the key must be absent entirely.
		gt.Bool(t, strings.Contains(capturedBody, `"filter"`)).False()
		gt.Bool(t, strings.Contains(capturedBody, `"query":"closed network"`)).True()
	})

	t.Run("sends pagination, sort and filter as documented fields", func(t *testing.T) {
		var decoded map[string]any
		var capturedVersion, capturedContentType string

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/search", func(w http.ResponseWriter, r *http.Request) {
			capturedVersion = r.Header.Get("Notion-Version")
			capturedContentType = r.Header.Get("Content-Type")
			gt.NoError(t, json.NewDecoder(r.Body).Decode(&decoded))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","has_more":false,"next_cursor":null,"results":[]}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)

		_, err := c.Search(context.Background(), "runbook", notiontool.SearchOptions{
			PageSize:    30,
			FilterType:  "database",
			SortByEdit:  "ascending",
			StartCursor: "cursor-abc",
		})
		gt.NoError(t, err).Required()

		gt.String(t, capturedVersion).Equal("2022-06-28")
		gt.String(t, capturedContentType).Equal("application/json")

		gt.Value(t, decoded["query"]).Equal("runbook")
		gt.Value(t, decoded["start_cursor"]).Equal("cursor-abc")
		gt.Value(t, decoded["page_size"]).Equal(float64(30))

		filter := gt.Cast[map[string]any](t, decoded["filter"])
		gt.Value(t, filter["property"]).Equal("object")
		gt.Value(t, filter["value"]).Equal("database")

		sort := gt.Cast[map[string]any](t, decoded["sort"])
		gt.Value(t, sort["timestamp"]).Equal("last_edited_time")
		gt.Value(t, sort["direction"]).Equal("ascending")
	})

	t.Run("states the status and the upstream reason on a non-2xx response", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/search", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"object":"error","status":400,"code":"validation_error","message":"body failed validation"}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)

		_, err := c.Search(context.Background(), "q", notiontool.SearchOptions{})
		gt.Value(t, err).NotNil().Required()
		// The message is the whole of what the agent is told, so the status and
		// Notion's own reason must be in it, not only in the goerr values.
		gt.String(t, err.Error()).Contains("400")
		gt.String(t, err.Error()).Contains("validation_error")
		gt.String(t, err.Error()).Contains("body failed validation")
	})

	t.Run("keeps results carrying a property type the client does not model", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/search", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// "place" is a property type Notion added after jomei/notionapi's
			// closed switch was written; decoding through the library failed the
			// whole search on it and the agent got nothing back.
			_, _ = w.Write([]byte(`{
				"object": "list",
				"has_more": false,
				"next_cursor": null,
				"results": [
					{
						"object": "page",
						"id": "00000000-0000-0000-0000-000000000009",
						"last_edited_time": "2026-05-01T10:00:00Z",
						"url": "https://www.notion.so/Site-Visit-0009",
						"properties": {
							"Location": {"id": "abcd", "type": "place", "place": {"name": "Tokyo"}},
							"Name": {"id": "title", "type": "title", "title": [{"plain_text": "Site Visit"}]}
						}
					},
					{
						"object": "page",
						"id": "00000000-0000-0000-0000-000000000010",
						"last_edited_time": "2026-05-02T10:00:00Z",
						"url": "https://www.notion.so/0010",
						"properties": {
							"Location": {"id": "efgh", "type": "place", "place": {"name": "Osaka"}}
						}
					}
				]
			}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)

		got, err := c.Search(context.Background(), "site", notiontool.SearchOptions{})
		gt.NoError(t, err).Required()
		gt.Array(t, got.Items).Length(2).Required()
		gt.String(t, got.Items[0].ID).Equal("00000000-0000-0000-0000-000000000009")
		gt.String(t, got.Items[0].Title).Equal("Site Visit")
		gt.String(t, got.Items[0].URL).Equal("https://www.notion.so/Site-Visit-0009")

		// A page whose only property is one the client does not model still
		// reaches the agent; it just has no title to show.
		gt.String(t, got.Items[1].ID).Equal("00000000-0000-0000-0000-000000000010")
		gt.String(t, got.Items[1].Title).Equal("")
		gt.String(t, got.Items[1].URL).Equal("https://www.notion.so/0010")
	})

	t.Run("retries a 429 with the same body", func(t *testing.T) {
		var bodies []string

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/search", func(w http.ResponseWriter, r *http.Request) {
			raw, err := io.ReadAll(r.Body)
			gt.NoError(t, err)
			bodies = append(bodies, string(raw))
			if len(bodies) == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","has_more":false,"next_cursor":null,"results":[]}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)

		got, err := c.Search(context.Background(), "retry me", notiontool.SearchOptions{FilterType: "page"})
		gt.NoError(t, err).Required()
		gt.Array(t, got.Items).Length(0)

		gt.Array(t, bodies).Length(2).Required()
		gt.String(t, bodies[1]).Equal(bodies[0])
		gt.Bool(t, strings.Contains(bodies[1], `"query":"retry me"`)).True()
	})

	t.Run("retries after the default wait when Retry-After is unusable", func(t *testing.T) {
		for name, header := range map[string]string{
			"absent":      "",
			"unparseable": "later",
			"negative":    "-5",
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				attempts := 0
				mux := http.NewServeMux()
				mux.HandleFunc("/v1/search", func(w http.ResponseWriter, r *http.Request) {
					attempts++
					if attempts == 1 {
						if header != "" {
							w.Header().Set("Retry-After", header)
						}
						w.WriteHeader(http.StatusTooManyRequests)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"object":"list","has_more":false,"next_cursor":null,"results":[]}`))
				})
				srv := httptest.NewServer(mux)
				defer srv.Close()

				c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)

				_, err := c.Search(context.Background(), "q", notiontool.SearchOptions{})
				gt.NoError(t, err).Required()
				gt.Number(t, attempts).Equal(2)
			})
		}
	})

	t.Run("stops retrying when the context is cancelled during the wait", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		attempts := 0
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/search", func(w http.ResponseWriter, r *http.Request) {
			attempts++
			// A long Retry-After keeps the client inside the wait, so the only way
			// out is the context — which is what this test pins.
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			cancel()
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)

		_, err := c.Search(ctx, "q", notiontool.SearchOptions{})
		gt.Value(t, err).NotNil().Required()
		gt.Number(t, attempts).Equal(1)
	})

	t.Run("gives up after the retry budget is exhausted", func(t *testing.T) {
		attempts := 0

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/search", func(w http.ResponseWriter, r *http.Request) {
			attempts++
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)

		_, err := c.Search(context.Background(), "q", notiontool.SearchOptions{})
		gt.Value(t, err).NotNil().Required()
		gt.Number(t, attempts).Equal(3)
	})

	t.Run("clamps page size and applies defaults", func(t *testing.T) {
		var capturedBody string

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/search", func(w http.ResponseWriter, r *http.Request) {
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			capturedBody = string(body)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","has_more":false,"next_cursor":null,"results":[]}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)

		t.Run("page size over 100 is clamped", func(t *testing.T) {
			_, err := c.Search(context.Background(), "q", notiontool.SearchOptions{PageSize: 999})
			gt.NoError(t, err).Required()
			gt.Bool(t, strings.Contains(capturedBody, `"page_size":100`)).True()
		})

		t.Run("page size zero falls back to default 20", func(t *testing.T) {
			capturedBody = ""
			_, err := c.Search(context.Background(), "q", notiontool.SearchOptions{})
			gt.NoError(t, err).Required()
			gt.Bool(t, strings.Contains(capturedBody, `"page_size":20`)).True()
		})
	})
}

func TestGetPageMarkdown(t *testing.T) {
	t.Run("returns markdown body and truncated flag", func(t *testing.T) {
		var capturedAuth, capturedNotionVersion, capturedAccept, capturedPath string

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/pages/00000000-0000-0000-0000-000000000001/markdown",
			func(w http.ResponseWriter, r *http.Request) {
				capturedAuth = r.Header.Get("Authorization")
				capturedNotionVersion = r.Header.Get("Notion-Version")
				capturedAccept = r.Header.Get("Accept")
				capturedPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"markdown":"# Title\n\nbody","truncated":false}`))
			})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)

		got, err := c.GetPageMarkdown(context.Background(), "00000000-0000-0000-0000-000000000001")
		gt.NoError(t, err).Required()

		gt.String(t, capturedPath).Equal("/v1/pages/00000000-0000-0000-0000-000000000001/markdown")
		gt.String(t, capturedAuth).Equal("Bearer secret-token")
		gt.String(t, capturedNotionVersion).Equal("2026-03-11")
		gt.String(t, capturedAccept).Equal("application/json")

		gt.String(t, got.PageID).Equal("00000000-0000-0000-0000-000000000001")
		gt.String(t, got.Markdown).Equal("# Title\n\nbody")
		gt.Bool(t, got.Truncated).False()
	})

	t.Run("propagates truncated flag", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/pages/page-id/markdown", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"markdown":"x","truncated":true}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
		got, err := c.GetPageMarkdown(context.Background(), "page-id")
		gt.NoError(t, err).Required()
		gt.Bool(t, got.Truncated).True()
	})

	t.Run("retries a 429", func(t *testing.T) {
		attempts := 0

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/pages/page-id/markdown", func(w http.ResponseWriter, r *http.Request) {
			attempts++
			if attempts == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"markdown":"body","truncated":false}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)

		got, err := c.GetPageMarkdown(context.Background(), "page-id")
		gt.NoError(t, err).Required()
		gt.String(t, got.Markdown).Equal("body")
		gt.Number(t, attempts).Equal(2)
	})

	t.Run("states the status and the upstream reason on a non-2xx response", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/pages/missing/markdown", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"object":"error","status":404,"code":"object_not_found","message":"page not found"}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
		_, err := c.GetPageMarkdown(context.Background(), "missing")
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("404")
		gt.String(t, err.Error()).Contains("object_not_found")
		gt.String(t, err.Error()).Contains("page not found")

		// A page id reaches this endpoint from the model too, so it is demoted
		// on the same grounds as a database id.
		gt.Bool(t, goerr.HasTag(err, errutil.TagBenign)).True()
	})

	t.Run("falls back to the raw body when the error payload is not Notion JSON", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/pages/gateway/markdown", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("<html>upstream connect error</html>"))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
		_, err := c.GetPageMarkdown(context.Background(), "gateway")
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("502")
		gt.String(t, err.Error()).Contains("upstream connect error")
	})

	t.Run("returns error when pageID is empty", func(t *testing.T) {
		c, err := notiontool.NewClient("secret-token")
		gt.NoError(t, err).Required()
		_, err = c.GetPageMarkdown(context.Background(), "")
		gt.Value(t, err).NotNil()
	})
}

func TestGetDatabase(t *testing.T) {
	t.Run("converts the API response into Database", func(t *testing.T) {
		var capturedMethod, capturedPath, capturedAuth, capturedNotionVersion string

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/databases/00000000-0000-0000-0000-000000000002",
			func(w http.ResponseWriter, r *http.Request) {
				capturedMethod = r.Method
				capturedPath = r.URL.Path
				capturedAuth = r.Header.Get("Authorization")
				capturedNotionVersion = r.Header.Get("Notion-Version")

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"object": "database",
					"id": "00000000-0000-0000-0000-000000000002",
					"title": [{"type": "text", "plain_text": "Runbooks"}],
					"url": "https://www.notion.so/Runbooks-0002",
					"last_edited_time": "2026-04-02T09:00:00Z",
					"data_sources": [
						{"id": "ds-1", "name": "Active"},
						{"id": "ds-2", "name": "Archived"}
					]
				}`))
			})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)

		got, err := c.GetDatabase(context.Background(), "00000000-0000-0000-0000-000000000002")
		gt.NoError(t, err).Required()

		gt.String(t, capturedMethod).Equal(http.MethodGet)
		gt.String(t, capturedPath).Equal("/v1/databases/00000000-0000-0000-0000-000000000002")
		gt.String(t, capturedAuth).Equal("Bearer secret-token")
		gt.String(t, capturedNotionVersion).Equal("2026-03-11")

		gt.String(t, got.ID).Equal("00000000-0000-0000-0000-000000000002")
		gt.String(t, got.Title).Equal("Runbooks")
		gt.String(t, got.URL).Equal("https://www.notion.so/Runbooks-0002")
		gt.Bool(t, got.LastEdited.Equal(time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC))).True()
		gt.Array(t, got.DataSources).Length(2).Required()
		gt.String(t, got.DataSources[0].ID).Equal("ds-1")
		gt.String(t, got.DataSources[0].Name).Equal("Active")
		gt.String(t, got.DataSources[1].ID).Equal("ds-2")
		gt.String(t, got.DataSources[1].Name).Equal("Archived")
	})

	t.Run("states the status and the upstream reason on a non-2xx response", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/databases/missing", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"object":"error","status":404,"code":"object_not_found","message":"database not found"}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
		_, err := c.GetDatabase(context.Background(), "missing")
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("404")
		gt.String(t, err.Error()).Contains("object_not_found")
		gt.String(t, err.Error()).Contains("database not found")

		// An id the integration cannot see is the model's to correct, not an
		// operator's defect: the failure is still returned and still fed back to
		// the model, but errutil.Handle demotes it out of Sentry.
		gt.Bool(t, goerr.HasTag(err, errutil.TagBenign)).True()
	})

	t.Run("keeps a benign 404 benign along the chain that reports it", func(t *testing.T) {
		// The error reaching errutil.Handle is not the one returned here: the
		// tool wraps it, kernel's toolErrorValuesMiddleware wraps THAT in a
		// non-goerr type of its own, and react wraps the result again
		// (pkg/agent/react/react.go). A tag readable only on the innermost
		// error, or lost across the non-goerr link, would never demote anything.
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/databases/missing", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"object":"error","status":404,"code":"object_not_found","message":"database not found"}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
		_, err := c.GetDatabase(context.Background(), "missing")
		gt.Value(t, err).NotNil().Required()

		wrapped := goerr.Wrap(
			&nonGoerrWrapper{cause: goerr.Wrap(err, "failed to fetch notion database")},
			"react: tool call")
		gt.Bool(t, goerr.HasTag(wrapped, errutil.TagBenign)).True()
	})

	t.Run("leaves a non-404 reportable", func(t *testing.T) {
		// 401/403 name a token or sharing defect and 5xx names Notion being
		// down; both are an operator's to act on, so they must stay in Sentry.
		for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError} {
			mux := http.NewServeMux()
			mux.HandleFunc("/v1/databases/refused", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"object":"error","code":"unauthorized","message":"API token is invalid"}`))
			})
			srv := httptest.NewServer(mux)

			c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
			_, err := c.GetDatabase(context.Background(), "refused")
			gt.Value(t, err).NotNil().Required()
			gt.Bool(t, goerr.HasTag(err, errutil.TagBenign)).False()

			srv.Close()
		}
	})

	t.Run("returns error when databaseID is empty", func(t *testing.T) {
		c, err := notiontool.NewClient("secret-token")
		gt.NoError(t, err).Required()
		_, err = c.GetDatabase(context.Background(), "")
		gt.Value(t, err).NotNil()
	})
}

func TestQueryDataSource(t *testing.T) {
	t.Run("converts the row pages into SearchItems", func(t *testing.T) {
		var capturedMethod, capturedPath, capturedNotionVersion, capturedBody string

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/data_sources/ds-1/query", func(w http.ResponseWriter, r *http.Request) {
			capturedMethod = r.Method
			capturedPath = r.URL.Path
			capturedNotionVersion = r.Header.Get("Notion-Version")

			raw, err := io.ReadAll(r.Body)
			gt.NoError(t, err)
			capturedBody = string(raw)

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"object": "list",
				"has_more": true,
				"next_cursor": "cursor-row-2",
				"results": [
					{
						"object": "page",
						"id": "00000000-0000-0000-0000-0000000000a1",
						"last_edited_time": "2026-05-01T08:00:00Z",
						"properties": {
							"Name": {
								"id": "title",
								"type": "title",
								"title": [{"type": "text", "plain_text": "Restart the ingest worker"}]
							}
						},
						"url": "https://www.notion.so/Restart-the-ingest-worker-00a1"
					}
				]
			}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)

		got, err := c.QueryDataSource(context.Background(), "ds-1", notiontool.QueryOptions{
			PageSize:    50,
			StartCursor: "cursor-row-1",
		})
		gt.NoError(t, err).Required()

		gt.String(t, capturedMethod).Equal(http.MethodPost)
		gt.String(t, capturedPath).Equal("/v1/data_sources/ds-1/query")
		gt.String(t, capturedNotionVersion).Equal("2026-03-11")
		gt.Bool(t, strings.Contains(capturedBody, `"page_size":50`)).True()
		gt.Bool(t, strings.Contains(capturedBody, `"start_cursor":"cursor-row-1"`)).True()

		gt.Bool(t, got.HasMore).True()
		gt.String(t, got.NextCursor).Equal("cursor-row-2")
		gt.Array(t, got.Items).Length(1).Required()
		gt.String(t, got.Items[0].ID).Equal("00000000-0000-0000-0000-0000000000a1")
		gt.String(t, got.Items[0].Type).Equal("page")
		gt.String(t, got.Items[0].Title).Equal("Restart the ingest worker")
		gt.String(t, got.Items[0].URL).Equal("https://www.notion.so/Restart-the-ingest-worker-00a1")
		gt.Bool(t, got.Items[0].LastEdited.Equal(time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC))).True()
	})

	t.Run("clamps the page size to 100", func(t *testing.T) {
		var capturedBody string

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/data_sources/ds-1/query", func(w http.ResponseWriter, r *http.Request) {
			raw, err := io.ReadAll(r.Body)
			gt.NoError(t, err)
			capturedBody = string(raw)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","has_more":false,"next_cursor":null,"results":[]}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)

		got, err := c.QueryDataSource(context.Background(), "ds-1", notiontool.QueryOptions{PageSize: 500})
		gt.NoError(t, err).Required()
		gt.Bool(t, strings.Contains(capturedBody, `"page_size":100`)).True()
		gt.Array(t, got.Items).Length(0)
	})

	t.Run("states the status and the upstream reason on a non-2xx response", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/data_sources/ds-gone/query", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"object":"error","status":400,"code":"validation_error","message":"data source not found"}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
		_, err := c.QueryDataSource(context.Background(), "ds-gone", notiontool.QueryOptions{})
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("400")
		gt.String(t, err.Error()).Contains("validation_error")
		gt.String(t, err.Error()).Contains("data source not found")
	})

	t.Run("returns error when dataSourceID is empty", func(t *testing.T) {
		c, err := notiontool.NewClient("secret-token")
		gt.NoError(t, err).Required()
		_, err = c.QueryDataSource(context.Background(), "", notiontool.QueryOptions{})
		gt.Value(t, err).NotNil()
	})
}

func TestGetDataSource(t *testing.T) {
	const schemaBody = `{
		"object": "data_source",
		"id": "ds-1",
		"title": [{"type": "text", "plain_text": "Active"}],
		"properties": {
			"Name":        {"id": "title", "name": "Name", "type": "title", "title": {}},
			"Summary":  {"id": "sumr", "name": "Summary", "type": "rich_text", "rich_text": {}},
			"Keywords": {"id": "aikw", "name": "Keywords", "type": "multi_select",
			                "multi_select": {"options": [{"id": "o1", "name": "network"}, {"id": "o2", "name": "storage"}]}},
			"Status":      {"id": "stat", "name": "Status", "type": "select",
			                "select": {"options": [{"id": "o3", "name": "Published"}]}}
		}
	}`

	t.Run("converts the schema into property names and types", func(t *testing.T) {
		var capturedMethod, capturedPath, capturedNotionVersion, capturedAuth string

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/data_sources/ds-1", func(w http.ResponseWriter, r *http.Request) {
			capturedMethod = r.Method
			capturedPath = r.URL.Path
			capturedNotionVersion = r.Header.Get("Notion-Version")
			capturedAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(schemaBody))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
		got, err := c.GetDataSource(context.Background(), "ds-1")
		gt.NoError(t, err).Required()

		gt.String(t, capturedMethod).Equal(http.MethodGet)
		gt.String(t, capturedPath).Equal("/v1/data_sources/ds-1")
		gt.String(t, capturedNotionVersion).Equal("2026-03-11")
		gt.String(t, capturedAuth).Equal("Bearer secret-token")

		gt.String(t, got.ID).Equal("ds-1")
		gt.String(t, got.Name).Equal("Active")

		// Notion returns the properties as a JSON object, whose order is not
		// meaningful, so they are sorted by name.
		gt.Array(t, got.Properties).Length(4).Required()
		gt.String(t, got.Properties[0].Name).Equal("Keywords")
		gt.String(t, got.Properties[0].ID).Equal("aikw")
		gt.String(t, got.Properties[0].Type).Equal("multi_select")
		gt.Array(t, got.Properties[0].Options).Equal([]string{"network", "storage"})
		gt.Bool(t, got.Properties[0].OptionsTruncated).False()

		gt.String(t, got.Properties[1].Name).Equal("Name")
		gt.String(t, got.Properties[1].ID).Equal("title")
		gt.String(t, got.Properties[1].Type).Equal("title")

		gt.String(t, got.Properties[2].Name).Equal("Status")
		gt.String(t, got.Properties[2].ID).Equal("stat")
		gt.String(t, got.Properties[2].Type).Equal("select")
		gt.Array(t, got.Properties[2].Options).Equal([]string{"Published"})

		gt.String(t, got.Properties[3].Name).Equal("Summary")
		gt.String(t, got.Properties[3].ID).Equal("sumr")
		gt.String(t, got.Properties[3].Type).Equal("rich_text")
		gt.Array(t, got.Properties[3].Options).Length(0)
	})

	// The same reason searchResponse decodes narrowly: a property type Notion
	// adds later must not fail the whole read, and the agent should still see
	// that the column exists.
	t.Run("keeps an unknown property type and ignores unknown fields", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/data_sources/ds-2", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"object": "data_source",
				"id": "ds-2",
				"database_parent": {"type": "database_id", "database_id": "db-1"},
				"properties": {
					"Where": {"id": "plc", "name": "Where", "type": "place",
					          "place": {"unknown_shape": [1, 2, 3]}},
					"Name":  {"id": "title", "name": "Name", "type": "title", "title": {}}
				}
			}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
		got, err := c.GetDataSource(context.Background(), "ds-2")
		gt.NoError(t, err).Required()

		gt.Array(t, got.Properties).Length(2).Required()
		gt.String(t, got.Properties[0].Name).Equal("Name")
		gt.String(t, got.Properties[1].Name).Equal("Where")
		gt.String(t, got.Properties[1].Type).Equal("place")
	})

	t.Run("falls back to the map key when the object omits the name", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/data_sources/ds-3", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ds-3","properties":{"Status":{"id":"stat","type":"select","select":{"options":[]}}}}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
		got, err := c.GetDataSource(context.Background(), "ds-3")
		gt.NoError(t, err).Required()
		gt.Array(t, got.Properties).Length(1).Required()
		gt.String(t, got.Properties[0].Name).Equal("Status")
	})

	t.Run("caps the choices of one property and says it did", func(t *testing.T) {
		options := make([]string, 0, 60)
		for i := 0; i < 60; i++ {
			options = append(options, fmt.Sprintf(`{"id":"o%d","name":"choice-%d"}`, i, i))
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/data_sources/ds-4", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ds-4","properties":{"Tags":{"id":"tags","type":"multi_select","multi_select":{"options":[` +
				strings.Join(options, ",") + `]}}}}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
		got, err := c.GetDataSource(context.Background(), "ds-4")
		gt.NoError(t, err).Required()
		gt.Array(t, got.Properties).Length(1).Required()
		gt.Array(t, got.Properties[0].Options).Length(50)
		gt.Bool(t, got.Properties[0].OptionsTruncated).True()
	})

	// Notion answers object_not_found for every id the integration cannot see,
	// and the ids reaching here come from the model, so a 404 is not an
	// operator's defect to report (ARGUS-9E).
	t.Run("tags a 404 benign and states the upstream reason", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/data_sources/ds-gone", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"object":"error","status":404,"code":"object_not_found","message":"Could not find data source"}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
		_, err := c.GetDataSource(context.Background(), "ds-gone")
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("404")
		gt.String(t, err.Error()).Contains("object_not_found")
		gt.Bool(t, goerr.HasTag(err, errutil.TagBenign)).True()
	})

	t.Run("does not tag a 403 benign", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/data_sources/ds-denied", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"object":"error","status":403,"code":"restricted_resource","message":"Insufficient permissions"}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
		_, err := c.GetDataSource(context.Background(), "ds-denied")
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("restricted_resource")
		gt.Bool(t, goerr.HasTag(err, errutil.TagBenign)).False()
	})

	t.Run("returns error when dataSourceID is empty", func(t *testing.T) {
		c, err := notiontool.NewClient("secret-token")
		gt.NoError(t, err).Required()
		_, err = c.GetDataSource(context.Background(), "")
		gt.Value(t, err).NotNil()
	})
}

func TestQueryDataSourceNarrowsAndSelects(t *testing.T) {
	const rowBody = `{
		"object": "list",
		"has_more": false,
		"next_cursor": null,
		"results": [
			{
				"object": "page",
				"id": "row-1",
				"last_edited_time": "2026-05-01T08:00:00Z",
				"properties": {
					"Name":        {"id": "title", "type": "title",
					                "title": [{"type": "text", "plain_text": "Reset a stuck job"}]},
					"Summary":  {"id": "sumr", "type": "rich_text",
					                "rich_text": [{"type": "text", "plain_text": "Steps to clear the queue and retry."}]},
					"Keywords": {"id": "aikw", "type": "multi_select",
					                "multi_select": [{"id": "o1", "name": "network"}]},
					"Status":      {"id": "stat", "type": "select", "select": {"name": "Published"}}
				},
				"url": "https://www.notion.so/row-1"
			}
		]
	}`

	// The unfiltered listing is the shape notion__get_database has always sent.
	// A "filter":null or an empty sorts array would change what Notion is asked
	// for, so the keys have to be absent, not empty.
	t.Run("sends no filter, sorts or query string when nothing narrows", func(t *testing.T) {
		var capturedBody, capturedQuery string

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/data_sources/ds-1/query", func(w http.ResponseWriter, r *http.Request) {
			raw, err := io.ReadAll(r.Body)
			gt.NoError(t, err)
			capturedBody = string(raw)
			capturedQuery = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(rowBody))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
		got, err := c.QueryDataSource(context.Background(), "ds-1", notiontool.QueryOptions{})
		gt.NoError(t, err).Required()

		gt.String(t, capturedBody).Equal(`{"page_size":20}`)
		gt.String(t, capturedQuery).Equal("")
		gt.Array(t, got.Items).Length(1).Required()
		gt.String(t, got.Items[0].Title).Equal("Reset a stuck job")
		gt.Value(t, got.Items[0].Properties).Nil()
	})

	t.Run("sends the filter and the sorts it was given", func(t *testing.T) {
		var capturedBody string

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/data_sources/ds-1/query", func(w http.ResponseWriter, r *http.Request) {
			raw, err := io.ReadAll(r.Body)
			gt.NoError(t, err)
			capturedBody = string(raw)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(rowBody))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
		_, err := c.QueryDataSource(context.Background(), "ds-1", notiontool.QueryOptions{
			Filter: map[string]any{
				"and": []map[string]any{
					{"property": "title", "rich_text": map[string]any{"contains": "network"}},
				},
			},
			Sorts: []map[string]any{
				{"timestamp": "last_edited_time", "direction": "descending"},
			},
		})
		gt.NoError(t, err).Required()

		gt.Bool(t, strings.Contains(capturedBody, `"filter":{"and":[{"property":"title","rich_text":{"contains":"network"}}]}`)).True()
		gt.Bool(t, strings.Contains(capturedBody, `"sorts":[{"direction":"descending","timestamp":"last_edited_time"}]`)).True()
	})

	t.Run("asks Notion for the requested columns and the title", func(t *testing.T) {
		var capturedValues url.Values

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/data_sources/ds-1/query", func(w http.ResponseWriter, r *http.Request) {
			capturedValues = r.URL.Query()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(rowBody))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
		got, err := c.QueryDataSource(context.Background(), "ds-1", notiontool.QueryOptions{
			Properties: []notiontool.PropertySchema{
				{ID: "sumr", Name: "Summary", Type: "rich_text"},
				{ID: "aikw", Name: "Keywords", Type: "multi_select"},
			},
		})
		gt.NoError(t, err).Required()

		// The title id rides along unasked: the row's title is read out of the
		// same properties map, so a response narrowed to these two columns
		// would otherwise carry no title.
		gt.Array(t, capturedValues["filter_properties"]).Equal([]string{"sumr", "aikw", "title"})

		gt.Array(t, got.Items).Length(1).Required()
		gt.String(t, got.Items[0].Title).Equal("Reset a stuck job")
		gt.Map(t, got.Items[0].Properties).HasKey("Summary")
		gt.Map(t, got.Items[0].Properties).HasKey("Keywords")
		gt.String(t, got.Items[0].Properties["Summary"]).Equal("Steps to clear the queue and retry.")
		gt.String(t, got.Items[0].Properties["Keywords"]).Equal("network")

		// Status was not asked for, so it is not carried even though the
		// response contains it.
		_, ok := got.Items[0].Properties["Status"]
		gt.Bool(t, ok).False()
	})

	t.Run("does not repeat the title id when it was requested", func(t *testing.T) {
		var capturedValues url.Values

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/data_sources/ds-1/query", func(w http.ResponseWriter, r *http.Request) {
			capturedValues = r.URL.Query()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(rowBody))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
		_, err := c.QueryDataSource(context.Background(), "ds-1", notiontool.QueryOptions{
			Properties: []notiontool.PropertySchema{{ID: "title", Name: "Name", Type: "title"}},
		})
		gt.NoError(t, err).Required()
		gt.Array(t, capturedValues["filter_properties"]).Equal([]string{"title"})
	})

	// filter_properties is a transfer-size optimisation. The columns a row
	// carries are picked locally, so a Notion version that ignores the parameter
	// changes nothing but the size of the response.
	t.Run("carries only the requested columns even when Notion returns them all", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/data_sources/ds-1/query", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(rowBody))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
		got, err := c.QueryDataSource(context.Background(), "ds-1", notiontool.QueryOptions{
			Properties: []notiontool.PropertySchema{{ID: "stat", Name: "Status", Type: "select"}},
		})
		gt.NoError(t, err).Required()

		gt.Array(t, got.Items).Length(1).Required()
		gt.Number(t, len(got.Items[0].Properties)).Equal(1)
		gt.String(t, got.Items[0].Properties["Status"]).Equal("Published")
	})

	t.Run("skips a requested column the row does not carry", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/data_sources/ds-1/query", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(rowBody))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
		got, err := c.QueryDataSource(context.Background(), "ds-1", notiontool.QueryOptions{
			Properties: []notiontool.PropertySchema{
				{ID: "gone", Name: "Retired column", Type: "rich_text"},
				{ID: "stat", Name: "Status", Type: "select"},
			},
		})
		gt.NoError(t, err).Required()
		gt.Number(t, len(got.Items[0].Properties)).Equal(1)
		gt.String(t, got.Items[0].Properties["Status"]).Equal("Published")
	})
}

// nonGoerrWrapper stands in for kernel's toolErrorValuesError: a wrapper that is
// not a *goerr.Error but keeps the chain reachable through Unwrap.
type nonGoerrWrapper struct {
	cause error
}

func (e *nonGoerrWrapper) Error() string { return e.cause.Error() }

func (e *nonGoerrWrapper) Unwrap() error { return e.cause }
