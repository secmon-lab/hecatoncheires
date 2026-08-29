package interfaces

import "github.com/secmon-lab/hecatoncheires/pkg/domain/types"

// ListCaseOption is a functional option for filtering cases in List
type ListCaseOption func(*listCaseConfig)

type listCaseConfig struct {
	status       *types.CaseStatus
	archiveScope CaseArchiveScope
}

// CaseArchiveScope selects which slice of a case list to return.
type CaseArchiveScope int

const (
	// CaseArchiveScopeActiveOnly returns only non-archived cases. This is the
	// zero value, so a caller that names no scope never sees archived cases —
	// the board, the dashboard, the Job scanner, the agent and MCP tools and
	// the case_ref picker all rely on that default rather than each
	// remembering to exclude them.
	CaseArchiveScopeActiveOnly CaseArchiveScope = iota
	// CaseArchiveScopeArchivedOnly returns only archived cases.
	CaseArchiveScopeArchivedOnly
	// CaseArchiveScopeAll returns both active and archived cases. Intended for
	// inventory passes (the BigQuery export), not for user-facing listings.
	CaseArchiveScopeAll
)

// Allows reports whether a case with the given archived state passes this
// scope's filter.
func (s CaseArchiveScope) Allows(isArchived bool) bool {
	switch s {
	case CaseArchiveScopeArchivedOnly:
		return isArchived
	case CaseArchiveScopeAll:
		return true
	default: // CaseArchiveScopeActiveOnly
		return !isArchived
	}
}

// WithStatus filters cases by status
func WithStatus(status types.CaseStatus) ListCaseOption {
	return func(c *listCaseConfig) {
		c.status = &status
	}
}

// WithArchiveScope selects active / archived / both. Without it the zero value
// (active only) applies.
func WithArchiveScope(scope CaseArchiveScope) ListCaseOption {
	return func(c *listCaseConfig) {
		c.archiveScope = scope
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

// ArchiveScope returns the archive slice the caller asked for.
func (c *listCaseConfig) ArchiveScope() CaseArchiveScope {
	return c.archiveScope
}
