package github

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"os"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	ghapi "github.com/google/go-github/v88/github"
	"github.com/m-mizutani/goerr/v2"
	"github.com/shurcooL/githubv4"
)

// Client wraps the GitHub App-authenticated GraphQL v4 + REST clients.
// There is exactly one production implementation; tests inject a fake via
// the package-private interface in tools.go.
// Both clients are built eagerly in NewClient and never reassigned: a single
// *Client is shared across concurrent agent runs (CommonDeps.GitHubClient), so
// lazily populating restClient on first use would be a data race.
type Client struct {
	gql        *githubv4.Client
	restClient *ghapi.Client
}

// NewClient creates a Client using GitHub App credentials.
// privateKey can be a PEM string or a file path to a PEM file.
//
// Named NewClient (not New) so the package-level New is reserved for the
// agent-tool factory in tools.go.
func NewClient(appID, installationID int64, privateKey string) (*Client, error) {
	var key []byte

	// Try reading as file path first.
	// #nosec G304 -- path comes from CLI flag, not user input
	if data, err := os.ReadFile(privateKey); err == nil {
		key = data
	} else {
		key = []byte(privateKey)
	}

	tr, err := ghinstallation.New(http.DefaultTransport, appID, installationID, key)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create GitHub App transport")
	}

	httpClient := &http.Client{Transport: tr}
	gql := githubv4.NewClient(httpClient)

	rest, err := ghapi.NewClient(ghapi.WithHTTPClient(httpClient))
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create GitHub REST client")
	}

	return &Client{gql: gql, restClient: rest}, nil
}

// FetchRecentPullRequests fetches PRs created since the given time using the
// GitHub GraphQL search.
func (c *Client) FetchRecentPullRequests(ctx context.Context, owner, repo string, since time.Time) iter.Seq2[*PullRequest, error] {
	return func(yield func(*PullRequest, error) bool) {
		query := fmt.Sprintf("repo:%s/%s is:pr created:>=%s sort:created-asc", owner, repo, since.Format("2006-01-02T15:04:05Z"))
		var cursor *githubv4.String

		for {
			var q searchPRQuery
			variables := map[string]any{
				"query":  githubv4.String(query),
				"first":  githubv4.Int(50),
				"cursor": cursor,
			}

			if err := c.gql.Query(ctx, &q, variables); err != nil {
				yield(nil, goerr.Wrap(err, "failed to search pull requests",
					goerr.V("owner", owner), goerr.V("repo", repo)))
				return
			}

			for _, edge := range q.Search.Edges {
				pr := edge.Node.PullRequest
				if pr.CreatedAt.Before(since) {
					continue
				}

				result := convertPullRequest(pr)
				if !yield(result, nil) {
					return
				}
			}

			if !q.Search.PageInfo.HasNextPage {
				return
			}
			cursor = &q.Search.PageInfo.EndCursor
		}
	}
}

// FetchRecentIssues fetches issues (excluding PRs) created since the given
// time, with all comments.
func (c *Client) FetchRecentIssues(ctx context.Context, owner, repo string, since time.Time) iter.Seq2[*Issue, error] {
	return func(yield func(*Issue, error) bool) {
		query := fmt.Sprintf("repo:%s/%s is:issue created:>=%s sort:created-asc", owner, repo, since.Format("2006-01-02T15:04:05Z"))
		var cursor *githubv4.String

		for {
			var q searchIssueQuery
			variables := map[string]any{
				"query":  githubv4.String(query),
				"first":  githubv4.Int(50),
				"cursor": cursor,
			}

			if err := c.gql.Query(ctx, &q, variables); err != nil {
				yield(nil, goerr.Wrap(err, "failed to search issues",
					goerr.V("owner", owner), goerr.V("repo", repo)))
				return
			}

			for _, edge := range q.Search.Edges {
				issue := edge.Node.Issue
				if issue.CreatedAt.Before(since) {
					continue
				}

				result := convertIssue(issue)
				if !yield(result, nil) {
					return
				}
			}

			if !q.Search.PageInfo.HasNextPage {
				return
			}
			cursor = &q.Search.PageInfo.EndCursor
		}
	}
}

// FetchUpdatedIssueComments fetches issues/PRs that received new comments in
// the time range, excluding any numbers given in excludeNumbers.
func (c *Client) FetchUpdatedIssueComments(ctx context.Context, owner, repo string, since time.Time, excludeNumbers map[int]struct{}) iter.Seq2[*IssueWithComments, error] {
	return func(yield func(*IssueWithComments, error) bool) {
		query := fmt.Sprintf("repo:%s/%s updated:>=%s sort:updated-asc", owner, repo, since.Format("2006-01-02T15:04:05Z"))
		var cursor *githubv4.String

		for {
			var q searchIssueWithCommentsQuery
			variables := map[string]any{
				"query":  githubv4.String(query),
				"first":  githubv4.Int(50),
				"cursor": cursor,
			}

			if err := c.gql.Query(ctx, &q, variables); err != nil {
				yield(nil, goerr.Wrap(err, "failed to search updated issues",
					goerr.V("owner", owner), goerr.V("repo", repo)))
				return
			}

			for _, edge := range q.Search.Edges {
				issue := edge.Node.Issue
				number := int(issue.Number)

				if _, excluded := excludeNumbers[number]; excluded {
					continue
				}

				hasNewComment := false
				var comments []Comment
				for _, c := range issue.Comments.Nodes {
					comments = append(comments, Comment{
						Author:    string(c.Author.Login),
						Body:      string(c.Body),
						CreatedAt: c.CreatedAt.Time,
						URL:       string(c.URL),
					})
					if !c.CreatedAt.Before(since) {
						hasNewComment = true
					}
				}

				if !hasNewComment {
					continue
				}

				result := &IssueWithComments{
					Number:    number,
					Title:     string(issue.Title),
					Body:      string(issue.Body),
					Author:    string(issue.Author.Login),
					State:     string(issue.State),
					URL:       string(issue.URL),
					IsPR:      issue.IsPullRequest(),
					CreatedAt: issue.CreatedAt.Time,
					Comments:  comments,
					Since:     since,
				}

				if !yield(result, nil) {
					return
				}
			}

			if !q.Search.PageInfo.HasNextPage {
				return
			}
			cursor = &q.Search.PageInfo.EndCursor
		}
	}
}

// ValidateRepository checks repository accessibility and returns metadata.
func (c *Client) ValidateRepository(ctx context.Context, owner, repo string) (*RepositoryValidation, error) {
	var q repositoryQuery
	variables := map[string]any{
		"owner": githubv4.String(owner),
		"name":  githubv4.String(repo),
	}

	if err := c.gql.Query(ctx, &q, variables); err != nil {
		return &RepositoryValidation{
			Valid:        false,
			Owner:        owner,
			Repo:         repo,
			ErrorMessage: err.Error(),
		}, nil
	}

	r := q.Repository
	result := &RepositoryValidation{
		Valid:            true,
		Owner:            owner,
		Repo:             repo,
		FullName:         fmt.Sprintf("%s/%s", owner, repo),
		Description:      string(r.Description),
		IsPrivate:        bool(r.IsPrivate),
		PullRequestCount: int(r.PullRequests.TotalCount),
		IssueCount:       int(r.Issues.TotalCount),
	}

	result.CanFetchPullRequests = r.PullRequests.TotalCount >= 0
	result.CanFetchIssues = r.Issues.TotalCount >= 0

	return result, nil
}

// === GraphQL query types and conversion helpers (legacy) ===

type searchPRQuery struct {
	Search struct {
		Edges []struct {
			Node struct {
				PullRequest prFragment `graphql:"... on PullRequest"`
			}
		}
		PageInfo pageInfo
	} `graphql:"search(query: $query, type: ISSUE, first: $first, after: $cursor)"`
}

type prFragment struct {
	Number    githubv4.Int
	Title     githubv4.String
	Body      githubv4.String
	State     githubv4.String
	URL       githubv4.String
	CreatedAt githubv4.DateTime
	Author    struct {
		Login githubv4.String
	}
	Labels struct {
		Nodes []struct {
			Name githubv4.String
		}
	} `graphql:"labels(first: 20)"`
	Comments struct {
		Nodes []commentNode
	} `graphql:"comments(first: 100)"`
	Reviews struct {
		Nodes []reviewNode
	} `graphql:"reviews(first: 100)"`
}

type searchIssueQuery struct {
	Search struct {
		Edges []struct {
			Node struct {
				Issue issueFragment `graphql:"... on Issue"`
			}
		}
		PageInfo pageInfo
	} `graphql:"search(query: $query, type: ISSUE, first: $first, after: $cursor)"`
}

type issueFragment struct {
	Number    githubv4.Int
	Title     githubv4.String
	Body      githubv4.String
	State     githubv4.String
	URL       githubv4.String
	CreatedAt githubv4.DateTime
	UpdatedAt githubv4.DateTime
	ClosedAt  *githubv4.DateTime
	Author    struct {
		Login githubv4.String
	}
	Labels struct {
		Nodes []struct {
			Name githubv4.String
		}
	} `graphql:"labels(first: 20)"`
	Comments struct {
		Nodes []commentNode
	} `graphql:"comments(first: 100)"`
}

type searchIssueWithCommentsQuery struct {
	Search struct {
		Edges []struct {
			Node struct {
				Issue issueWithPRCheck `graphql:"... on Issue"`
			}
		}
		PageInfo pageInfo
	} `graphql:"search(query: $query, type: ISSUE, first: $first, after: $cursor)"`
}

type issueWithPRCheck struct {
	Number    githubv4.Int
	Title     githubv4.String
	Body      githubv4.String
	State     githubv4.String
	URL       githubv4.String
	CreatedAt githubv4.DateTime
	Author    struct {
		Login githubv4.String
	}
	Comments struct {
		Nodes []commentNode
	} `graphql:"comments(first: 100)"`
	Typename githubv4.String `graphql:"__typename"`
}

func (i issueWithPRCheck) IsPullRequest() bool {
	return i.Typename == "PullRequest"
}

type commentNode struct {
	Author struct {
		Login githubv4.String
	}
	Body      githubv4.String
	CreatedAt githubv4.DateTime
	URL       githubv4.String
}

type reviewNode struct {
	Author struct {
		Login githubv4.String
	}
	Body      githubv4.String
	State     githubv4.String
	CreatedAt githubv4.DateTime
}

type pageInfo struct {
	HasNextPage bool
	EndCursor   githubv4.String
}

type repositoryQuery struct {
	Repository struct {
		Description  githubv4.String
		IsPrivate    githubv4.Boolean
		PullRequests struct {
			TotalCount githubv4.Int
		} `graphql:"pullRequests"`
		Issues struct {
			TotalCount githubv4.Int
		}
	} `graphql:"repository(owner: $owner, name: $name)"`
}

func convertPullRequest(pr prFragment) *PullRequest {
	var labels []string
	for _, l := range pr.Labels.Nodes {
		labels = append(labels, string(l.Name))
	}

	var comments []Comment
	for _, c := range pr.Comments.Nodes {
		comments = append(comments, Comment{
			Author:    string(c.Author.Login),
			Body:      string(c.Body),
			CreatedAt: c.CreatedAt.Time,
			URL:       string(c.URL),
		})
	}

	var reviews []Review
	for _, r := range pr.Reviews.Nodes {
		reviews = append(reviews, Review{
			Author:    string(r.Author.Login),
			Body:      string(r.Body),
			State:     string(r.State),
			CreatedAt: r.CreatedAt.Time,
		})
	}

	return &PullRequest{
		Number:    int(pr.Number),
		Title:     string(pr.Title),
		Body:      string(pr.Body),
		Author:    string(pr.Author.Login),
		State:     string(pr.State),
		URL:       string(pr.URL),
		Labels:    labels,
		CreatedAt: pr.CreatedAt.Time,
		Comments:  comments,
		Reviews:   reviews,
	}
}

func convertIssue(issue issueFragment) *Issue {
	var labels []string
	for _, l := range issue.Labels.Nodes {
		labels = append(labels, string(l.Name))
	}

	var comments []Comment
	for _, c := range issue.Comments.Nodes {
		comments = append(comments, Comment{
			Author:    string(c.Author.Login),
			Body:      string(c.Body),
			CreatedAt: c.CreatedAt.Time,
			URL:       string(c.URL),
		})
	}

	var closedAt *time.Time
	if issue.ClosedAt != nil {
		t := issue.ClosedAt.Time
		closedAt = &t
	}

	return &Issue{
		Number:    int(issue.Number),
		Title:     string(issue.Title),
		Body:      string(issue.Body),
		Author:    string(issue.Author.Login),
		State:     string(issue.State),
		URL:       string(issue.URL),
		Labels:    labels,
		CreatedAt: issue.CreatedAt.Time,
		UpdatedAt: issue.UpdatedAt.Time,
		ClosedAt:  closedAt,
		Comments:  comments,
	}
}
