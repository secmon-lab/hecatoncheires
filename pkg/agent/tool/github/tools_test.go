package github_test

import (
	"context"
	"testing"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/github"
)

// fakeToolClient records each call and returns canned responses. Any call
// not pre-loaded returns the zero value, which gt assertions will surface.
type fakeToolClient struct {
	searchCalls   []github.SearchOptions
	getIssueCalls []struct {
		Owner, Repo string
		Number      int
	}
	getPRCalls []struct {
		Owner, Repo  string
		Number       int
		IncludeFiles bool
	}
	getFileCalls    []struct{ Owner, Repo, Path, Ref string }
	listCommitCalls []github.ListCommitsOptions

	searchResp *github.SearchResult
	issueResp  *github.Issue
	prResp     *github.PullRequestDetail
	fileResp   *github.FileContent
	commitResp *github.CommitList

	err error
}

func (f *fakeToolClient) SearchIssuesAndPRs(_ context.Context, opts github.SearchOptions) (*github.SearchResult, error) {
	f.searchCalls = append(f.searchCalls, opts)
	return f.searchResp, f.err
}
func (f *fakeToolClient) GetIssue(_ context.Context, owner, repo string, number int) (*github.Issue, error) {
	f.getIssueCalls = append(f.getIssueCalls, struct {
		Owner, Repo string
		Number      int
	}{owner, repo, number})
	return f.issueResp, f.err
}
func (f *fakeToolClient) GetPullRequestDetail(_ context.Context, owner, repo string, number int, includeFiles bool) (*github.PullRequestDetail, error) {
	f.getPRCalls = append(f.getPRCalls, struct {
		Owner, Repo  string
		Number       int
		IncludeFiles bool
	}{owner, repo, number, includeFiles})
	return f.prResp, f.err
}
func (f *fakeToolClient) GetFileContent(_ context.Context, owner, repo, path, ref string) (*github.FileContent, error) {
	f.getFileCalls = append(f.getFileCalls, struct{ Owner, Repo, Path, Ref string }{owner, repo, path, ref})
	return f.fileResp, f.err
}
func (f *fakeToolClient) ListCommits(_ context.Context, opts github.ListCommitsOptions) (*github.CommitList, error) {
	f.listCommitCalls = append(f.listCommitCalls, opts)
	return f.commitResp, f.err
}

// findTool returns the tool with the given Spec().Name, or fails the test.
func findTool(t *testing.T, c github.ToolClientForTest, name string) gollem.Tool {
	t.Helper()
	for _, tt := range github.NewToolsForTest(c) {
		if tt.Spec().Name == name {
			return tt
		}
	}
	t.Fatalf("tool %q not found", name)
	return nil
}

func TestNew_NilClientReturnsNilSlice(t *testing.T) {
	t.Parallel()
	gt.Value(t, github.New(nil)).Nil()
}

func TestNew_AllToolsRegistered(t *testing.T) {
	t.Parallel()

	tools := github.NewToolsForTest(&fakeToolClient{})
	gt.Array(t, tools).Length(5).Required()

	want := map[string]bool{
		"github__search":           false,
		"github__get_issue":        false,
		"github__get_pull_request": false,
		"github__get_file":         false,
		"github__list_commits":     false,
	}
	for _, tt := range tools {
		want[tt.Spec().Name] = true
	}
	gt.Bool(t, want["github__search"]).True()
	gt.Bool(t, want["github__get_issue"]).True()
	gt.Bool(t, want["github__get_pull_request"]).True()
	gt.Bool(t, want["github__get_file"]).True()
	gt.Bool(t, want["github__list_commits"]).True()
}

func TestSearchTool_RunPassesOpts(t *testing.T) {
	t.Parallel()

	fake := &fakeToolClient{
		searchResp: &github.SearchResult{
			Total: 1,
			Items: []github.SearchHit{{
				Number:    7,
				Title:     "hello",
				URL:       "https://example.com/i/7",
				Author:    "alice",
				State:     "OPEN",
				CreatedAt: time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
				Labels:    []string{"bug"},
				IsPR:      false,
				RepoOwner: "foo",
				RepoName:  "bar",
			}},
		},
	}

	tt := findTool(t, fake, "github__search")
	out, err := tt.Run(context.Background(), map[string]any{
		"query":    "repo:foo/bar bug",
		"type":     "issue",
		"per_page": float64(5),
	})
	gt.NoError(t, err).Required()

	gt.Array(t, fake.searchCalls).Length(1).Required()
	gt.String(t, fake.searchCalls[0].Query).Equal("repo:foo/bar bug")
	gt.String(t, fake.searchCalls[0].Type).Equal("issue")
	gt.Number(t, fake.searchCalls[0].PerPage).Equal(5)

	gt.Number(t, out["total"].(int)).Equal(1)
	items := out["items"].([]map[string]any)
	gt.Array(t, items).Length(1).Required()
	gt.Number(t, items[0]["number"].(int)).Equal(7)
	gt.Bool(t, items[0]["is_pr"].(bool)).False()
}

func TestSearchTool_RejectsEmptyQuery(t *testing.T) {
	t.Parallel()
	fake := &fakeToolClient{}
	tt := findTool(t, fake, "github__search")
	_, err := tt.Run(context.Background(), map[string]any{"query": ""})
	gt.Value(t, err).NotNil()
	gt.Array(t, fake.searchCalls).Length(0)
}

func TestGetIssueTool_RequiresOwnerRepoNumber(t *testing.T) {
	t.Parallel()
	fake := &fakeToolClient{}
	tt := findTool(t, fake, "github__get_issue")

	_, err := tt.Run(context.Background(), map[string]any{
		"owner": "foo", "repo": "", "number": float64(1),
	})
	gt.Value(t, err).NotNil()

	_, err = tt.Run(context.Background(), map[string]any{
		"owner": "foo", "repo": "bar",
	})
	gt.Value(t, err).NotNil()

	gt.Array(t, fake.getIssueCalls).Length(0)
}

func TestGetIssueTool_HappyPath(t *testing.T) {
	t.Parallel()

	closed := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)
	fake := &fakeToolClient{
		issueResp: &github.Issue{
			Number:    11,
			Title:     "X",
			Body:      "y",
			Author:    "alice",
			State:     "CLOSED",
			URL:       "https://example.com/i/11",
			Labels:    []string{"bug"},
			CreatedAt: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC),
			ClosedAt:  &closed,
			Comments: []github.Comment{{
				Author:    "bob",
				Body:      "comment",
				CreatedAt: time.Date(2025, 6, 1, 1, 0, 0, 0, time.UTC),
			}},
		},
	}

	tt := findTool(t, fake, "github__get_issue")
	out, err := tt.Run(context.Background(), map[string]any{
		"owner":  "foo",
		"repo":   "bar",
		"number": float64(11),
	})
	gt.NoError(t, err).Required()
	gt.Number(t, out["number"].(int)).Equal(11)
	gt.String(t, out["state"].(string)).Equal("CLOSED")
	gt.Map(t, out).HasKey("closed_at")
	gt.Array(t, out["comments"].([]map[string]any)).Length(1)
}

func TestGetPullRequestTool_DefaultIncludeFilesIsFalse(t *testing.T) {
	t.Parallel()

	fake := &fakeToolClient{
		prResp: &github.PullRequestDetail{
			PullRequest: github.PullRequest{
				Number:    3,
				Title:     "fix",
				State:     "OPEN",
				CreatedAt: time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC),
				Labels:    []string{},
				Comments:  []github.Comment{},
				Reviews:   []github.Review{},
			},
			BaseRef:   "main",
			HeadRef:   "fix",
			UpdatedAt: time.Date(2025, 8, 2, 0, 0, 0, 0, time.UTC),
		},
	}

	tt := findTool(t, fake, "github__get_pull_request")
	out, err := tt.Run(context.Background(), map[string]any{
		"owner":  "foo",
		"repo":   "bar",
		"number": float64(3),
	})
	gt.NoError(t, err).Required()

	gt.Array(t, fake.getPRCalls).Length(1).Required()
	gt.Bool(t, fake.getPRCalls[0].IncludeFiles).False()

	_, hasFiles := out["files"]
	gt.Bool(t, hasFiles).False()
}

func TestGetPullRequestTool_IncludeFilesPropagated(t *testing.T) {
	t.Parallel()

	fake := &fakeToolClient{
		prResp: &github.PullRequestDetail{
			PullRequest: github.PullRequest{Number: 4, State: "OPEN", Comments: []github.Comment{}, Reviews: []github.Review{}},
			Files: []github.FileChange{{
				Path:           "a.go",
				Status:         "modified",
				Additions:      1,
				Deletions:      0,
				Patch:          "@@",
				PatchTruncated: false,
			}},
		},
	}

	tt := findTool(t, fake, "github__get_pull_request")
	out, err := tt.Run(context.Background(), map[string]any{
		"owner":         "foo",
		"repo":          "bar",
		"number":        float64(4),
		"include_files": true,
	})
	gt.NoError(t, err).Required()
	gt.Bool(t, fake.getPRCalls[0].IncludeFiles).True()

	files := out["files"].([]map[string]any)
	gt.Array(t, files).Length(1).Required()
	gt.String(t, files[0]["path"].(string)).Equal("a.go")
}

func TestGetFileTool_RequiresArgs(t *testing.T) {
	t.Parallel()
	fake := &fakeToolClient{}
	tt := findTool(t, fake, "github__get_file")

	_, err := tt.Run(context.Background(), map[string]any{
		"owner": "foo", "repo": "bar", "path": "",
	})
	gt.Value(t, err).NotNil()
	gt.Array(t, fake.getFileCalls).Length(0)
}

func TestGetFileTool_HappyPath(t *testing.T) {
	t.Parallel()

	fake := &fakeToolClient{
		fileResp: &github.FileContent{
			Path:      "pkg/foo.go",
			Ref:       "abc",
			Size:      10,
			Content:   "package foo",
			Truncated: false,
			IsBinary:  false,
		},
	}
	tt := findTool(t, fake, "github__get_file")
	out, err := tt.Run(context.Background(), map[string]any{
		"owner": "foo",
		"repo":  "bar",
		"path":  "pkg/foo.go",
		"ref":   "main",
	})
	gt.NoError(t, err).Required()
	gt.String(t, out["content"].(string)).Equal("package foo")
	gt.Bool(t, out["is_binary"].(bool)).False()

	gt.Array(t, fake.getFileCalls).Length(1).Required()
	gt.String(t, fake.getFileCalls[0].Ref).Equal("main")
}

func TestListCommitsTool_ParsesSinceUntil(t *testing.T) {
	t.Parallel()

	fake := &fakeToolClient{
		commitResp: &github.CommitList{Items: []github.Commit{{
			SHA:           "abc",
			AuthorLogin:   "alice",
			Message:       "fix",
			AuthoredDate:  time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC),
			CommitterDate: time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC),
		}}},
	}

	tt := findTool(t, fake, "github__list_commits")
	out, err := tt.Run(context.Background(), map[string]any{
		"owner":  "foo",
		"repo":   "bar",
		"since":  "2025-09-01T00:00:00Z",
		"until":  "2025-09-30T00:00:00Z",
		"author": "alice",
	})
	gt.NoError(t, err).Required()

	gt.Array(t, fake.listCommitCalls).Length(1).Required()
	gt.Bool(t, fake.listCommitCalls[0].Since.Equal(time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC))).True()
	gt.Bool(t, fake.listCommitCalls[0].Until.Equal(time.Date(2025, 9, 30, 0, 0, 0, 0, time.UTC))).True()
	gt.String(t, fake.listCommitCalls[0].Author).Equal("alice")

	items := out["items"].([]map[string]any)
	gt.Array(t, items).Length(1).Required()
	gt.String(t, items[0]["sha"].(string)).Equal("abc")
}

func TestListCommitsTool_RejectsBadSince(t *testing.T) {
	t.Parallel()
	fake := &fakeToolClient{}
	tt := findTool(t, fake, "github__list_commits")
	_, err := tt.Run(context.Background(), map[string]any{
		"owner": "foo", "repo": "bar", "since": "not-a-date",
	})
	gt.Value(t, err).NotNil()
	gt.Array(t, fake.listCommitCalls).Length(0)
}
