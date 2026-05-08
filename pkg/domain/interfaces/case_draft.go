package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// CaseDraftRepository persists workspace-agnostic Case drafts that are created
// when a user mentions the bot in a non-Case Slack channel. A draft holds
// collected source material plus the current AI materialization for the
// selected workspace; switching workspace overwrites the materialization.
type CaseDraftRepository interface {
	// Save creates or fully overwrites a draft.
	Save(ctx context.Context, draft *model.CaseDraft) error

	// Get retrieves a draft by ID. Returns ErrNotFound when missing.
	// Implementations may return ErrNotFound for expired drafts.
	Get(ctx context.Context, id model.CaseDraftID) (*model.CaseDraft, error)

	// SetMaterialization updates the SelectedWorkspaceID, Materialization, and
	// InferenceInProgress fields atomically. Other fields are left untouched.
	// Pass m=nil with inProgress=true to mark inference as started.
	SetMaterialization(
		ctx context.Context,
		id model.CaseDraftID,
		workspaceID string,
		m *model.WorkspaceMaterialization,
		inProgress bool,
	) error

	// Delete removes the draft.
	Delete(ctx context.Context, id model.CaseDraftID) error
}
