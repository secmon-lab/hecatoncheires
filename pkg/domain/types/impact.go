package types

import (
	"github.com/m-mizutani/goerr/v2"
)

// ImpactID represents a unique identifier for an impact level
type ImpactID string

// Validate checks if the ImpactID is valid
func (i ImpactID) Validate() error {
	if i == "" {
		return goerr.New("impact ID cannot be empty")
	}
	if !idPattern.MatchString(string(i)) {
		return goerr.New("impact ID must be lowercase alphanumeric with hyphens", goerr.V("id", i))
	}
	return nil
}

// String returns the string representation of ImpactID
func (i ImpactID) String() string {
	return string(i)
}
