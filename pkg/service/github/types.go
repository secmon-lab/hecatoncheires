package github

import (
	"context"
	"iter"
	"time"
)

// Service provides interface to GitHub API for fetching repository data
type Service interface {
	// FetchRecentPullRequests returns PRs created since the given time, with all comments
	FetchRecentPullRequests(ctx context.Context, owner, repo string, since time.Time) iter.Seq2[*PullRequest, error]

	// FetchRecentIssues returns issues (excluding PRs) created since the given time, with all comments
	FetchRecentIssues(ctx context.Context, owner, repo string, since time.Time) iter.Seq2[*Issue, error]

	// FetchUpdatedIssueComments returns issues/PRs that received new comments since the given time,
	// excluding those already returned by FetchRecentPullRequests/FetchRecentIssues.
	// Each returned item includes the full comment history and parent context.
	FetchUpdatedIssueComments(ctx context.Context, owner, repo string, since time.Time, excludeNumbers map[int]struct{}) iter.Seq2[*IssueWithComments, error]

	// ValidateRepository checks if the repository is accessible and returns metadata
	ValidateRepository(ctx context.Context, owner, repo string) (*RepositoryValidation, error)
}

// PullRequest represents a GitHub pull request with all comments
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

// Issue represents a GitHub issue with all comments
type Issue struct {
	Number    int
	Title     string
	Body      string
	Author    string
	State     string
	URL       string
	Labels    []string
	CreatedAt time.Time
	Comments  []Comment
}

// IssueWithComments represents an issue or PR with updated comments,
// including full comment history and parent context
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
	// Since marks the boundary; comments created after this are marked as NEW
	Since time.Time
}

// Comment represents a comment on a GitHub issue or PR
type Comment struct {
	Author    string
	Body      string
	CreatedAt time.Time
	URL       string
}

// Review represents a PR review
type Review struct {
	Author    string
	Body      string
	State     string
	CreatedAt time.Time
}

// RepositoryValidation holds the result of repository validation
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
