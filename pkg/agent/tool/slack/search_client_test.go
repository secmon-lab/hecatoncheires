package slacktool_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
)

func TestNewSearchClient(t *testing.T) {
	t.Run("returns error when token is empty", func(t *testing.T) {
		_, err := slacktool.NewSearchClient("")
		gt.Value(t, err).NotNil()
	})

	t.Run("creates service when token is provided", func(t *testing.T) {
		svc, err := slacktool.NewSearchClient("xoxp-test")
		gt.NoError(t, err).Required()
		gt.Value(t, svc).NotNil()
	})
}

func TestSearchMessages(t *testing.T) {
	t.Run("returns error when query is empty", func(t *testing.T) {
		svc, err := slacktool.NewSearchClient("xoxp-test")
		gt.NoError(t, err).Required()

		_, err = svc.SearchMessages(context.Background(), "", slacktool.SearchOptions{})
		gt.Value(t, err).NotNil()
	})

	t.Run("converts API response into SearchResult", func(t *testing.T) {
		var capturedQuery, capturedCount, capturedSort, capturedSortDir, capturedToken string

		mux := http.NewServeMux()
		mux.HandleFunc("/search.messages", func(w http.ResponseWriter, r *http.Request) {
			gt.NoError(t, r.ParseForm()).Required()
			capturedToken = r.Form.Get("token")
			capturedQuery = r.Form.Get("query")
			capturedCount = r.Form.Get("count")
			capturedSort = r.Form.Get("sort")
			capturedSortDir = r.Form.Get("sort_dir")

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"ok": true,
				"query": "incident",
				"messages": {
					"matches": [
						{
							"type": "message",
							"channel": {"id": "C111", "name": "incidents"},
							"user": "U999",
							"username": "alice",
							"ts": "1700000000.000100",
							"text": "incident playbook",
							"permalink": "https://example.slack.com/archives/C111/p1700000000000100"
						},
						{
							"type": "message",
							"channel": {"id": "C222", "name": "ops"},
							"user": "U888",
							"username": "bob",
							"ts": "1700000111.000200",
							"text": "incident review",
							"permalink": "https://example.slack.com/archives/C222/p1700000111000200"
						}
					],
					"total": 2,
					"paging": {"count": 20, "total": 2, "page": 1, "pages": 1},
					"pagination": {"total_count": 2, "page": 1, "per_page": 20, "page_count": 1, "first": 1, "last": 2}
				},
				"files": {
					"matches": [],
					"total": 0,
					"paging": {"count": 20, "total": 0, "page": 1, "pages": 0},
					"pagination": {"total_count": 0, "page": 1, "per_page": 20, "page_count": 0, "first": 0, "last": 0}
				}
			}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		svc := slacktool.NewSearchClientWithAPIURLForTest("xoxp-test", srv.URL+"/")

		got, err := svc.SearchMessages(context.Background(), "incident", slacktool.SearchOptions{Count: 50, Sort: "timestamp", SortDir: "asc"})
		gt.NoError(t, err).Required()

		gt.String(t, capturedToken).Equal("xoxp-test")
		gt.String(t, capturedQuery).Equal("incident")
		gt.String(t, capturedCount).Equal("50")
		gt.String(t, capturedSort).Equal("timestamp")
		gt.String(t, capturedSortDir).Equal("asc")

		gt.Number(t, got.Total).Equal(2)
		gt.Array(t, got.Messages).Length(2).Required()

		gt.String(t, got.Messages[0].ChannelID).Equal("C111")
		gt.String(t, got.Messages[0].ChannelName).Equal("incidents")
		gt.String(t, got.Messages[0].UserID).Equal("U999")
		gt.String(t, got.Messages[0].Username).Equal("alice")
		gt.String(t, got.Messages[0].Text).Equal("incident playbook")
		gt.String(t, got.Messages[0].Timestamp).Equal("1700000000.000100")
		gt.String(t, got.Messages[0].Permalink).Equal("https://example.slack.com/archives/C111/p1700000000000100")

		gt.String(t, got.Messages[1].ChannelID).Equal("C222")
		gt.String(t, got.Messages[1].Text).Equal("incident review")
	})

	t.Run("clamps count to max 100 and applies defaults", func(t *testing.T) {
		var capturedCount, capturedSort, capturedSortDir string

		mux := http.NewServeMux()
		mux.HandleFunc("/search.messages", func(w http.ResponseWriter, r *http.Request) {
			gt.NoError(t, r.ParseForm()).Required()
			capturedCount = r.Form.Get("count")
			capturedSort = r.Form.Get("sort")
			capturedSortDir = r.Form.Get("sort_dir")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok": true, "messages": {"matches": [], "total": 0, "paging": {}, "pagination": {}}, "files": {"matches": [], "total": 0, "paging": {}, "pagination": {}}}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		svc := slacktool.NewSearchClientWithAPIURLForTest("xoxp-test", srv.URL+"/")

		t.Run("count over max is clamped", func(t *testing.T) {
			_, err := svc.SearchMessages(context.Background(), "q", slacktool.SearchOptions{Count: 999})
			gt.NoError(t, err).Required()
			gt.String(t, capturedCount).Equal("100")
		})

		t.Run("count zero falls back to default 20 (omitted by slack-go because it equals API default)", func(t *testing.T) {
			capturedCount, capturedSort, capturedSortDir = "", "", ""
			_, err := svc.SearchMessages(context.Background(), "q", slacktool.SearchOptions{})
			gt.NoError(t, err).Required()
			gt.String(t, capturedCount).Equal("")
			gt.String(t, capturedSort).Equal("")
			gt.String(t, capturedSortDir).Equal("")
		})
	})

	t.Run("propagates error when API returns ok: false", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/search.messages", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok": false, "error": "missing_scope", "needed": "search:read"}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		svc := slacktool.NewSearchClientWithAPIURLForTest("xoxp-test", srv.URL+"/")
		_, err := svc.SearchMessages(context.Background(), "q", slacktool.SearchOptions{})
		gt.Value(t, err).NotNil()
		gt.Bool(t, strings.Contains(err.Error(), "missing_scope")).True()
	})
}
