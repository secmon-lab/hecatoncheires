// Package github provides a GitHub App-authenticated client and a set of
// gollem agent tools (search, get_issue, get_pull_request, get_file,
// list_commits) that the AI agent can call against GitHub.
//
// Both the Source pipeline (legacy fetch methods) and the agent tools are
// served by the same *Client. There is no Service interface — there is only
// one implementation, and tool-side fakes are wired through a package-private
// interface in tools.go.
package github

import (
	"time"

	"github.com/m-mizutani/goerr/v2"
)

// ErrNotFound is returned when a requested GitHub resource (issue, PR, file,
// commit) does not exist or is not accessible to the GitHub App installation.
// Callers can use errors.Is to detect this without parsing error messages.
var ErrNotFound = goerr.New("github resource not found")

// PullRequest represents a GitHub pull request with all comments and reviews.
type PullRequest struct {
	Number    int
	Title     string
	Body      string
	Author    string
	State     string
	URL       string
	Labels    []string
	CreatedAt time.Time
	Comments  []Comment
	Reviews   []Review
}

// Issue represents a GitHub issue with all comments.
type Issue struct {
	Number    int
	Title     string
	Body      string
	Author    string
	State     string
	URL       string
	Labels    []string
	CreatedAt time.Time
	UpdatedAt time.Time
	ClosedAt  *time.Time // nil when the issue is still open
	Comments  []Comment
}

// IssueWithComments represents an issue or PR that received new comments
// since a given point in time, with the full comment history attached.
type IssueWithComments struct {
	Number    int
	Title     string
	Body      string
	Author    string
	State     string
	URL       string
	IsPR      bool
	CreatedAt time.Time
	Comments  []Comment
	// Since marks the boundary; comments at or after this timestamp are NEW.
	Since time.Time
}

// Comment represents a comment on a GitHub issue or PR.
type Comment struct {
	Author    string
	Body      string
	CreatedAt time.Time
	URL       string
}

// Review represents a PR review.
type Review struct {
	Author    string
	Body      string
	State     string
	CreatedAt time.Time
}

// RepositoryValidation holds the result of repository validation.
type RepositoryValidation struct {
	Valid                bool
	Owner                string
	Repo                 string
	FullName             string
	Description          string
	IsPrivate            bool
	PullRequestCount     int
	IssueCount           int
	CanFetchPullRequests bool
	CanFetchIssues       bool
	ErrorMessage         string
}

// SearchOptions configures a SearchIssuesAndPRs call.
type SearchOptions struct {
	// Query is the GitHub search query. Supports all GitHub search operators
	// (repo:, is:open, author:, label:, etc).
	Query string
	// Type narrows results: "issue", "pr", or "" / "both" for no narrowing.
	// When set, the corresponding "is:issue" / "is:pr" qualifier is appended
	// to Query if not already present.
	Type string
	// PerPage is the page size, clamped to [1, 50]. Zero defaults to 20.
	PerPage int
}

// SearchResult is the response of a SearchIssuesAndPRs call.
type SearchResult struct {
	Total int
	Items []SearchHit
}

// SearchHit is a single matched issue or PR in the search response.
type SearchHit struct {
	Number    int
	Title     string
	URL       string
	Author    string
	State     string
	CreatedAt time.Time
	Labels    []string
	IsPR      bool
	RepoOwner string
	RepoName  string
}

// PullRequestDetail is a PR with extra metadata and optionally the file diff.
// Embeds PullRequest so all base fields (comments, reviews, labels) are
// accessible directly on the detail value.
type PullRequestDetail struct {
	PullRequest
	Merged    bool
	Draft     bool
	BaseRef   string
	HeadRef   string
	UpdatedAt time.Time
	ClosedAt  *time.Time // nil when the PR is still open
	// Files is non-nil only when GetPullRequestDetail was called with
	// includeFiles=true.
	Files []FileChange
}

// FileChange represents a single file's change set in a PR.
type FileChange struct {
	Path           string
	Status         string // "added" | "modified" | "removed" | "renamed"
	Additions      int
	Deletions      int
	Patch          string
	PatchTruncated bool // true when Patch was truncated to fit the size cap
}

// FileContent is the response of GetFileContent.
type FileContent struct {
	Path      string
	Ref       string // resolved commit SHA
	Size      int64
	Content   string // empty when IsBinary is true
	Truncated bool   // true when the content was truncated to fit the size cap
	IsBinary  bool
}

// ListCommitsOptions configures a ListCommits call.
type ListCommitsOptions struct {
	Owner   string
	Repo    string
	Ref     string    // branch / tag / SHA; empty means default branch
	Path    string    // limit to commits touching this path; empty for any
	Author  string    // GitHub login or email; empty for any
	Since   time.Time // zero value means no lower bound
	Until   time.Time // zero value means no upper bound
	PerPage int       // 1..50; 0 defaults to 20
}

// CommitList is the response of ListCommits.
type CommitList struct {
	Items []Commit
}

// Commit represents a single commit in a list response.
type Commit struct {
	SHA           string
	AuthorLogin   string
	AuthorName    string
	AuthorEmail   string
	AuthoredDate  time.Time
	CommitterDate time.Time
	Message       string
	URL           string
}
