package types

import (
	"regexp"

	"github.com/m-mizutani/goerr/v2"
)

// CategoryID represents a unique identifier for a risk category
type CategoryID string

var idPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Validate checks if the CategoryID is valid
func (c CategoryID) Validate() error {
	if c == "" {
		return goerr.New("category ID cannot be empty")
	}
	if !idPattern.MatchString(string(c)) {
		return goerr.New("category ID must be lowercase alphanumeric with hyphens", goerr.V("id", c))
	}
	return nil
}

// String returns the string representation of CategoryID
func (c CategoryID) String() string {
	return string(c)
}
