package slack

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
