package notiontool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jomei/notionapi"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/safe"
)

// notionHTTPTimeout caps every Notion API request. Without it, http.DefaultClient
// would let goroutines hang indefinitely if Notion stops responding mid-stream.
const notionHTTPTimeout = 30 * time.Second

// Client provides agent-tool-scoped access to the Notion API. It wraps the
// notionapi search endpoint and adds the dedicated Markdown content endpoint
// (GET /v1/pages/{id}/markdown, Notion-Version 2026-03-11) which the
// jomei/notionapi library does not yet expose.
type Client interface {
	// Search performs a Notion-wide search via POST /v1/search.
	Search(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error)

	// GetPageMarkdown retrieves a page's content rendered as Notion-flavored Markdown.
	GetPageMarkdown(ctx context.Context, pageID string) (*PageMarkdown, error)
}

// markdownAPIVersion is the minimum Notion-Version that exposes the
// GET /v1/pages/{id}/markdown endpoint.
const markdownAPIVersion = "2026-03-11"

type client struct {
	api        *notionapi.Client
	token      string
	httpClient *http.Client
	apiBaseURL string
}

// NewClient constructs a Client backed by the Notion API. The token must have
// the integration's read_content capability and be shared with the pages /
// databases the agent should be able to surface.
func NewClient(token string) (Client, error) {
	if token == "" {
		return nil, goerr.New("Notion API token is required")
	}
	httpClient := &http.Client{Timeout: notionHTTPTimeout}
	return &client{
		api: notionapi.NewClient(
			notionapi.Token(token),
			notionapi.WithRetry(3),
			notionapi.WithHTTPClient(httpClient),
		),
		token:      token,
		httpClient: httpClient,
		apiBaseURL: "https://api.notion.com",
	}, nil
}

// Search performs a Notion-wide search via POST /v1/search and converts the response.
func (c *client) Search(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	req := &notionapi.SearchRequest{
		Query:    query,
		PageSize: pageSize,
	}
	if opts.FilterType != "" {
		req.Filter = notionapi.SearchFilter{
			Property: "object",
			Value:    opts.FilterType,
		}
	}
	if opts.SortByEdit != "" {
		req.Sort = &notionapi.SortObject{
			Timestamp: notionapi.TimestampType("last_edited_time"),
			Direction: notionapi.SortOrder(opts.SortByEdit),
		}
	}
	if opts.StartCursor != "" {
		req.StartCursor = notionapi.Cursor(opts.StartCursor)
	}

	resp, err := c.api.Search.Do(ctx, req)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to search notion",
			goerr.V("query", query),
			goerr.V("page_size", pageSize),
		)
	}

	out := &SearchResult{
		Items:      make([]SearchItem, 0, len(resp.Results)),
		HasMore:    resp.HasMore,
		NextCursor: string(resp.NextCursor),
	}
	for _, obj := range resp.Results {
		item, ok := convertSearchItem(obj)
		if !ok {
			continue
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// convertSearchItem converts a notionapi.Object (Page or Database) into a SearchItem.
// Returns false when the object type is not recognised.
func convertSearchItem(obj notionapi.Object) (SearchItem, bool) {
	switch v := obj.(type) {
	case *notionapi.Page:
		return SearchItem{
			ID:         v.ID.String(),
			Type:       "page",
			Title:      extractPageTitle(v),
			URL:        v.URL,
			LastEdited: v.LastEditedTime,
		}, true
	case *notionapi.Database:
		var title strings.Builder
		for _, rt := range v.Title {
			title.WriteString(rt.PlainText)
		}
		return SearchItem{
			ID:         v.ID.String(),
			Type:       "database",
			Title:      title.String(),
			URL:        v.URL,
			LastEdited: v.LastEditedTime,
		}, true
	default:
		return SearchItem{}, false
	}
}

// extractPageTitle pulls the first title-typed property's plain-text content
// out of a Notion page.
func extractPageTitle(page *notionapi.Page) string {
	for _, prop := range page.Properties {
		if title, ok := prop.(*notionapi.TitleProperty); ok {
			var sb strings.Builder
			for _, rt := range title.Title {
				sb.WriteString(rt.PlainText)
			}
			if sb.Len() > 0 {
				return sb.String()
			}
		}
	}
	return ""
}

// markdownResponse is the JSON shape returned by GET /v1/pages/{id}/markdown.
type markdownResponse struct {
	Markdown  string `json:"markdown"`
	Truncated bool   `json:"truncated"`
}

// GetPageMarkdown fetches a page's content as Notion-flavored Markdown via the
// dedicated endpoint introduced in API version 2026-03-11.
func (c *client) GetPageMarkdown(ctx context.Context, pageID string) (*PageMarkdown, error) {
	if pageID == "" {
		return nil, goerr.New("pageID is required")
	}

	// PathEscape: pageID arrives from LLM tool args, so guard against accidental
	// slashes / spaces / non-UUID characters that would break the URL or escape
	// the /v1/pages/ scope.
	endpoint := fmt.Sprintf("%s/v1/pages/%s/markdown", c.apiBaseURL, url.PathEscape(pageID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to build markdown request", goerr.V("pageID", pageID))
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Notion-Version", markdownAPIVersion)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to call notion markdown endpoint", goerr.V("pageID", pageID))
	}
	defer safe.Close(ctx, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, goerr.New("notion markdown endpoint returned non-2xx",
			goerr.V("pageID", pageID),
			goerr.V("status", resp.StatusCode),
			goerr.V("body", string(body)),
		)
	}

	var decoded markdownResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, goerr.Wrap(err, "failed to decode notion markdown response", goerr.V("pageID", pageID))
	}

	return &PageMarkdown{
		PageID:    pageID,
		Markdown:  decoded.Markdown,
		Truncated: decoded.Truncated,
	}, nil
}
