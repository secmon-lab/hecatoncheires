package http

// Export private functions for testing
var (
	VerifySlackSignature       = verifySlackSignature
	ValidateReturnToForTest    = validateReturnTo
	AuthLoginHandlerForTest    = authLoginHandler
	AuthCallbackHandlerForTest = authCallbackHandler
	WorkspacesHandlerForTest   = workspacesHandler
)

// ReturnToCookieNameForTest exposes the cookie name so tests can assert
// without duplicating the literal.
const ReturnToCookieNameForTest = returnToCookieName
