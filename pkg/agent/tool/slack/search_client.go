package slacktool

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/slack-go/slack"

	slackservice "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

// slackHTTPTimeout caps every Slack User-token API call. The default
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

// MessageRetriever fetches channel messages using a User OAuth Token.
//
// With a User token, conversations.history / conversations.replies can read
// public channels the user is in OR any public channel even without
// membership (Slack docs: "Only user tokens can access public channels they
// are not in"). Bot tokens require the bot to be a channel member, which
// produces the not_in_channel error when the bot hasn't been invited.
//
// Private channels still require user membership in both token types, so
// this interface does not change behaviour there — the underlying scope
// added by this interface is channels:history only.
type MessageRetriever interface {
	// GetConversationReplies fetches replies for a thread root.
	GetConversationReplies(ctx context.Context, channelID, threadTS string, limit int) ([]slackservice.ConversationMessage, error)
	// GetConversationHistory fetches channel messages newer than oldest.
	GetConversationHistory(ctx context.Context, channelID string, oldest time.Time, limit int) ([]slackservice.ConversationMessage, error)
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
	return newUserClient(userToken)
}

// NewMessageRetriever creates a MessageRetriever using the provided Slack User
// OAuth Token. The token must have the channels:history scope for public-
// channel reads. Private channels still require the user to be a member of
// the channel; the channels:history scope alone does not relax that.
func NewMessageRetriever(userToken string) (MessageRetriever, error) {
	if userToken == "" {
		return nil, goerr.New("Slack User OAuth Token is required for MessageRetriever")
	}
	return newUserClient(userToken)
}

// newUserClient builds a searchClient backed by a User-token slack.Client. Both
// NewSearchClient and NewMessageRetriever funnel through here so the HTTP
// timeout / scope-capture wrapping stays in one place.
func newUserClient(userToken string) (*searchClient, error) {
	httpClient := &capturingHTTPClient{
		inner: &http.Client{Timeout: slackHTTPTimeout},
	}
	return &searchClient{
		api: slack.New(userToken, slack.OptionHTTPClient(httpClient)),
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
	captureCtx, capture := contextWithScopeCapture(ctx)
	resp, err := c.api.SearchMessagesContext(captureCtx, query, params)
	if err != nil {
		opts := []goerr.Option{
			goerr.V("query", query),
			goerr.V("count", count),
		}
		opts = append(opts, slackErrorAttrs(err)...)
		if capture.needed != "" {
			opts = append(opts, goerr.V("slack_needed_scope", capture.needed))
		}
		if capture.provided != "" {
			opts = append(opts, goerr.V("slack_provided_scope", capture.provided))
		}
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

// GetConversationReplies fetches the replies for a thread using the User token.
// Public channels are readable without bot membership; private channels still
// require user membership (Slack-side constraint).
func (c *searchClient) GetConversationReplies(ctx context.Context, channelID, threadTS string, limit int) ([]slackservice.ConversationMessage, error) {
	params := &slack.GetConversationRepliesParameters{
		ChannelID: channelID,
		Timestamp: threadTS,
		Limit:     limit,
	}

	captureCtx, capture := contextWithScopeCapture(ctx)
	msgs, _, _, err := c.api.GetConversationRepliesContext(captureCtx, params)
	if err != nil {
		opts := []goerr.Option{
			goerr.V("channel_id", channelID),
			goerr.V("thread_ts", threadTS),
			goerr.V("limit", limit),
		}
		opts = append(opts, slackErrorAttrs(err)...)
		if capture.needed != "" {
			opts = append(opts, goerr.V("slack_needed_scope", capture.needed))
		}
		if capture.provided != "" {
			opts = append(opts, goerr.V("slack_provided_scope", capture.provided))
		}
		return nil, goerr.Wrap(err, "failed to get conversation replies", opts...)
	}

	return toConversationMessages(msgs), nil
}

// GetConversationHistory fetches channel messages newer than oldest using the
// User token. Same membership semantics as GetConversationReplies.
func (c *searchClient) GetConversationHistory(ctx context.Context, channelID string, oldest time.Time, limit int) ([]slackservice.ConversationMessage, error) {
	params := &slack.GetConversationHistoryParameters{
		ChannelID: channelID,
		Oldest:    fmt.Sprintf("%d.000000", oldest.Unix()),
		Limit:     limit,
	}

	captureCtx, capture := contextWithScopeCapture(ctx)
	resp, err := c.api.GetConversationHistoryContext(captureCtx, params)
	if err != nil {
		opts := []goerr.Option{
			goerr.V("channel_id", channelID),
			goerr.V("oldest", oldest),
			goerr.V("limit", limit),
		}
		opts = append(opts, slackErrorAttrs(err)...)
		if capture.needed != "" {
			opts = append(opts, goerr.V("slack_needed_scope", capture.needed))
		}
		if capture.provided != "" {
			opts = append(opts, goerr.V("slack_provided_scope", capture.provided))
		}
		return nil, goerr.Wrap(err, "failed to get conversation history", opts...)
	}

	return toConversationMessages(resp.Messages), nil
}

// toConversationMessages converts slack-go messages to our ConversationMessage.
// Used by both GetConversationReplies and GetConversationHistory.
func toConversationMessages(msgs []slack.Message) []slackservice.ConversationMessage {
	result := make([]slackservice.ConversationMessage, 0, len(msgs))
	for _, msg := range msgs {
		result = append(result, slackservice.ConversationMessage{
			UserID:    msg.User,
			UserName:  msg.Username,
			Text:      msg.Text,
			Timestamp: msg.Timestamp,
			ThreadTS:  msg.ThreadTimestamp,
		})
	}
	return result
}
