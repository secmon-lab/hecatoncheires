package slack

import "context"

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

// SetChannelInfoFetcherForTest swaps the per-channel info fetcher on a
// Service produced by New. Tests use this to drive GetChannelNames
// without a live Slack workspace (the production fetcher closes over
// slack.Client.GetConversationInfoContext). Returns the previous
// fetcher so a test can restore it if needed.
func SetChannelInfoFetcherForTest(svc Service, f func(ctx context.Context, id string) (string, error)) func(ctx context.Context, id string) (string, error) {
	c := svc.(*client)
	prev := c.fetchChannelInfo
	c.fetchChannelInfo = f
	return prev
}

// ChannelInfoParallelismForTest reports the active fetch parallelism so
// tests can assert that WithChannelInfoParallelism is honoured.
func ChannelInfoParallelismForTest(svc Service) int {
	return svc.(*client).channelInfoParallelism
}
