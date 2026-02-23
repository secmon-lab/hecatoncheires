package github

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"os"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/m-mizutani/goerr/v2"
	"github.com/shurcooL/githubv4"
)

type client struct {
	gql *githubv4.Client
}

// New creates a new GitHub Service using GitHub App authentication.
// privateKey can be a PEM string or a file path to a PEM file.
func New(appID, installationID int64, privateKey string) (Service, error) {
	var key []byte

	// Try reading as file path first
	// #nosec G304 -- path comes from CLI flag, not user input
	if data, err := os.ReadFile(privateKey); err == nil {
		key = data
	} else {
		// Treat as PEM string
		key = []byte(privateKey)
	}

	tr, err := ghinstallation.New(http.DefaultTransport, appID, installationID, key)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create GitHub App transport")
	}

	httpClient := &http.Client{Transport: tr}
	gql := githubv4.NewClient(httpClient)

	return &client{gql: gql}, nil
}

// FetchRecentPullRequests fetches PRs created since the given time using GitHub GraphQL search
func (c *client) FetchRecentPullRequests(ctx context.Context, owner, repo string, since time.Time) iter.Seq2[*PullRequest, error] {
	return func(yield func(*PullRequest, error) bool) {
		query := fmt.Sprintf("repo:%s/%s is:pr created:>=%s sort:created-asc", owner, repo, since.Format("2006-01-02T15:04:05Z"))
		var cursor *githubv4.String

		for {
			var q searchPRQuery
			variables := map[string]interface{}{
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

				result := convertPullRequest(pr, owner, repo)
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

// FetchRecentIssues fetches issues (excluding PRs) created since the given time
func (c *client) FetchRecentIssues(ctx context.Context, owner, repo string, since time.Time) iter.Seq2[*Issue, error] {
	return func(yield func(*Issue, error) bool) {
		query := fmt.Sprintf("repo:%s/%s is:issue created:>=%s sort:created-asc", owner, repo, since.Format("2006-01-02T15:04:05Z"))
		var cursor *githubv4.String

		for {
			var q searchIssueQuery
			variables := map[string]interface{}{
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

				result := convertIssue(issue, owner, repo)
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

// FetchUpdatedIssueComments fetches issues/PRs that received new comments in the time range
func (c *client) FetchUpdatedIssueComments(ctx context.Context, owner, repo string, since time.Time, excludeNumbers map[int]struct{}) iter.Seq2[*IssueWithComments, error] {
	return func(yield func(*IssueWithComments, error) bool) {
		// Search for issues/PRs updated since the given time (may include comment updates)
		query := fmt.Sprintf("repo:%s/%s updated:>=%s sort:updated-asc", owner, repo, since.Format("2006-01-02T15:04:05Z"))
		var cursor *githubv4.String

		for {
			var q searchIssueWithCommentsQuery
			variables := map[string]interface{}{
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

				// Skip if already processed by FetchRecentPullRequests/FetchRecentIssues
				if _, excluded := excludeNumbers[number]; excluded {
					continue
				}

				// Check if any comment was created since the given time
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

// ValidateRepository checks repository accessibility and returns metadata
func (c *client) ValidateRepository(ctx context.Context, owner, repo string) (*RepositoryValidation, error) {
	var q repositoryQuery
	variables := map[string]interface{}{
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

	// Check if PRs can be fetched (at least the query succeeds)
	result.CanFetchPullRequests = r.PullRequests.TotalCount >= 0
	result.CanFetchIssues = r.Issues.TotalCount >= 0

	return result, nil
}

// GraphQL query types

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
	// Use typename to detect if this is a PR
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

// Conversion helpers

func convertPullRequest(pr prFragment, owner, repo string) *PullRequest {
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

func convertIssue(issue issueFragment, owner, repo string) *Issue {
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

	return &Issue{
		Number:    int(issue.Number),
		Title:     string(issue.Title),
		Body:      string(issue.Body),
		Author:    string(issue.Author.Login),
		State:     string(issue.State),
		URL:       string(issue.URL),
		Labels:    labels,
		CreatedAt: issue.CreatedAt.Time,
		Comments:  comments,
	}
}
