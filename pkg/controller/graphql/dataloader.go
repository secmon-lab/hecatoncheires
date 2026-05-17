package graphql

import (
	"context"
	"fmt"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/slackid"
)

// ErrSlackUserNotInRepo is reported when SlackUserLoader is asked to
// resolve a user ID that the SlackUser repository has no record of.
// Surfacing it (instead of silently dropping the ID) makes "the Reporter
// column is suddenly empty" investigations 5 minutes instead of 5 hours
// — the empty cell now has a paired Sentry event / log line naming the
// missing ID, and the field-level GraphQL error tells the client that
// the resolver actually failed, not that the data is genuinely absent.
var ErrSlackUserNotInRepo = goerr.New("slack user not found in repository")

// DataLoaders holds all the data loaders used in the GraphQL resolvers
type DataLoaders struct {
	repo                        interfaces.Repository
	SlackUserLoader             *SlackUserLoader
	ActionLoader                *ActionLoader
	ActiveActionsByCaseLoader   *ActionsByCaseLoader
	ArchivedActionsByCaseLoader *ActionsByCaseLoader
	AllActionsByCaseLoader      *ActionsByCaseLoader
	CaseLoader                  *CaseLoader
}

// NewDataLoaders creates a new DataLoaders instance
func NewDataLoaders(repo interfaces.Repository) *DataLoaders {
	return &DataLoaders{
		repo:                        repo,
		SlackUserLoader:             NewSlackUserLoader(repo),
		ActionLoader:                NewActionLoader(repo),
		ActiveActionsByCaseLoader:   NewActionsByCaseLoader(repo, interfaces.ActionArchiveScopeActiveOnly),
		ArchivedActionsByCaseLoader: NewActionsByCaseLoader(repo, interfaces.ActionArchiveScopeArchivedOnly),
		AllActionsByCaseLoader:      NewActionsByCaseLoader(repo, interfaces.ActionArchiveScopeAll),
		CaseLoader:                  NewCaseLoader(repo),
	}
}

// actionsByCaseLoaderForScope picks the right per-case dataloader for the
// given archive scope.
func (d *DataLoaders) actionsByCaseLoaderForScope(scope interfaces.ActionArchiveScope) *ActionsByCaseLoader {
	switch scope {
	case interfaces.ActionArchiveScopeArchivedOnly:
		return d.ArchivedActionsByCaseLoader
	case interfaces.ActionArchiveScopeAll:
		return d.AllActionsByCaseLoader
	default:
		return d.ActiveActionsByCaseLoader
	}
}

// GetDataLoaders retrieves DataLoaders from context
func GetDataLoaders(ctx context.Context) *DataLoaders {
	loaders, ok := ctx.Value(dataLoadersKey).(*DataLoaders)
	if !ok {
		panic("dataloaders not found in context")
	}
	return loaders
}

type dataLoadersKeyType string

const dataLoadersKey dataLoadersKeyType = "dataloaders"

// WithDataLoaders adds DataLoaders to context
func WithDataLoaders(ctx context.Context, loaders *DataLoaders) context.Context {
	return context.WithValue(ctx, dataLoadersKey, loaders)
}

// SlackUserLoader loads Slack users by ID
type SlackUserLoader struct {
	repo interfaces.Repository
}

func NewSlackUserLoader(repo interfaces.Repository) *SlackUserLoader {
	return &SlackUserLoader{repo: repo}
}

func (l *SlackUserLoader) Load(ctx context.Context, ids []string) ([]*graphql1.SlackUser, error) {
	// Normalise inbound IDs so legacy composite "Uxxx-Txxx" sub claims
	// persisted before the auth-side fix still resolve against the
	// SlackUser repository (keyed on the bare "Uxxx" / "Wxxx" form).
	// We dedupe by the normalised key so a single batch hits the repo
	// once per real user.
	normalizedSet := make(map[model.SlackUserID]struct{}, len(ids))
	resolvedIDs := make([]model.SlackUserID, len(ids))
	for i, id := range ids {
		n := model.SlackUserID(slackid.Normalize(id))
		resolvedIDs[i] = n
		normalizedSet[n] = struct{}{}
	}
	uniqueIDs := make([]model.SlackUserID, 0, len(normalizedSet))
	for id := range normalizedSet {
		uniqueIDs = append(uniqueIDs, id)
	}

	users, err := l.repo.SlackUser().GetByIDs(ctx, uniqueIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to load slack users: %w", err)
	}

	userMap := make(map[model.SlackUserID]*graphql1.SlackUser)
	for _, user := range users {
		imageURL := user.ImageURL
		var imageURLPtr *string
		if imageURL != "" {
			imageURLPtr = &imageURL
		}
		userMap[user.ID] = &graphql1.SlackUser{
			ID:       string(user.ID),
			Name:     user.Name,
			RealName: user.RealName,
			ImageURL: imageURLPtr,
		}
	}

	// Detect missing IDs and report them as a non-fatal error so the
	// "Slack user repo never got synced" failure mode stops being
	// invisible. We still return successfully-resolved users for the
	// rest of the batch — the schema declares assignees as
	// [SlackUser!]! (non-null elements), so emitting nil elements would
	// blow up GraphQL marshalling for the whole list.
	missing := make([]string, 0)
	seen := make(map[model.SlackUserID]struct{}, len(uniqueIDs))
	for _, id := range uniqueIDs {
		if _, ok := userMap[id]; ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		missing = append(missing, string(id))
	}
	if len(missing) > 0 {
		errutil.Handle(ctx, goerr.Wrap(ErrSlackUserNotInRepo,
			"slack user lookup missed repository entries",
			goerr.V("missing_ids", missing),
		), "slack user lookup")
	}

	result := make([]*graphql1.SlackUser, 0, len(resolvedIDs))
	for _, id := range resolvedIDs {
		if user, ok := userMap[id]; ok {
			result = append(result, user)
		}
	}

	return result, nil
}

// ActionLoader loads actions by ID
type ActionLoader struct {
	repo interfaces.Repository
}

func NewActionLoader(repo interfaces.Repository) *ActionLoader {
	return &ActionLoader{repo: repo}
}

func (l *ActionLoader) Load(ctx context.Context, workspaceID string, ids []int64) ([]*model.Action, error) {
	actions := make([]*model.Action, len(ids))
	for i, id := range ids {
		action, err := l.repo.Action().Get(ctx, workspaceID, id)
		if err != nil {
			return nil, fmt.Errorf("failed to load action %d: %w", id, err)
		}
		actions[i] = action
	}

	return actions, nil
}

// ActionsByCaseLoader loads actions by case ID for a fixed ArchiveScope.
// Active and archived views are kept on separate loader instances so each
// caches its own slice and the per-case dataloader pattern works for both.
type ActionsByCaseLoader struct {
	repo  interfaces.Repository
	scope interfaces.ActionArchiveScope
}

func NewActionsByCaseLoader(repo interfaces.Repository, scope interfaces.ActionArchiveScope) *ActionsByCaseLoader {
	return &ActionsByCaseLoader{repo: repo, scope: scope}
}

// Load returns the actions for each case ID under the loader's scope.
func (l *ActionsByCaseLoader) Load(ctx context.Context, workspaceID string, caseIDs []int64) (map[int64][]*model.Action, error) {
	actions, err := l.repo.Action().GetByCases(ctx, workspaceID, caseIDs, interfaces.ActionListOptions{ArchiveScope: l.scope})
	if err != nil {
		return nil, fmt.Errorf("failed to load actions by case: %w", err)
	}

	return actions, nil
}

// CaseLoader loads cases by ID
type CaseLoader struct {
	repo interfaces.Repository
}

func NewCaseLoader(repo interfaces.Repository) *CaseLoader {
	return &CaseLoader{repo: repo}
}

func (l *CaseLoader) Load(ctx context.Context, workspaceID string, ids []int64) ([]*model.Case, error) {
	cases := make([]*model.Case, len(ids))
	for i, id := range ids {
		c, err := l.repo.Case().Get(ctx, workspaceID, id)
		if err != nil {
			return nil, fmt.Errorf("failed to load case %d: %w", id, err)
		}
		cases[i] = c
	}

	return cases, nil
}
