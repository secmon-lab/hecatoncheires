package notiontool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jomei/notionapi"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/safe"
)

// notionHTTPTimeout caps every Notion API request. Without it, http.DefaultClient
// would let goroutines hang indefinitely if Notion stops responding mid-stream.
const notionHTTPTimeout = 30 * time.Second

// Client provides agent-tool-scoped access to the Notion API. It calls the
// search endpoint and the dedicated Markdown content endpoint
// (GET /v1/pages/{id}/markdown, Notion-Version 2026-03-11) directly; the
// jomei/notionapi library is used for its request/response types only.
type Client interface {
	// Search performs a Notion-wide search via POST /v1/search.
	Search(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error)

	// GetPageMarkdown retrieves a page's content rendered as Notion-flavored Markdown.
	GetPageMarkdown(ctx context.Context, pageID string) (*PageMarkdown, error)
}

// markdownAPIVersion is the minimum Notion-Version that exposes the
// GET /v1/pages/{id}/markdown endpoint.
const markdownAPIVersion = "2026-03-11"

// searchAPIVersion pins POST /v1/search to the version jomei/notionapi is
// written against, so notionapi.SearchResponse still decodes the payload.
const searchAPIVersion = "2022-06-28"

// notionAPIMaxAttempts bounds how many times a request is sent when Notion
// answers 429. It stands in for notionapi.WithRetry(3): the library re-sent the
// same *http.Request whose body had already been consumed, so a retried POST
// carried an empty body — rebuilding the request per attempt avoids that.
const notionAPIMaxAttempts = 3

// defaultRetryAfter applies when a 429 carries no usable Retry-After header.
const defaultRetryAfter = time.Second

// errorBodyLimit bounds how much of a response body is read for diagnostics and
// how much of a 429 body is drained before the connection is reused.
const errorBodyLimit = 4096

type client struct {
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
	return &client{
		token:      token,
		httpClient: &http.Client{Timeout: notionHTTPTimeout},
		apiBaseURL: "https://api.notion.com",
	}, nil
}

// searchRequest is the POST /v1/search body. It exists instead of
// notionapi.SearchRequest because that type declares Filter as a non-pointer
// struct whose fields carry no omitempty (the struct-level omitempty is a no-op
// in encoding/json), so an unfiltered search serialised as
// "filter":{"value":"","property":""} and Notion rejected the whole request with
// 400 `body.filter.property should be "object", instead was ""`. A pointer lets
// the key be omitted, which is how "search pages and databases" is expressed.
type searchRequest struct {
	Query       string                  `json:"query,omitempty"`
	Sort        *notionapi.SortObject   `json:"sort,omitempty"`
	Filter      *notionapi.SearchFilter `json:"filter,omitempty"`
	StartCursor string                  `json:"start_cursor,omitempty"`
	PageSize    int                     `json:"page_size,omitempty"`
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

	body := &searchRequest{
		Query:       query,
		StartCursor: opts.StartCursor,
		PageSize:    pageSize,
	}
	if opts.FilterType != "" {
		body.Filter = &notionapi.SearchFilter{
			Property: "object",
			Value:    opts.FilterType,
		}
	}
	if opts.SortByEdit != "" {
		body.Sort = &notionapi.SortObject{
			Timestamp: notionapi.TimestampType("last_edited_time"),
			Direction: notionapi.SortOrder(opts.SortByEdit),
		}
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to encode notion search request",
			goerr.V("query", query),
		)
	}

	resp, err := c.doJSON(ctx, http.MethodPost, "/v1/search", searchAPIVersion, encoded)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to search notion",
			goerr.V("query", query),
			goerr.V("page_size", pageSize),
		)
	}
	defer safe.Close(ctx, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, goerr.New("notion search endpoint returned non-2xx",
			goerr.V("query", query),
			goerr.V("page_size", pageSize),
			goerr.V("status", resp.StatusCode),
			goerr.V("body", readErrorBody(ctx, resp.Body)),
		)
	}

	var decoded notionapi.SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, goerr.Wrap(err, "failed to decode notion search response",
			goerr.V("query", query),
		)
	}

	out := &SearchResult{
		Items:      make([]SearchItem, 0, len(decoded.Results)),
		HasMore:    decoded.HasMore,
		NextCursor: string(decoded.NextCursor),
	}
	for _, obj := range decoded.Results {
		item, ok := convertSearchItem(obj)
		if !ok {
			continue
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// doJSON sends one Notion API request, retrying while Notion answers 429. The
// request is rebuilt on every attempt so the retry carries the same payload.
func (c *client) doJSON(ctx context.Context, method, path, notionVersion string, body []byte) (*http.Response, error) {
	endpoint := c.apiBaseURL + path

	for attempt := 1; ; attempt++ {
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to build notion request", goerr.V("path", path))
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Notion-Version", notionVersion)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to call notion api", goerr.V("path", path))
		}
		if resp.StatusCode != http.StatusTooManyRequests || attempt >= notionAPIMaxAttempts {
			return resp, nil
		}

		wait := retryAfter(resp.Header.Get("Retry-After"))
		// Drain before closing: net/http can only reuse a connection whose body
		// was read to EOF, and reconnecting on every attempt makes a rate limit
		// more expensive to recover from than it needs to be.
		safe.Copy(ctx, io.Discard, io.LimitReader(resp.Body, errorBodyLimit))
		safe.Close(ctx, resp.Body)

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, goerr.Wrap(ctx.Err(), "notion api retry aborted", goerr.V("path", path))
		case <-timer.C:
		}
	}
}

// retryAfter reads Notion's Retry-After header, which carries whole seconds.
// Notion always sends it on 429, but a missing or unparseable value falls back
// to a fixed wait rather than failing the call: for an agent tool, one more
// attempt is a better outcome than surfacing a rate limit as a hard error.
func retryAfter(header string) time.Duration {
	sec, err := strconv.Atoi(header)
	if err != nil || sec < 0 {
		return defaultRetryAfter
	}
	return time.Duration(sec) * time.Second
}

// readErrorBody reads a bounded prefix of a non-2xx body for diagnostics. A read
// failure is non-fatal — the caller is already returning the HTTP error — so it
// is reported rather than returned.
func readErrorBody(ctx context.Context, body io.Reader) string {
	raw, err := io.ReadAll(io.LimitReader(body, errorBodyLimit))
	if err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to read notion error body"),
			"failed to read notion error body")
		return ""
	}
	return string(raw)
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
	path := fmt.Sprintf("/v1/pages/%s/markdown", url.PathEscape(pageID))

	resp, err := c.doJSON(ctx, http.MethodGet, path, markdownAPIVersion, nil)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to call notion markdown endpoint", goerr.V("pageID", pageID))
	}
	defer safe.Close(ctx, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, goerr.New("notion markdown endpoint returned non-2xx",
			goerr.V("pageID", pageID),
			goerr.V("status", resp.StatusCode),
			goerr.V("body", readErrorBody(ctx, resp.Body)),
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
