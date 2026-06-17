package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// MemoArchiveScope selects which slice of a memo list to return.
type MemoArchiveScope int

const (
	// MemoArchiveScopeActiveOnly returns only non-archived memos. This is the
	// zero value and the default: an unspecified List excludes archived memos
	// so callers never surface soft-deleted memories by accident.
	MemoArchiveScopeActiveOnly MemoArchiveScope = iota
	// MemoArchiveScopeArchivedOnly returns only archived memos.
	MemoArchiveScopeArchivedOnly
	// MemoArchiveScopeAll returns both active and archived memos.
	MemoArchiveScopeAll
)

// Allows reports whether a memo with the given archived state passes this
// scope's filter.
func (s MemoArchiveScope) Allows(isArchived bool) bool {
	switch s {
	case MemoArchiveScopeArchivedOnly:
		return isArchived
	case MemoArchiveScopeAll:
		return true
	default: // MemoArchiveScopeActiveOnly
		return !isArchived
	}
}

// MemoListOptions controls how List filters memos.
type MemoListOptions struct {
	// ArchiveScope selects active / archived / both. Defaults to active only.
	ArchiveScope MemoArchiveScope
}

// MemoRepository defines the interface for Memo data access. Every method is
// Case-scoped (requires caseID) so a memo can never be read or written outside
// its parent Case; there is no memoID-only lookup.
type MemoRepository interface {
	// Create persists a new memo. The caller assigns the MemoID (via
	// model.NewMemoID) before calling; the repository does not generate IDs.
	Create(ctx context.Context, workspaceID string, memo *model.Memo) (*model.Memo, error)

	// Get retrieves a memo by ID within a Case. Archived memos are returned
	// as-is so callers holding the ID can inspect history; List is the
	// archive-filtered entry point.
	Get(ctx context.Context, workspaceID string, caseID int64, id model.MemoID) (*model.Memo, error)

	// GetByIDs retrieves multiple memos by ID within a Case in a single batch.
	// Returns a map keyed by MemoID containing only the memos that were found;
	// missing IDs are silently absent. Used by the GraphQL dataloader.
	GetByIDs(ctx context.Context, workspaceID string, caseID int64, ids []model.MemoID) (map[model.MemoID]*model.Memo, error)

	// List retrieves the memos of a Case, filtered by opts.ArchiveScope.
	List(ctx context.Context, workspaceID string, caseID int64, opts MemoListOptions) ([]*model.Memo, error)

	// Update persists changes to an existing memo (including archive/unarchive,
	// expressed by setting/clearing ArchivedAt on the caller's pointer).
	Update(ctx context.Context, workspaceID string, memo *model.Memo) (*model.Memo, error)
}
