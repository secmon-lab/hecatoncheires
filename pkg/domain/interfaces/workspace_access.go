package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// WorkspaceAuthorizer decides whether a Slack user may access a workspace.
// A workspace without an authorization policy is accessible to everyone.
type WorkspaceAuthorizer interface {
	// Authorize returns nil when slackUserID may access workspaceID, an error
	// wrapping model.ErrWorkspaceAccessDenied when the policy denies it, and any
	// other error when the decision could not be made.
	Authorize(ctx context.Context, workspaceID, slackUserID string) error
	// AuthorizeCurrentUser applies Authorize to the auth token's Sub. It returns
	// nil when ctx carries no token (a system context).
	AuthorizeCurrentUser(ctx context.Context, workspaceID string) error
	// FilterAccessible returns the entries slackUserID may access, in the input order.
	FilterAccessible(ctx context.Context, entries []*model.WorkspaceEntry, slackUserID string) ([]*model.WorkspaceEntry, error)
	// FilterAccessibleForCurrentUser applies FilterAccessible to the auth token's
	// Sub, and returns entries unchanged when ctx carries no token.
	FilterAccessibleForCurrentUser(ctx context.Context, entries []*model.WorkspaceEntry) ([]*model.WorkspaceEntry, error)
}
