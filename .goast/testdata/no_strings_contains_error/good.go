package sample

import (
	"errors"
	"strings"
)

var errSentinel = errors.New("sentinel")

// Substring checks on ordinary strings and typed error checks are both fine.
func good(err error, s string) bool {
	if strings.Contains(s, "needle") {
		return true
	}
	return errors.Is(err, errSentinel)
}
