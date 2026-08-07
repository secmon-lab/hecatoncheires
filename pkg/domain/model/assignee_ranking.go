package model

import (
	"slices"
	"time"

	"github.com/m-mizutani/goerr/v2"
)

// ErrAssigneeRankingValidation is returned when an AssigneeRanking fails
// validation.
var ErrAssigneeRankingValidation = goerr.New("assignee ranking validation failed")

// AssigneeRanking orders the Slack users of one workspace by how often they
// are assigned to its Cases, so the WebUI assignee picker can float the people
// who actually take work there to the top. One document per workspace, keyed
// by WorkspaceID.
//
// It is derived data recomputed from the Case collection: losing it costs a
// recompute, nothing more. The order is a hint for a picker, not a statistic —
// it is served from this cache and lags the underlying Cases by up to the
// caller's freshness window.
type AssigneeRanking struct {
	// WorkspaceID is the workspace the ranking belongs to. Document key.
	// Required.
	WorkspaceID string
	// UserIDs are the Slack User IDs in ranking order, most assigned first.
	// May be empty (no one has been assigned anything yet); no element may be
	// an empty string.
	UserIDs []string
	// ComputedAt is when UserIDs was produced. The zero value means the
	// ranking has never been computed, which callers treat as "stale".
	ComputedAt time.Time
}

// IsFresh reports whether the ranking was computed within window before now.
// A never-computed ranking is never fresh, and the boundary (now-ComputedAt
// exactly window) counts as stale so a refresh is not deferred another window.
func (r *AssigneeRanking) IsFresh(now time.Time, window time.Duration) bool {
	if r == nil || r.ComputedAt.IsZero() {
		return false
	}
	return now.Sub(r.ComputedAt) < window
}

// Validate enforces the invariants the repository relies on before every
// write.
func (r *AssigneeRanking) Validate() error {
	if r == nil {
		return goerr.Wrap(ErrAssigneeRankingValidation, "assignee ranking is nil")
	}
	if r.WorkspaceID == "" {
		return goerr.Wrap(ErrAssigneeRankingValidation, "workspace id is required")
	}
	if slices.Contains(r.UserIDs, "") {
		return goerr.Wrap(ErrAssigneeRankingValidation, "user id must not be empty",
			goerr.V("workspace_id", r.WorkspaceID))
	}
	return nil
}
