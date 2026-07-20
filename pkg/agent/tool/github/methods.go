package github

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	ghapi "github.com/google/go-github/v88/github"
	"github.com/m-mizutani/goerr/v2"
	"github.com/shurcooL/githubv4"
)

// === Limits and clamps ===

// maxFileBytes caps the size of a single file's body returned by
// GetFileContent. 1 MB matches the GitHub Contents API ceiling and keeps the
// agent's context window safe from runaway diffs.
const maxFileBytes int64 = 1 << 20

// maxPatchBytes caps the size of a single FileChange.Patch in
// GetPullRequestDetail. 20 KB is large enough for substantial diffs while
// preventing one rogue file from drowning the rest of the response.
const maxPatchBytes = 20 * 1024

// defaultPerPage / maxPerPage clamp the search and list per_page parameter.
const (
	defaultPerPage = 20
	maxPerPage     = 50
)

func clampPerPage(n int) int {
	switch {
	case n <= 0:
		return defaultPerPage
	case n > maxPerPage:
		return maxPerPage
	default:
		return n
	}
}

// === SearchIssuesAndPRs ===

func (c *Client) SearchIssuesAndPRs(ctx context.Context, opts SearchOptions) (*SearchResult, error) {
	if strings.TrimSpace(opts.Query) == "" {
		return nil, goerr.New("query is required")
	}

	query := opts.Query
	switch strings.ToLower(opts.Type) {
	case "issue":
		if !strings.Contains(query, "is:issue") {
			query = "is:issue " + query
		}
	case "pr":
		if !strings.Contains(query, "is:pr") {
			query = "is:pr " + query
		}
	case "", "both":
		// no narrowing
	default:
		return nil, goerr.New("invalid type", goerr.V("type", opts.Type))
	}

	perPage := clampPerPage(opts.PerPage)

	var q searchUnifiedQuery
	variables := map[string]any{
		"query": githubv4.String(query),
		// #nosec G115 -- perPage is clamped to [1, maxPerPage] by clampPerPage.
		"first": githubv4.Int(int32(perPage)),
	}
	if err := c.gql.Query(ctx, &q, variables); err != nil {
		return nil, goerr.Wrap(err, "failed to search issues and pull requests",
			goerr.V("query", query))
	}

	hits := make([]SearchHit, 0, len(q.Search.Edges))
	for _, edge := range q.Search.Edges {
		hit := edge.Node.Hit
		labels := make([]string, 0, len(hit.Labels.Nodes))
		for _, l := range hit.Labels.Nodes {
			labels = append(labels, string(l.Name))
		}
		hits = append(hits, SearchHit{
			Number:    int(hit.Number),
			Title:     string(hit.Title),
			URL:       string(hit.URL),
			Author:    string(hit.Author.Login),
			State:     string(hit.State),
			CreatedAt: hit.CreatedAt.Time,
			Labels:    labels,
			IsPR:      string(hit.Typename) == "PullRequest",
			RepoOwner: string(hit.Repository.Owner.Login),
			RepoName:  string(hit.Repository.Name),
		})
	}

	return &SearchResult{
		Total: int(q.Search.IssueCount),
		Items: hits,
	}, nil
}

type searchUnifiedQuery struct {
	Search struct {
		IssueCount githubv4.Int
		Edges      []struct {
			Node struct {
				Hit searchHit `graphql:"... on Issue"`
			}
		}
	} `graphql:"search(query: $query, type: ISSUE, first: $first)"`
}

// searchHit captures the fields we read from each search node. We query as
// `... on Issue` (PullRequest implements the same scalar fields needed for a
// search hit), and use __typename to distinguish issue vs PR.
type searchHit struct {
	Typename  githubv4.String `graphql:"__typename"`
	Number    githubv4.Int
	Title     githubv4.String
	URL       githubv4.String
	State     githubv4.String
	CreatedAt githubv4.DateTime
	Author    struct {
		Login githubv4.String
	}
	Labels struct {
		Nodes []struct {
			Name githubv4.String
		}
	} `graphql:"labels(first: 20)"`
	Repository struct {
		Name  githubv4.String
		Owner struct {
			Login githubv4.String
		}
	}
}

// === GetIssue ===

// errIssueIsPR is returned when GetIssue is called with a number that
// resolves to a pull request. Callers should redirect to GetPullRequestDetail.
var errIssueIsPR = goerr.New("number resolves to a pull request, not an issue")

// ErrIssueIsPR is the public alias for errIssueIsPR so callers (including the
// agent tool) can detect this case via errors.Is.
var ErrIssueIsPR = errIssueIsPR

func (c *Client) GetIssue(ctx context.Context, owner, repo string, number int) (*Issue, error) {
	if number <= 0 {
		return nil, goerr.New("number must be > 0", goerr.V("number", number))
	}

	rest := c.restClient
	issue, resp, err := rest.Issues.Get(ctx, owner, repo, number)
	if err != nil {
		if isHTTP404(resp, err) {
			return nil, goerr.Wrap(ErrNotFound, "issue not found",
				goerr.V("owner", owner), goerr.V("repo", repo), goerr.V("number", number))
		}
		return nil, goerr.Wrap(err, "failed to get issue",
			goerr.V("owner", owner), goerr.V("repo", repo), goerr.V("number", number))
	}
	if issue.IsPullRequest() {
		return nil, goerr.Wrap(ErrIssueIsPR, "use GetPullRequestDetail for PRs",
			goerr.V("number", number))
	}

	comments, err := c.fetchAllComments(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}

	out := &Issue{
		Number:    issue.GetNumber(),
		Title:     issue.GetTitle(),
		Body:      issue.GetBody(),
		Author:    issue.GetUser().GetLogin(),
		State:     strings.ToUpper(issue.GetState()),
		URL:       issue.GetHTMLURL(),
		Labels:    labelsToStrings(issue.Labels),
		CreatedAt: issue.GetCreatedAt().Time,
		UpdatedAt: issue.GetUpdatedAt().Time,
		Comments:  comments,
	}
	if !issue.GetClosedAt().IsZero() {
		t := issue.GetClosedAt().Time
		out.ClosedAt = &t
	}
	return out, nil
}

func (c *Client) fetchAllComments(ctx context.Context, owner, repo string, number int) ([]Comment, error) {
	rest := c.restClient
	listOpts := &ghapi.IssueListCommentsOptions{
		ListOptions: ghapi.ListOptions{PerPage: 100},
	}
	var out []Comment
	for {
		page, resp, err := rest.Issues.ListComments(ctx, owner, repo, number, listOpts)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to list comments",
				goerr.V("owner", owner), goerr.V("repo", repo), goerr.V("number", number))
		}
		for _, c := range page {
			out = append(out, Comment{
				Author:    c.GetUser().GetLogin(),
				Body:      c.GetBody(),
				CreatedAt: c.GetCreatedAt().Time,
				URL:       c.GetHTMLURL(),
			})
		}
		if resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
	}
	return out, nil
}

// === GetPullRequestDetail ===

func (c *Client) GetPullRequestDetail(ctx context.Context, owner, repo string, number int, includeFiles bool) (*PullRequestDetail, error) {
	if number <= 0 {
		return nil, goerr.New("number must be > 0", goerr.V("number", number))
	}

	rest := c.restClient
	pr, resp, err := rest.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		if isHTTP404(resp, err) {
			return nil, goerr.Wrap(ErrNotFound, "pull request not found",
				goerr.V("owner", owner), goerr.V("repo", repo), goerr.V("number", number))
		}
		return nil, goerr.Wrap(err, "failed to get pull request",
			goerr.V("owner", owner), goerr.V("repo", repo), goerr.V("number", number))
	}

	comments, err := c.fetchAllComments(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}

	reviewComments, err := c.fetchAllReviewComments(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	comments = append(comments, reviewComments...)

	reviews, err := c.fetchAllReviews(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}

	detail := &PullRequestDetail{
		PullRequest: PullRequest{
			Number:    pr.GetNumber(),
			Title:     pr.GetTitle(),
			Body:      pr.GetBody(),
			Author:    pr.GetUser().GetLogin(),
			State:     strings.ToUpper(pr.GetState()),
			URL:       pr.GetHTMLURL(),
			Labels:    labelsToStrings(pr.Labels),
			CreatedAt: pr.GetCreatedAt().Time,
			Comments:  comments,
			Reviews:   reviews,
		},
		Merged:    pr.GetMerged(),
		Draft:     pr.GetDraft(),
		BaseRef:   pr.GetBase().GetRef(),
		HeadRef:   pr.GetHead().GetRef(),
		UpdatedAt: pr.GetUpdatedAt().Time,
	}
	if !pr.GetClosedAt().IsZero() {
		t := pr.GetClosedAt().Time
		detail.ClosedAt = &t
	}

	if includeFiles {
		files, err := c.fetchAllFiles(ctx, owner, repo, number)
		if err != nil {
			return nil, err
		}
		detail.Files = files
	}

	return detail, nil
}

func (c *Client) fetchAllReviewComments(ctx context.Context, owner, repo string, number int) ([]Comment, error) {
	rest := c.restClient
	listOpts := &ghapi.PullRequestListCommentsOptions{
		ListOptions: ghapi.ListOptions{PerPage: 100},
	}
	var out []Comment
	for {
		page, resp, err := rest.PullRequests.ListComments(ctx, owner, repo, number, listOpts)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to list review comments",
				goerr.V("owner", owner), goerr.V("repo", repo), goerr.V("number", number))
		}
		for _, c := range page {
			out = append(out, Comment{
				Author:    c.GetUser().GetLogin(),
				Body:      c.GetBody(),
				CreatedAt: c.GetCreatedAt().Time,
				URL:       c.GetHTMLURL(),
			})
		}
		if resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
	}
	return out, nil
}

func (c *Client) fetchAllReviews(ctx context.Context, owner, repo string, number int) ([]Review, error) {
	rest := c.restClient
	listOpts := &ghapi.ListOptions{PerPage: 100}
	var out []Review
	for {
		page, resp, err := rest.PullRequests.ListReviews(ctx, owner, repo, number, listOpts)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to list reviews",
				goerr.V("owner", owner), goerr.V("repo", repo), goerr.V("number", number))
		}
		for _, r := range page {
			out = append(out, Review{
				Author:    r.GetUser().GetLogin(),
				Body:      r.GetBody(),
				State:     r.GetState(),
				CreatedAt: r.GetSubmittedAt().Time,
			})
		}
		if resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
	}
	return out, nil
}

func (c *Client) fetchAllFiles(ctx context.Context, owner, repo string, number int) ([]FileChange, error) {
	rest := c.restClient
	listOpts := &ghapi.ListOptions{PerPage: 100}
	var out []FileChange
	for {
		page, resp, err := rest.PullRequests.ListFiles(ctx, owner, repo, number, listOpts)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to list files",
				goerr.V("owner", owner), goerr.V("repo", repo), goerr.V("number", number))
		}
		for _, f := range page {
			patch := f.GetPatch()
			truncated := false
			if len(patch) > maxPatchBytes {
				patch = safeTruncate(patch, maxPatchBytes)
				truncated = true
			}
			out = append(out, FileChange{
				Path:           f.GetFilename(),
				Status:         f.GetStatus(),
				Additions:      f.GetAdditions(),
				Deletions:      f.GetDeletions(),
				Patch:          patch,
				PatchTruncated: truncated,
			})
		}
		if resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
	}
	return out, nil
}

// === GetFileContent ===

func (c *Client) GetFileContent(ctx context.Context, owner, repo, path, ref string) (*FileContent, error) {
	if path == "" {
		return nil, goerr.New("path is required")
	}

	rest := c.restClient
	getOpts := &ghapi.RepositoryContentGetOptions{Ref: ref}
	fileContent, _, resp, err := rest.Repositories.GetContents(ctx, owner, repo, path, getOpts)
	if err != nil {
		if isHTTP404(resp, err) {
			return nil, goerr.Wrap(ErrNotFound, "file not found",
				goerr.V("owner", owner), goerr.V("repo", repo), goerr.V("path", path), goerr.V("ref", ref))
		}
		return nil, goerr.Wrap(err, "failed to get file content",
			goerr.V("owner", owner), goerr.V("repo", repo), goerr.V("path", path), goerr.V("ref", ref))
	}
	if fileContent == nil {
		return nil, goerr.Wrap(ErrNotFound, "path is not a file",
			goerr.V("path", path))
	}

	out := &FileContent{
		Path: fileContent.GetPath(),
		Ref:  fileContent.GetSHA(),
		Size: int64(fileContent.GetSize()),
	}

	body, decodeErr := fileContent.GetContent()
	if decodeErr != nil {
		return nil, goerr.Wrap(decodeErr, "failed to decode file content",
			goerr.V("path", path))
	}

	if !utf8.ValidString(body) {
		out.IsBinary = true
		return out, nil
	}

	if int64(len(body)) > maxFileBytes {
		out.Content = safeTruncate(body, int(maxFileBytes))
		out.Truncated = true
	} else {
		out.Content = body
	}
	return out, nil
}

// === ListCommits ===

func (c *Client) ListCommits(ctx context.Context, opts ListCommitsOptions) (*CommitList, error) {
	if opts.Owner == "" || opts.Repo == "" {
		return nil, goerr.New("owner and repo are required")
	}

	rest := c.restClient
	listOpts := &ghapi.CommitsListOptions{
		SHA:    opts.Ref,
		Path:   opts.Path,
		Author: opts.Author,
		ListOptions: ghapi.ListOptions{
			PerPage: clampPerPage(opts.PerPage),
		},
	}
	if !opts.Since.IsZero() {
		listOpts.Since = opts.Since
	}
	if !opts.Until.IsZero() {
		listOpts.Until = opts.Until
	}

	page, resp, err := rest.Repositories.ListCommits(ctx, opts.Owner, opts.Repo, listOpts)
	if err != nil {
		if isHTTP404(resp, err) {
			return nil, goerr.Wrap(ErrNotFound, "repository or ref not found",
				goerr.V("owner", opts.Owner), goerr.V("repo", opts.Repo), goerr.V("ref", opts.Ref))
		}
		return nil, goerr.Wrap(err, "failed to list commits",
			goerr.V("owner", opts.Owner), goerr.V("repo", opts.Repo))
	}

	items := make([]Commit, 0, len(page))
	for _, c := range page {
		gitCommit := c.GetCommit()
		commit := Commit{
			SHA:         c.GetSHA(),
			AuthorLogin: c.GetAuthor().GetLogin(),
			Message:     gitCommit.GetMessage(),
			URL:         c.GetHTMLURL(),
		}
		if a := gitCommit.GetAuthor(); a != nil {
			commit.AuthorName = a.GetName()
			commit.AuthorEmail = a.GetEmail()
			commit.AuthoredDate = a.GetDate().Time
		}
		if cm := gitCommit.GetCommitter(); cm != nil {
			commit.CommitterDate = cm.GetDate().Time
		}
		items = append(items, commit)
	}

	return &CommitList{Items: items}, nil
}

// === Helpers ===

// isHTTP404 reports whether the given response/error pair is a 404 from
// GitHub. The go-github package wraps HTTP errors with the response body, so
// status code is the most reliable signal.
func isHTTP404(resp *ghapi.Response, err error) bool {
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		return true
	}
	var er *ghapi.ErrorResponse
	if errors.As(err, &er) && er.Response != nil && er.Response.StatusCode == http.StatusNotFound {
		return true
	}
	return false
}

func labelsToStrings(in []*ghapi.Label) []string {
	out := make([]string, 0, len(in))
	for _, l := range in {
		out = append(out, l.GetName())
	}
	return out
}

// safeTruncate returns the longest valid-UTF8 prefix of s no longer than max
// bytes. Avoids splitting a multi-byte rune in half.
func safeTruncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
