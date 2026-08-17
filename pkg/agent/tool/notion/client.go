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
// jomei/notionapi library is used for its request types only. Responses are
// decoded by this package — see searchResponse for why.
type Client interface {
	// Search performs a Notion-wide search via POST /v1/search.
	Search(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error)

	// GetPageMarkdown retrieves a page's content rendered as Notion-flavored Markdown.
	GetPageMarkdown(ctx context.Context, pageID string) (*PageMarkdown, error)
}

// markdownAPIVersion is the minimum Notion-Version that exposes the
// GET /v1/pages/{id}/markdown endpoint.
const markdownAPIVersion = "2026-03-11"

// searchAPIVersion pins POST /v1/search to the response shape searchResponse is
// written against. Notion changes the payload between versions, so the pin is
// what keeps the decoder and the API in step.
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
		return nil, goerr.Wrap(err, "failed to call notion search endpoint",
			goerr.V("query", query),
			goerr.V("page_size", pageSize),
		)
	}
	defer safe.Close(ctx, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, newAPIError(ctx, "search", resp,
			goerr.V("query", query),
			goerr.V("page_size", pageSize),
		)
	}

	var decoded searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, goerr.Wrap(err, "failed to decode notion search response",
			goerr.V("query", query),
		)
	}

	out := &SearchResult{
		Items:      make([]SearchItem, 0, len(decoded.Results)),
		HasMore:    decoded.HasMore,
		NextCursor: decoded.NextCursor,
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

// apiErrorDetailLimit bounds how much of the upstream explanation is repeated in
// the error message. The whole body still travels as a goerr value; this is the
// part a model reads, and a long one pushes the rest of its context out.
const apiErrorDetailLimit = 512

// notionErrorBody is the error payload Notion returns on every non-2xx response.
type notionErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// newAPIError reports a non-2xx Notion response. The status and Notion's own
// error code are put IN the message rather than only in goerr values because
// that message is the whole of what the agent is told: a failed tool call is
// rendered into its function response as err.Error() (pkg/agent/toolcall), which
// drops goerr values. Told only "returned non-2xx", neither the model nor an
// operator reading the recorded event can tell a page that does not exist from
// one the integration cannot see, or either from a rate limit.
func newAPIError(ctx context.Context, endpoint string, resp *http.Response, opts ...goerr.Option) error {
	body := readErrorBody(ctx, resp.Body)

	var decoded notionErrorBody
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		// Not reported: a non-JSON body is a normal shape for an error served by
		// a proxy in front of Notion, and the raw prefix below carries it.
		decoded = notionErrorBody{}
	}

	msg := fmt.Sprintf("notion %s endpoint returned HTTP %d", endpoint, resp.StatusCode)
	if decoded.Code != "" {
		msg += " (" + decoded.Code + ")"
	}
	if detail := firstNonEmpty(decoded.Message, body); detail != "" {
		msg += ": " + truncate(detail, apiErrorDetailLimit)
	}

	return goerr.New(msg, append(opts,
		goerr.V("endpoint", endpoint),
		goerr.V("status", resp.StatusCode),
		goerr.V("code", decoded.Code),
		goerr.V("body", body),
	)...)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// truncate cuts s to at most limit runes. Runes rather than bytes: the cut text
// reaches the model, the run timeline and Sentry alike, and a byte-offset cut
// splits a multi-byte character.
func truncate(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "..."
}

// searchResponse decodes only what the search tool surfaces.
//
// jomei/notionapi's SearchResponse is deliberately not used: it decodes every
// property of every page through a closed switch over the property types the
// library knows (property.go, decodeProperty), and returns an error for any
// other one. A single page carrying a property type Notion added since — `place`
// in production — therefore failed the whole search, and the agent got nothing
// back for an otherwise valid query. Decoding just id / title / url / timestamp
// leaves an unknown property type harmless.
type searchResponse struct {
	Results    []searchObject `json:"results"`
	HasMore    bool           `json:"has_more"`
	NextCursor string         `json:"next_cursor"`
}

// searchObject is one entry of the search response: a page or a database.
type searchObject struct {
	Object         string    `json:"object"`
	ID             string    `json:"id"`
	URL            string    `json:"url"`
	LastEditedTime time.Time `json:"last_edited_time"`
	// Title is where a database carries its name.
	Title []richText `json:"title"`
	// Properties is where a page carries its name, under a user-defined key
	// whose value is the property typed "title". The values stay raw so that a
	// property type this code does not model cannot fail the decode.
	Properties map[string]json.RawMessage `json:"properties"`
}

type richText struct {
	PlainText string `json:"plain_text"`
}

// titleProperty is a page property narrowed to the title case.
type titleProperty struct {
	Type  string     `json:"type"`
	Title []richText `json:"title"`
}

// convertSearchItem converts one search result into a SearchItem. Returns false
// when the object is neither a page nor a database.
func convertSearchItem(obj searchObject) (SearchItem, bool) {
	switch obj.Object {
	case "page":
		return SearchItem{
			ID:         obj.ID,
			Type:       "page",
			Title:      extractPageTitle(obj.Properties),
			URL:        obj.URL,
			LastEdited: obj.LastEditedTime,
		}, true
	case "database":
		return SearchItem{
			ID:         obj.ID,
			Type:       "database",
			Title:      plainText(obj.Title),
			URL:        obj.URL,
			LastEdited: obj.LastEditedTime,
		}, true
	default:
		return SearchItem{}, false
	}
}

// extractPageTitle pulls the plain text of the page's title property. A property
// that does not decode is skipped rather than failing the item: the page is
// still worth returning without its title.
func extractPageTitle(props map[string]json.RawMessage) string {
	for _, raw := range props {
		var prop titleProperty
		if err := json.Unmarshal(raw, &prop); err != nil {
			continue
		}
		if prop.Type != "title" {
			continue
		}
		if title := plainText(prop.Title); title != "" {
			return title
		}
	}
	return ""
}

func plainText(parts []richText) string {
	var sb strings.Builder
	for _, rt := range parts {
		sb.WriteString(rt.PlainText)
	}
	return sb.String()
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
		return nil, newAPIError(ctx, "markdown", resp, goerr.V("pageID", pageID))
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
