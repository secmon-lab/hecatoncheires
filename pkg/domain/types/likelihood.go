package types

import (
	"github.com/m-mizutani/goerr/v2"
)

// LikelihoodID represents a unique identifier for a likelihood level
type LikelihoodID string

// Validate checks if the LikelihoodID is valid
func (l LikelihoodID) Validate() error {
	if l == "" {
		return goerr.New("likelihood ID cannot be empty")
	}
	if !idPattern.MatchString(string(l)) {
		return goerr.New("likelihood ID must be lowercase alphanumeric with hyphens", goerr.V("id", l))
	}
	return nil
}

// String returns the string representation of LikelihoodID
func (l LikelihoodID) String() string {
	return string(l)
}
