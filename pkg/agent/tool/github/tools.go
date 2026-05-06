package github

import (
	"context"
	"fmt"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
)

// New returns the GitHub-backed agent tools when client != nil; nil
// otherwise. Callers can append the result directly to gollem's tool list.
func New(client *Client) []gollem.Tool {
	if client == nil {
		return nil
	}
	return newTools(client)
}

// toolClient is the package-private surface the tools use, defined here as
// the test seam. *Client satisfies it implicitly. Tests inject a fake by
// calling newTools directly.
type toolClient interface {
	SearchIssuesAndPRs(ctx context.Context, opts SearchOptions) (*SearchResult, error)
	GetIssue(ctx context.Context, owner, repo string, number int) (*Issue, error)
	GetPullRequestDetail(ctx context.Context, owner, repo string, number int, includeFiles bool) (*PullRequestDetail, error)
	GetFileContent(ctx context.Context, owner, repo, path, ref string) (*FileContent, error)
	ListCommits(ctx context.Context, opts ListCommitsOptions) (*CommitList, error)
}

// newTools constructs the five gollem tools backed by the given toolClient.
// Reachable from in-package tests; outside callers go through New(*Client).
func newTools(c toolClient) []gollem.Tool {
	return []gollem.Tool{
		&searchTool{client: c},
		&getIssueTool{client: c},
		&getPullRequestTool{client: c},
		&getFileTool{client: c},
		&listCommitsTool{client: c},
	}
}

// === github__search ===

type searchTool struct {
	client toolClient
}

func (t *searchTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "github__search",
		Description: "Search GitHub issues and pull requests using GitHub search syntax. Supports operators like 'repo:owner/name', 'is:open', 'is:pr', 'is:issue', 'author:login', 'label:name', 'created:>=YYYY-MM-DD', 'updated:>=YYYY-MM-DD'.",
		Parameters: map[string]*gollem.Parameter{
			"query": {
				Type:        gollem.TypeString,
				Description: "GitHub search query.",
				Required:    true,
			},
			"type": {
				Type:        gollem.TypeString,
				Description: "Restrict to issues, pull requests, or both. Defaults to both.",
				Required:    false,
				Enum:        []string{"issue", "pr", "both"},
			},
			"per_page": {
				Type:        gollem.TypeInteger,
				Description: "Number of results per page (1-50, default 20).",
				Required:    false,
			},
		},
	}
}

func (t *searchTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return nil, goerr.New("query is required")
	}
	opts := SearchOptions{Query: query}
	if s, ok := args["type"].(string); ok {
		opts.Type = s
	}
	if v, err := tool.ExtractInt64(args, "per_page"); err == nil && v > 0 {
		opts.PerPage = int(v)
	}

	tool.Update(ctx, fmt.Sprintf("Searching GitHub: %s", query))

	res, err := t.client.SearchIssuesAndPRs(ctx, opts)
	if err != nil {
		return nil, goerr.Wrap(err, "github search failed")
	}

	items := make([]map[string]any, 0, len(res.Items))
	for _, h := range res.Items {
		items = append(items, map[string]any{
			"number":     h.Number,
			"title":      h.Title,
			"url":        h.URL,
			"author":     h.Author,
			"state":      h.State,
			"created_at": h.CreatedAt.Format(time.RFC3339),
			"labels":     h.Labels,
			"is_pr":      h.IsPR,
			"repo_owner": h.RepoOwner,
			"repo_name":  h.RepoName,
		})
	}
	return map[string]any{
		"total": res.Total,
		"items": items,
	}, nil
}

// === github__get_issue ===

type getIssueTool struct {
	client toolClient
}

func (t *getIssueTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "github__get_issue",
		Description: "Fetch a single GitHub issue (not PR) by number, with full body, labels, and all comments. If the number resolves to a pull request, the call fails — use github__get_pull_request instead.",
		Parameters: map[string]*gollem.Parameter{
			"owner":  {Type: gollem.TypeString, Description: "Repository owner (organization or user).", Required: true},
			"repo":   {Type: gollem.TypeString, Description: "Repository name.", Required: true},
			"number": {Type: gollem.TypeInteger, Description: "Issue number (positive integer).", Required: true},
		},
	}
}

func (t *getIssueTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	if owner == "" || repo == "" {
		return nil, goerr.New("owner and repo are required")
	}
	number, err := tool.ExtractInt64(args, "number")
	if err != nil {
		return nil, err
	}

	tool.Update(ctx, fmt.Sprintf("Fetching GitHub issue %s/%s#%d", owner, repo, number))

	issue, err := t.client.GetIssue(ctx, owner, repo, int(number))
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get issue")
	}

	out := map[string]any{
		"number":     issue.Number,
		"title":      issue.Title,
		"body":       issue.Body,
		"author":     issue.Author,
		"state":      issue.State,
		"url":        issue.URL,
		"labels":     issue.Labels,
		"created_at": issue.CreatedAt.Format(time.RFC3339),
		"updated_at": issue.UpdatedAt.Format(time.RFC3339),
		"comments":   commentsToMap(issue.Comments),
	}
	if issue.ClosedAt != nil {
		out["closed_at"] = issue.ClosedAt.Format(time.RFC3339)
	}
	return out, nil
}

// === github__get_pull_request ===

type getPullRequestTool struct {
	client toolClient
}

func (t *getPullRequestTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "github__get_pull_request",
		Description: "Fetch a single GitHub pull request by number, with body, labels, all comments, all reviews, and optionally the file diff. Use include_files=true only when the diff is needed; large PRs can return many files.",
		Parameters: map[string]*gollem.Parameter{
			"owner":         {Type: gollem.TypeString, Description: "Repository owner.", Required: true},
			"repo":          {Type: gollem.TypeString, Description: "Repository name.", Required: true},
			"number":        {Type: gollem.TypeInteger, Description: "Pull request number.", Required: true},
			"include_files": {Type: gollem.TypeBoolean, Description: "When true, include the changed files with status/additions/deletions/patch. Default false.", Required: false},
		},
	}
}

func (t *getPullRequestTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	if owner == "" || repo == "" {
		return nil, goerr.New("owner and repo are required")
	}
	number, err := tool.ExtractInt64(args, "number")
	if err != nil {
		return nil, err
	}
	includeFiles, _ := args["include_files"].(bool)

	tool.Update(ctx, fmt.Sprintf("Fetching GitHub PR %s/%s#%d", owner, repo, number))

	pr, err := t.client.GetPullRequestDetail(ctx, owner, repo, int(number), includeFiles)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get pull request")
	}

	reviews := make([]map[string]any, 0, len(pr.Reviews))
	for _, r := range pr.Reviews {
		reviews = append(reviews, map[string]any{
			"author":     r.Author,
			"body":       r.Body,
			"state":      r.State,
			"created_at": r.CreatedAt.Format(time.RFC3339),
		})
	}

	out := map[string]any{
		"number":     pr.Number,
		"title":      pr.Title,
		"body":       pr.Body,
		"author":     pr.Author,
		"state":      pr.State,
		"url":        pr.URL,
		"labels":     pr.Labels,
		"merged":     pr.Merged,
		"draft":      pr.Draft,
		"base_ref":   pr.BaseRef,
		"head_ref":   pr.HeadRef,
		"created_at": pr.CreatedAt.Format(time.RFC3339),
		"updated_at": pr.UpdatedAt.Format(time.RFC3339),
		"comments":   commentsToMap(pr.Comments),
		"reviews":    reviews,
	}
	if pr.ClosedAt != nil {
		out["closed_at"] = pr.ClosedAt.Format(time.RFC3339)
	}
	if pr.Files != nil {
		files := make([]map[string]any, 0, len(pr.Files))
		for _, f := range pr.Files {
			files = append(files, map[string]any{
				"path":            f.Path,
				"status":          f.Status,
				"additions":       f.Additions,
				"deletions":       f.Deletions,
				"patch":           f.Patch,
				"patch_truncated": f.PatchTruncated,
			})
		}
		out["files"] = files
	}
	return out, nil
}

// === github__get_file ===

type getFileTool struct {
	client toolClient
}

func (t *getFileTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "github__get_file",
		Description: "Fetch a file's content at a specific revision. Returns up to 1 MB of UTF-8 text; binary files return is_binary=true with empty content. Truncation is reported via the 'truncated' flag.",
		Parameters: map[string]*gollem.Parameter{
			"owner": {Type: gollem.TypeString, Description: "Repository owner.", Required: true},
			"repo":  {Type: gollem.TypeString, Description: "Repository name.", Required: true},
			"path":  {Type: gollem.TypeString, Description: "Path within the repository (no leading slash).", Required: true},
			"ref":   {Type: gollem.TypeString, Description: "Branch name, tag, or commit SHA. Empty defaults to the repository's default branch.", Required: false},
		},
	}
}

func (t *getFileTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	path, _ := args["path"].(string)
	if owner == "" || repo == "" || path == "" {
		return nil, goerr.New("owner, repo, and path are required")
	}
	ref, _ := args["ref"].(string)

	tool.Update(ctx, fmt.Sprintf("Fetching %s/%s:%s", owner, repo, path))

	res, err := t.client.GetFileContent(ctx, owner, repo, path, ref)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get file content")
	}
	return map[string]any{
		"path":      res.Path,
		"ref":       res.Ref,
		"size":      res.Size,
		"content":   res.Content,
		"truncated": res.Truncated,
		"is_binary": res.IsBinary,
	}, nil
}

// === github__list_commits ===

type listCommitsTool struct {
	client toolClient
}

func (t *listCommitsTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "github__list_commits",
		Description: "List commits in a repository with optional filters (path, author, since, until). Returns up to 50 commits per call.",
		Parameters: map[string]*gollem.Parameter{
			"owner":    {Type: gollem.TypeString, Description: "Repository owner.", Required: true},
			"repo":     {Type: gollem.TypeString, Description: "Repository name.", Required: true},
			"ref":      {Type: gollem.TypeString, Description: "Branch, tag, or SHA. Empty defaults to the repository's default branch.", Required: false},
			"path":     {Type: gollem.TypeString, Description: "Restrict to commits touching this path.", Required: false},
			"author":   {Type: gollem.TypeString, Description: "GitHub login or email of the commit author.", Required: false},
			"since":    {Type: gollem.TypeString, Description: "RFC3339 lower bound on commit time (inclusive).", Required: false},
			"until":    {Type: gollem.TypeString, Description: "RFC3339 upper bound on commit time (exclusive).", Required: false},
			"per_page": {Type: gollem.TypeInteger, Description: "Number of commits to return (1-50, default 20).", Required: false},
		},
	}
}

func (t *listCommitsTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	if owner == "" || repo == "" {
		return nil, goerr.New("owner and repo are required")
	}

	opts := ListCommitsOptions{Owner: owner, Repo: repo}
	if s, ok := args["ref"].(string); ok {
		opts.Ref = s
	}
	if s, ok := args["path"].(string); ok {
		opts.Path = s
	}
	if s, ok := args["author"].(string); ok {
		opts.Author = s
	}
	if s, ok := args["since"].(string); ok && s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, goerr.Wrap(err, "invalid since (must be RFC3339)", goerr.V("since", s))
		}
		opts.Since = t
	}
	if s, ok := args["until"].(string); ok && s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, goerr.Wrap(err, "invalid until (must be RFC3339)", goerr.V("until", s))
		}
		opts.Until = t
	}
	if v, err := tool.ExtractInt64(args, "per_page"); err == nil && v > 0 {
		opts.PerPage = int(v)
	}

	tool.Update(ctx, fmt.Sprintf("Listing commits in %s/%s", owner, repo))

	res, err := t.client.ListCommits(ctx, opts)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list commits")
	}

	items := make([]map[string]any, 0, len(res.Items))
	for _, c := range res.Items {
		item := map[string]any{
			"sha":            c.SHA,
			"author_login":   c.AuthorLogin,
			"author_name":    c.AuthorName,
			"author_email":   c.AuthorEmail,
			"authored_date":  c.AuthoredDate.Format(time.RFC3339),
			"committer_date": c.CommitterDate.Format(time.RFC3339),
			"message":        c.Message,
			"url":            c.URL,
		}
		items = append(items, item)
	}
	return map[string]any{"items": items}, nil
}

// commentsToMap renders a slice of Comment values as gollem-friendly maps.
func commentsToMap(in []Comment) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, c := range in {
		out = append(out, map[string]any{
			"author":     c.Author,
			"body":       c.Body,
			"created_at": c.CreatedAt.Format(time.RFC3339),
			"url":        c.URL,
		})
	}
	return out
}
