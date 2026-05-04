package notiontool_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
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
						"properties": {},
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
		gt.String(t, capturedNotionVersion).NotEqual("") // notionapi sets a version
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

		gt.String(t, got.Items[1].ID).Equal("00000000-0000-0000-0000-000000000002")
		gt.String(t, got.Items[1].Type).Equal("database")
		gt.String(t, got.Items[1].Title).Equal("Runbooks")
		gt.String(t, got.Items[1].URL).Equal("https://www.notion.so/Runbooks-0002")
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

	t.Run("returns error on non-2xx response", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/pages/missing/markdown", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"object":"error","status":404,"code":"object_not_found","message":"page not found"}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := notiontool.NewClientWithBaseURLForTest("secret-token", srv.URL)
		_, err := c.GetPageMarkdown(context.Background(), "missing")
		gt.Value(t, err).NotNil()
		gt.Bool(t, strings.Contains(err.Error(), "non-2xx")).True()
	})

	t.Run("returns error when pageID is empty", func(t *testing.T) {
		c, err := notiontool.NewClient("secret-token")
		gt.NoError(t, err).Required()
		_, err = c.GetPageMarkdown(context.Background(), "")
		gt.Value(t, err).NotNil()
	})
}
