package slack

// Export internal functions and types for testing
var (
	// WithCacheTTL is exported for testing
	TestWithCacheTTL = WithCacheTTL

	// TruncateToMaxBytes is exported for testing UTF-8 truncation
	TruncateToMaxBytes = truncateToMaxBytes
)
