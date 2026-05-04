package slacktool

import (
	"context"
	"net/http"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/slack-go/slack"
)

// slackHTTPTimeout caps every Slack search.messages call. The default
// http.Client used by slack.New has no timeout, which can hang goroutines
// indefinitely if Slack stops responding mid-stream.
const slackHTTPTimeout = 30 * time.Second

// SearchService provides search-related Slack API operations using a User OAuth Token.
// The search.messages API requires the search:read user-token scope and cannot be
// called with a Bot token, so it is intentionally separate from pkg/service/slack
// (which only ever holds bot-token operations).
type SearchService interface {
	// SearchMessages searches messages across the workspace using the search.messages API.
	SearchMessages(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error)
}

// SearchOptions configures a SearchMessages call.
type SearchOptions struct {
	// Count is the number of results per page. Clamped to [1, 100]. Defaults to 20 when zero.
	Count int
	// Sort is the result ordering. Either "score" or "timestamp". Defaults to "score".
	Sort string
	// SortDir is the sort direction. Either "asc" or "desc". Defaults to "desc".
	SortDir string
}

// SearchResult is the response of SearchMessages.
type SearchResult struct {
	Total    int
	Messages []SearchMessage
}

// SearchMessage is a single matched message in the search response.
type SearchMessage struct {
	ChannelID   string
	ChannelName string
	UserID      string
	Username    string
	Text        string
	Timestamp   string
	Permalink   string
}

const (
	defaultSearchCount = 20
	maxSearchCount     = 100
	defaultSearchSort  = "score"
	defaultSortDir     = "desc"
)

// searchClient implements SearchService using a Slack User OAuth Token.
type searchClient struct {
	api *slack.Client
}

// NewSearchClient creates a SearchService using the provided Slack User OAuth Token.
// The token must have the search:read scope.
func NewSearchClient(userToken string) (SearchService, error) {
	if userToken == "" {
		return nil, goerr.New("Slack User OAuth Token is required for SearchService")
	}
	return &searchClient{
		api: slack.New(userToken, slack.OptionHTTPClient(&http.Client{Timeout: slackHTTPTimeout})),
	}, nil
}

// SearchMessages calls the search.messages API and converts the response.
func (c *searchClient) SearchMessages(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	if query == "" {
		return nil, goerr.New("query is required")
	}

	count := opts.Count
	if count <= 0 {
		count = defaultSearchCount
	}
	if count > maxSearchCount {
		count = maxSearchCount
	}
	sort := opts.Sort
	if sort == "" {
		sort = defaultSearchSort
	}
	sortDir := opts.SortDir
	if sortDir == "" {
		sortDir = defaultSortDir
	}

	params := slack.SearchParameters{
		Sort:          sort,
		SortDirection: sortDir,
		Highlight:     false,
		Count:         count,
		Page:          1,
	}
	resp, err := c.api.SearchMessagesContext(ctx, query, params)
	if err != nil {
		opts := []goerr.Option{
			goerr.V("query", query),
			goerr.V("count", count),
		}
		opts = append(opts, slackErrorAttrs(err)...)
		return nil, goerr.Wrap(err, "failed to search slack messages", opts...)
	}

	out := &SearchResult{
		Total:    resp.Total,
		Messages: make([]SearchMessage, 0, len(resp.Matches)),
	}
	for _, m := range resp.Matches {
		out.Messages = append(out.Messages, SearchMessage{
			ChannelID:   m.Channel.ID,
			ChannelName: m.Channel.Name,
			UserID:      m.User,
			Username:    m.Username,
			Text:        m.Text,
			Timestamp:   m.Timestamp,
			Permalink:   m.Permalink,
		})
	}
	return out, nil
}
