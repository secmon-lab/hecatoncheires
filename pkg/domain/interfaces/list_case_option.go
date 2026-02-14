package interfaces

import "github.com/secmon-lab/hecatoncheires/pkg/domain/types"

// ListCaseOption is a functional option for filtering cases in List
type ListCaseOption func(*listCaseConfig)

type listCaseConfig struct {
	status *types.CaseStatus
}

// WithStatus filters cases by status
func WithStatus(status types.CaseStatus) ListCaseOption {
	return func(c *listCaseConfig) {
		c.status = &status
	}
}

// BuildListCaseConfig builds a listCaseConfig from options
func BuildListCaseConfig(opts ...ListCaseOption) *listCaseConfig {
	cfg := &listCaseConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// Status returns the status filter value, or nil if not set
func (c *listCaseConfig) Status() *types.CaseStatus {
	return c.status
}
