package slack

import (
	"github.com/m-mizutani/goerr/v2"
	goslack "github.com/slack-go/slack"
)

// Export internal functions and types for testing
var (
	// WithCacheTTL is exported for testing
	TestWithCacheTTL = WithCacheTTL

	// TruncateToMaxBytes is exported for testing UTF-8 truncation
	TruncateToMaxBytes = truncateToMaxBytes

	// WrapSlackViewErrorForTest exposes wrapSlackViewError so tests can
	// verify that SlackErrorResponse metadata is surfaced on goerr values.
	WrapSlackViewErrorForTest = wrapSlackViewError

	// ResolveDisplayNameForTest exposes resolveDisplayName so client_test.go
	// can verify the Profile.DisplayName → Profile.RealName → RealName
	// fallback order without hitting the Slack API.
	ResolveDisplayNameForTest = resolveDisplayName
)

// NewWithAPIURLForTest builds a Service backed by a slack-go client that
// points at apiURL instead of slack.com. The URL is appended with the
// method name (e.g. ".../conversations.invite"), so apiURL MUST end with
// a slash. Used to drive client.go behaviour against an httptest server.
func NewWithAPIURLForTest(token, apiURL string) (Service, error) {
	if token == "" {
		return nil, goerr.New("Slack bot token is required")
	}
	return &client{
		api:      goslack.New(token, goslack.OptionAPIURL(apiURL)),
		cacheTTL: DefaultCacheTTL,
		cache:    make(map[string]cacheEntry),
	}, nil
}
