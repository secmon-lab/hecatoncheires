package types

import (
	"github.com/m-mizutani/goerr/v2"
)

// TeamID represents a unique identifier for a team
type TeamID string

// Validate checks if the TeamID is valid
func (t TeamID) Validate() error {
	if t == "" {
		return goerr.New("team ID cannot be empty")
	}
	if !idPattern.MatchString(string(t)) {
		return goerr.New("team ID must be lowercase alphanumeric with hyphens", goerr.V("id", t))
	}
	return nil
}

// String returns the string representation of TeamID
func (t TeamID) String() string {
	return string(t)
}
