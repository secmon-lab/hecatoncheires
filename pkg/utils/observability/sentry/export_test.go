package sentry

// SetEnabledForTest forces the package-level enabled flag. Tests that wire
// up their own Sentry transport via sentrygo.Init still need to flip this
// flag so Capture / HTTPMiddleware take their non-no-op path.
func SetEnabledForTest(v bool) { enabled.Store(v) }
