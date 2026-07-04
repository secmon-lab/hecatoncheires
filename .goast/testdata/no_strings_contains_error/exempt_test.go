package sample

import "strings"

// The same violation inside a _test.go file is exempt: test assertions on a
// third-party error's message have no typed sentinel to match against.
func assertErr(err error) bool {
	return strings.Contains(err.Error(), "missing_scope")
}
