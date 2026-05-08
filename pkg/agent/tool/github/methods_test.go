package github_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/github"
)

// newServerClient wires an httptest.Server to a Client. The handler decides
// per-test what the GitHub API responds with.
func newServerClient(t *testing.T, handler http.HandlerFunc) (*github.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := github.NewTestClient(srv.URL, srv.Client())
	return c, srv
}

func TestSearchIssuesAndPRs_AppendsTypeQualifier(t *testing.T) {
	t.Parallel()

	var receivedQuery string
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		gt.String(t, r.URL.Path).Equal("/api/graphql")

		var body struct {
			Variables map[string]any `json:"variables"`
		}
		gt.NoError(t, json.NewDecoder(r.Body).Decode(&body)).Required()
		q, _ := body.Variables["query"].(string)
		receivedQuery = q

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"search":{"issueCount":0,"edges":[]}}}`))
	})

	res, err := c.SearchIssuesAndPRs(context.Background(), github.SearchOptions{
		Query: "repo:foo/bar bug",
		Type:  "issue",
	})
	gt.NoError(t, err).Required()
	gt.Number(t, res.Total).Equal(0)
	gt.Array(t, res.Items).Length(0)
	gt.String(t, receivedQuery).Contains("is:issue")
	gt.String(t, receivedQuery).Contains("repo:foo/bar bug")
}

func TestSearchIssuesAndPRs_ParsesIssueAndPR(t *testing.T) {
	t.Parallel()

	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
            "data": {
                "search": {
                    "issueCount": 2,
                    "edges": [
                        {"node": {
                            "__typename": "Issue",
                            "number": 1,
                            "title": "Issue one",
                            "url": "https://github.com/foo/bar/issues/1",
                            "state": "OPEN",
                            "createdAt": "2025-04-01T00:00:00Z",
                            "author": {"login": "alice"},
                            "labels": {"nodes": [{"name": "bug"}]},
                            "repository": {"name": "bar", "owner": {"login": "foo"}}
                        }},
                        {"node": {
                            "__typename": "PullRequest",
                            "number": 2,
                            "title": "PR two",
                            "url": "https://github.com/foo/bar/pull/2",
                            "state": "MERGED",
                            "createdAt": "2025-04-02T00:00:00Z",
                            "author": {"login": "bob"},
                            "labels": {"nodes": []},
                            "repository": {"name": "bar", "owner": {"login": "foo"}}
                        }}
                    ]
                }
            }
        }`))
	})

	res, err := c.SearchIssuesAndPRs(context.Background(), github.SearchOptions{
		Query: "repo:foo/bar",
	})
	gt.NoError(t, err).Required()
	gt.Number(t, res.Total).Equal(2)
	gt.Array(t, res.Items).Length(2).Required()

	gt.Number(t, res.Items[0].Number).Equal(1)
	gt.String(t, res.Items[0].Title).Equal("Issue one")
	gt.Bool(t, res.Items[0].IsPR).False()
	gt.String(t, res.Items[0].RepoOwner).Equal("foo")
	gt.String(t, res.Items[0].RepoName).Equal("bar")
	gt.Array(t, res.Items[0].Labels).Length(1).Required()
	gt.String(t, res.Items[0].Labels[0]).Equal("bug")

	gt.Number(t, res.Items[1].Number).Equal(2)
	gt.String(t, res.Items[1].Title).Equal("PR two")
	gt.Bool(t, res.Items[1].IsPR).True()
}

func TestSearchIssuesAndPRs_RejectsEmptyQuery(t *testing.T) {
	t.Parallel()

	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be called when query is empty")
	})

	_, err := c.SearchIssuesAndPRs(context.Background(), github.SearchOptions{Query: "  "})
	gt.Value(t, err).NotNil()
}

func TestGetIssue_NotFound(t *testing.T) {
	t.Parallel()

	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	})

	_, err := c.GetIssue(context.Background(), "foo", "bar", 99)
	gt.Error(t, err).Is(github.ErrNotFound)
}

func TestGetIssue_RejectsPullRequest(t *testing.T) {
	t.Parallel()

	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
            "number": 7,
            "title": "PR title",
            "state": "open",
            "user": {"login": "alice"},
            "pull_request": {"url": "https://example.com/pull/7"}
        }`))
	})

	_, err := c.GetIssue(context.Background(), "foo", "bar", 7)
	gt.Error(t, err).Is(github.ErrIssueIsPR)
}

func TestGetIssue_Success(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/foo/bar/issues/12", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
            "number": 12,
            "title": "Bug",
            "body": "broken",
            "state": "closed",
            "html_url": "https://github.com/foo/bar/issues/12",
            "user": {"login": "alice"},
            "labels": [{"name": "bug"}, {"name": "p1"}],
            "created_at": "2025-04-01T00:00:00Z",
            "updated_at": "2025-04-02T00:00:00Z",
            "closed_at": "2025-04-02T00:00:00Z"
        }`))
	})
	mux.HandleFunc("/repos/foo/bar/issues/12/comments", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
            {"user": {"login": "bob"}, "body": "+1", "html_url": "https://example.com/c/1", "created_at": "2025-04-01T01:00:00Z"}
        ]`))
	})

	c, _ := newServerClient(t, mux.ServeHTTP)
	issue, err := c.GetIssue(context.Background(), "foo", "bar", 12)
	gt.NoError(t, err).Required()
	gt.Number(t, issue.Number).Equal(12)
	gt.String(t, issue.Title).Equal("Bug")
	gt.String(t, issue.State).Equal("CLOSED")
	gt.Array(t, issue.Labels).Length(2)
	gt.Value(t, issue.ClosedAt).NotNil()
	gt.Array(t, issue.Comments).Length(1).Required()
	gt.String(t, issue.Comments[0].Author).Equal("bob")
}

func TestGetPullRequestDetail_WithFiles(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/foo/bar/pulls/3", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
            "number": 3,
            "title": "Fix",
            "body": "details",
            "state": "open",
            "merged": false,
            "draft": true,
            "html_url": "https://github.com/foo/bar/pull/3",
            "user": {"login": "alice"},
            "labels": [],
            "created_at": "2025-05-01T00:00:00Z",
            "updated_at": "2025-05-02T00:00:00Z",
            "base": {"ref": "main"},
            "head": {"ref": "feat/fix"}
        }`))
	})
	mux.HandleFunc("/repos/foo/bar/issues/3/comments", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})
	mux.HandleFunc("/repos/foo/bar/pulls/3/comments", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})
	mux.HandleFunc("/repos/foo/bar/pulls/3/reviews", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
            {"user": {"login": "carol"}, "body": "LGTM", "state": "APPROVED", "submitted_at": "2025-05-02T00:00:00Z"}
        ]`))
	})
	mux.HandleFunc("/repos/foo/bar/pulls/3/files", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
            {"filename": "a.go", "status": "modified", "additions": 5, "deletions": 1, "patch": "@@ small @@"}
        ]`))
	})

	c, _ := newServerClient(t, mux.ServeHTTP)
	detail, err := c.GetPullRequestDetail(context.Background(), "foo", "bar", 3, true)
	gt.NoError(t, err).Required()
	gt.Number(t, detail.Number).Equal(3)
	gt.Bool(t, detail.Draft).True()
	gt.String(t, detail.BaseRef).Equal("main")
	gt.String(t, detail.HeadRef).Equal("feat/fix")
	gt.Array(t, detail.Reviews).Length(1).Required()
	gt.String(t, detail.Reviews[0].State).Equal("APPROVED")
	gt.Array(t, detail.Files).Length(1).Required()
	gt.String(t, detail.Files[0].Path).Equal("a.go")
	gt.Bool(t, detail.Files[0].PatchTruncated).False()
}

func TestGetPullRequestDetail_FilesOmittedWhenIncludeFilesFalse(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/foo/bar/pulls/4", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
            "number": 4, "state": "open", "user": {"login": "x"}, "labels": [],
            "base": {"ref": "main"}, "head": {"ref": "f"}
        }`))
	})
	mux.HandleFunc("/repos/foo/bar/issues/4/comments", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	mux.HandleFunc("/repos/foo/bar/pulls/4/comments", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	mux.HandleFunc("/repos/foo/bar/pulls/4/reviews", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	mux.HandleFunc("/repos/foo/bar/pulls/4/files", func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("files endpoint must not be called when includeFiles=false")
	})

	c, _ := newServerClient(t, mux.ServeHTTP)
	detail, err := c.GetPullRequestDetail(context.Background(), "foo", "bar", 4, false)
	gt.NoError(t, err).Required()
	gt.Value(t, detail.Files).Nil()
}

func TestGetFileContent_Text(t *testing.T) {
	t.Parallel()

	body := "package foo\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(body))

	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
            "type": "file",
            "encoding": "base64",
            "size": ` + sizeJSON(len(body)) + `,
            "name": "foo.go",
            "path": "pkg/foo.go",
            "sha": "abc123",
            "content": "` + encoded + `"
        }`))
	})

	res, err := c.GetFileContent(context.Background(), "foo", "bar", "pkg/foo.go", "")
	gt.NoError(t, err).Required()
	gt.String(t, res.Path).Equal("pkg/foo.go")
	gt.String(t, res.Ref).Equal("abc123")
	gt.String(t, res.Content).Equal(body)
	gt.Bool(t, res.IsBinary).False()
	gt.Bool(t, res.Truncated).False()
}

func TestGetFileContent_Binary(t *testing.T) {
	t.Parallel()

	binary := []byte{0xff, 0xfe, 0x00, 0x01, 0x02}
	encoded := base64.StdEncoding.EncodeToString(binary)

	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
            "type": "file",
            "encoding": "base64",
            "size": ` + sizeJSON(len(binary)) + `,
            "path": "icon.png",
            "sha": "deadbeef",
            "content": "` + encoded + `"
        }`))
	})

	res, err := c.GetFileContent(context.Background(), "foo", "bar", "icon.png", "")
	gt.NoError(t, err).Required()
	gt.Bool(t, res.IsBinary).True()
	gt.String(t, res.Content).Equal("")
}

func TestGetFileContent_Truncates(t *testing.T) {
	t.Parallel()

	big := strings.Repeat("a", int(github.MaxFileBytesForTest)+10)
	encoded := base64.StdEncoding.EncodeToString([]byte(big))

	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
            "type": "file",
            "encoding": "base64",
            "size": ` + sizeJSON(len(big)) + `,
            "path": "big.txt",
            "sha": "sha",
            "content": "` + encoded + `"
        }`))
	})

	res, err := c.GetFileContent(context.Background(), "foo", "bar", "big.txt", "")
	gt.NoError(t, err).Required()
	gt.Bool(t, res.Truncated).True()
	gt.Number(t, len(res.Content)).Equal(int(github.MaxFileBytesForTest))
}

func TestGetFileContent_NotFound(t *testing.T) {
	t.Parallel()

	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	})

	_, err := c.GetFileContent(context.Background(), "foo", "bar", "missing", "")
	gt.Error(t, err).Is(github.ErrNotFound)
}

func TestListCommits_PassesFilters(t *testing.T) {
	t.Parallel()

	var receivedQuery string
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		gt.String(t, r.URL.Path).Equal("/repos/foo/bar/commits")
		receivedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
            {
                "sha": "abc",
                "html_url": "https://example.com/c/abc",
                "author": {"login": "alice"},
                "commit": {
                    "message": "fix",
                    "author": {"name": "Alice", "email": "alice@example.com", "date": "2025-06-01T00:00:00Z"},
                    "committer": {"date": "2025-06-01T00:00:00Z"}
                }
            }
        ]`))
	})

	list, err := c.ListCommits(context.Background(), github.ListCommitsOptions{
		Owner:  "foo",
		Repo:   "bar",
		Path:   "pkg/foo.go",
		Author: "alice",
	})
	gt.NoError(t, err).Required()
	gt.Array(t, list.Items).Length(1).Required()
	gt.String(t, list.Items[0].SHA).Equal("abc")
	gt.String(t, list.Items[0].AuthorLogin).Equal("alice")
	gt.String(t, list.Items[0].AuthorName).Equal("Alice")
	gt.String(t, receivedQuery).Contains("path=pkg")
	gt.String(t, receivedQuery).Contains("author=alice")
}

func TestListCommits_NotFound(t *testing.T) {
	t.Parallel()

	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	})

	_, err := c.ListCommits(context.Background(), github.ListCommitsOptions{
		Owner: "foo",
		Repo:  "bar",
	})
	gt.Error(t, err).Is(github.ErrNotFound)
}

func TestSafeTruncate_PreservesUTF8Boundary(t *testing.T) {
	t.Parallel()

	// "あ" is 3 bytes in UTF-8; truncating mid-rune must back off.
	s := "あいう"
	// 4 byte cap falls inside the second rune; expect just "あ" (3 bytes).
	got := github.SafeTruncateForTest(s, 4)
	gt.String(t, got).Equal("あ")
}

func sizeJSON(n int) string {
	return strconv.Itoa(n)
}
