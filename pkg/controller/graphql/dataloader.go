package graphql

import (
	"context"

	"github.com/graph-gophers/dataloader/v7"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/slackid"
)

// ErrSlackUserNotInRepo is reported when the SlackUser loader is asked
// to resolve a user ID that the SlackUser repository has no record of.
// Surfacing it (instead of silently dropping the ID) makes "the
// Reporter column is suddenly empty" investigations 5 minutes instead
// of 5 hours - the empty cell now has a paired Sentry event / log line
// naming the missing ID, and the field-level GraphQL error tells the
// client that the resolver actually failed, not that the data is
// genuinely absent.
var ErrSlackUserNotInRepo = goerr.New("slack user not found in repository")

type actionKey struct {
	workspaceID string
	id          int64
}

type caseKey struct {
	workspaceID string
	caseID      int64
}

type actionsByCaseKey struct {
	workspaceID string
	caseID      int64
}

// DataLoaders bundles every request-scoped batching loader the GraphQL
// resolvers use. A fresh instance is constructed per HTTP request by
// the dataloader middleware (see cli/serve.go); the loaders MUST NOT
// be reused across requests - their internal cache would otherwise
// leak data from one user's view into another's.
type DataLoaders struct {
	SlackUser                   *dataloader.Loader[string, *graphql1.SlackUser]
	SlackChannelName            *dataloader.Loader[string, *string]
	Action                      *dataloader.Loader[actionKey, *model.Action]
	Case                        *dataloader.Loader[caseKey, *model.Case]
	ActiveActionsByCaseLoader   *dataloader.Loader[actionsByCaseKey, []*model.Action]
	ArchivedActionsByCaseLoader *dataloader.Loader[actionsByCaseKey, []*model.Action]
	AllActionsByCaseLoader      *dataloader.Loader[actionsByCaseKey, []*model.Action]
}

// NewDataLoaders constructs a fresh set of request-scoped loaders.
// slackSvc may be nil when Slack is not configured; in that case the
// channel-name loader returns nil for every key (matching the old
// per-resolver behaviour).
func NewDataLoaders(repo interfaces.Repository, slackSvc slacksvc.Service) *DataLoaders {
	return &DataLoaders{
		SlackUser:                   dataloader.NewBatchedLoader(buildSlackUserBatch(repo)),
		SlackChannelName:            dataloader.NewBatchedLoader(buildSlackChannelNameBatch(slackSvc)),
		Action:                      dataloader.NewBatchedLoader(buildActionBatch(repo)),
		Case:                        dataloader.NewBatchedLoader(buildCaseBatch(repo)),
		ActiveActionsByCaseLoader:   dataloader.NewBatchedLoader(buildActionsByCaseBatch(repo, interfaces.ActionArchiveScopeActiveOnly)),
		ArchivedActionsByCaseLoader: dataloader.NewBatchedLoader(buildActionsByCaseBatch(repo, interfaces.ActionArchiveScopeArchivedOnly)),
		AllActionsByCaseLoader:      dataloader.NewBatchedLoader(buildActionsByCaseBatch(repo, interfaces.ActionArchiveScopeAll)),
	}
}

// ActionsByCaseLoaderForScope picks the right per-case dataloader for
// the given archive scope.
func (d *DataLoaders) ActionsByCaseLoaderForScope(scope interfaces.ActionArchiveScope) *dataloader.Loader[actionsByCaseKey, []*model.Action] {
	switch scope {
	case interfaces.ActionArchiveScopeArchivedOnly:
		return d.ArchivedActionsByCaseLoader
	case interfaces.ActionArchiveScopeAll:
		return d.AllActionsByCaseLoader
	default:
		return d.ActiveActionsByCaseLoader
	}
}

// MakeActionKey constructs an action loader key.
func MakeActionKey(workspaceID string, id int64) actionKey {
	return actionKey{workspaceID: workspaceID, id: id}
}

// MakeCaseKey constructs a case loader key.
func MakeCaseKey(workspaceID string, id int64) caseKey {
	return caseKey{workspaceID: workspaceID, caseID: id}
}

// MakeActionsByCaseKey constructs an actions-by-case loader key.
func MakeActionsByCaseKey(workspaceID string, caseID int64) actionsByCaseKey {
	return actionsByCaseKey{workspaceID: workspaceID, caseID: caseID}
}

// GetDataLoaders retrieves DataLoaders from context. Panics if no
// loaders are bound - every GraphQL request goes through the
// dataloader middleware, so an unset value indicates a wiring bug
// rather than an expected runtime condition.
func GetDataLoaders(ctx context.Context) *DataLoaders {
	loaders, ok := ctx.Value(dataLoadersKey).(*DataLoaders)
	if !ok {
		panic("dataloaders not found in context")
	}
	return loaders
}

type dataLoadersKeyType string

const dataLoadersKey dataLoadersKeyType = "dataloaders"

// WithDataLoaders adds DataLoaders to context.
func WithDataLoaders(ctx context.Context, loaders *DataLoaders) context.Context {
	return context.WithValue(ctx, dataLoadersKey, loaders)
}

// buildSlackUserBatch returns the SlackUser batch function. It
// normalises composite "Uxxx-Txxx" sub claims (legacy tokens persisted
// before the auth-side fix) down to the bare ID the SlackUser repo is
// keyed on, dedupes, and emits a non-fatal errutil.Handle for any IDs
// the repo could not resolve so the failure mode is visible in ops.
func buildSlackUserBatch(repo interfaces.Repository) dataloader.BatchFunc[string, *graphql1.SlackUser] {
	return func(ctx context.Context, ids []string) []*dataloader.Result[*graphql1.SlackUser] {
		normalized := make([]model.SlackUserID, len(ids))
		uniqueSet := make(map[model.SlackUserID]struct{}, len(ids))
		for i, id := range ids {
			n := model.SlackUserID(slackid.Normalize(id))
			normalized[i] = n
			uniqueSet[n] = struct{}{}
		}
		uniqueIDs := make([]model.SlackUserID, 0, len(uniqueSet))
		for id := range uniqueSet {
			uniqueIDs = append(uniqueIDs, id)
		}

		results := make([]*dataloader.Result[*graphql1.SlackUser], len(ids))
		users, err := repo.SlackUser().GetByIDs(ctx, uniqueIDs)
		if err != nil {
			wrapped := goerr.Wrap(err, "failed to load slack users")
			for i := range results {
				results[i] = &dataloader.Result[*graphql1.SlackUser]{Error: wrapped}
			}
			return results
		}

		userMap := make(map[model.SlackUserID]*graphql1.SlackUser, len(users))
		for _, u := range users {
			imageURL := u.ImageURL
			var imageURLPtr *string
			if imageURL != "" {
				imageURLPtr = &imageURL
			}
			userMap[u.ID] = &graphql1.SlackUser{
				ID:       string(u.ID),
				Name:     u.Name,
				RealName: u.RealName,
				ImageURL: imageURLPtr,
			}
		}

		missing := make([]string, 0)
		for id := range uniqueSet {
			if _, ok := userMap[id]; !ok {
				missing = append(missing, string(id))
			}
		}
		if len(missing) > 0 {
			errutil.Handle(ctx, goerr.Wrap(ErrSlackUserNotInRepo,
				"slack user lookup missed repository entries",
				goerr.V("missing_ids", missing),
			), "slack user lookup")
		}

		for i, n := range normalized {
			if user, ok := userMap[n]; ok {
				results[i] = &dataloader.Result[*graphql1.SlackUser]{Data: user}
				continue
			}
			results[i] = &dataloader.Result[*graphql1.SlackUser]{Data: nil}
		}
		return results
	}
}

// buildSlackChannelNameBatch returns the SlackChannelName batch
// function. When slackSvc is nil (Slack disabled), every key resolves
// to nil - the resolver passes that through as a null GraphQL field,
// matching the behaviour of the old non-batched path.
func buildSlackChannelNameBatch(slackSvc slacksvc.Service) dataloader.BatchFunc[string, *string] {
	return func(ctx context.Context, ids []string) []*dataloader.Result[*string] {
		results := make([]*dataloader.Result[*string], len(ids))

		if slackSvc == nil {
			for i := range results {
				results[i] = &dataloader.Result[*string]{Data: nil}
			}
			return results
		}

		uniqueSet := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			uniqueSet[id] = struct{}{}
		}
		uniqueIDs := make([]string, 0, len(uniqueSet))
		for id := range uniqueSet {
			uniqueIDs = append(uniqueIDs, id)
		}

		names, err := slackSvc.GetChannelNames(ctx, uniqueIDs)
		if err != nil {
			wrapped := goerr.Wrap(err, "failed to load slack channel names")
			for i := range results {
				results[i] = &dataloader.Result[*string]{Error: wrapped}
			}
			return results
		}

		for i, id := range ids {
			if name, ok := names[id]; ok {
				n := name
				results[i] = &dataloader.Result[*string]{Data: &n}
				continue
			}
			results[i] = &dataloader.Result[*string]{Data: nil}
		}
		return results
	}
}

// buildActionBatch returns the Action batch function.
func buildActionBatch(repo interfaces.Repository) dataloader.BatchFunc[actionKey, *model.Action] {
	return func(ctx context.Context, keys []actionKey) []*dataloader.Result[*model.Action] {
		byWs := make(map[string][]int64, 1)
		for _, k := range keys {
			byWs[k.workspaceID] = append(byWs[k.workspaceID], k.id)
		}

		type wsResult struct {
			actions map[int64]*model.Action
			err     error
		}
		wsResults := make(map[string]wsResult, len(byWs))
		for ws, ids := range byWs {
			actions, err := repo.Action().GetByIDs(ctx, ws, ids)
			wsResults[ws] = wsResult{actions: actions, err: err}
		}

		results := make([]*dataloader.Result[*model.Action], len(keys))
		for i, k := range keys {
			wr := wsResults[k.workspaceID]
			if wr.err != nil {
				results[i] = &dataloader.Result[*model.Action]{
					Error: goerr.Wrap(wr.err, "failed to batch get actions",
						goerr.V("workspace_id", k.workspaceID),
						goerr.V("id", k.id)),
				}
				continue
			}
			if a, ok := wr.actions[k.id]; ok {
				results[i] = &dataloader.Result[*model.Action]{Data: a}
				continue
			}
			results[i] = &dataloader.Result[*model.Action]{Data: nil}
		}
		return results
	}
}

// buildCaseBatch returns the Case batch function.
func buildCaseBatch(repo interfaces.Repository) dataloader.BatchFunc[caseKey, *model.Case] {
	return func(ctx context.Context, keys []caseKey) []*dataloader.Result[*model.Case] {
		byWs := make(map[string][]int64, 1)
		for _, k := range keys {
			byWs[k.workspaceID] = append(byWs[k.workspaceID], k.caseID)
		}

		type wsResult struct {
			cases map[int64]*model.Case
			err   error
		}
		wsResults := make(map[string]wsResult, len(byWs))
		for ws, ids := range byWs {
			cases, err := repo.Case().GetByIDs(ctx, ws, ids)
			wsResults[ws] = wsResult{cases: cases, err: err}
		}

		results := make([]*dataloader.Result[*model.Case], len(keys))
		for i, k := range keys {
			wr := wsResults[k.workspaceID]
			if wr.err != nil {
				results[i] = &dataloader.Result[*model.Case]{
					Error: goerr.Wrap(wr.err, "failed to batch get cases",
						goerr.V("workspace_id", k.workspaceID),
						goerr.V("id", k.caseID)),
				}
				continue
			}
			if c, ok := wr.cases[k.caseID]; ok {
				results[i] = &dataloader.Result[*model.Case]{Data: c}
				continue
			}
			results[i] = &dataloader.Result[*model.Case]{Data: nil}
		}
		return results
	}
}

// buildActionsByCaseBatch returns the ActionsByCase batch function for
// the given archive scope. One loader per scope keeps the cache slices
// disjoint so a request that loads active actions for case A and
// archived actions for case B does not see the wrong list.
func buildActionsByCaseBatch(repo interfaces.Repository, scope interfaces.ActionArchiveScope) dataloader.BatchFunc[actionsByCaseKey, []*model.Action] {
	return func(ctx context.Context, keys []actionsByCaseKey) []*dataloader.Result[[]*model.Action] {
		byWs := make(map[string][]int64, 1)
		for _, k := range keys {
			byWs[k.workspaceID] = append(byWs[k.workspaceID], k.caseID)
		}

		type wsResult struct {
			actions map[int64][]*model.Action
			err     error
		}
		wsResults := make(map[string]wsResult, len(byWs))
		opts := interfaces.ActionListOptions{ArchiveScope: scope}
		for ws, caseIDs := range byWs {
			actions, err := repo.Action().GetByCases(ctx, ws, caseIDs, opts)
			wsResults[ws] = wsResult{actions: actions, err: err}
		}

		results := make([]*dataloader.Result[[]*model.Action], len(keys))
		for i, k := range keys {
			wr := wsResults[k.workspaceID]
			if wr.err != nil {
				results[i] = &dataloader.Result[[]*model.Action]{
					Error: goerr.Wrap(wr.err, "failed to batch get actions by case",
						goerr.V("workspace_id", k.workspaceID),
						goerr.V("case_id", k.caseID)),
				}
				continue
			}
			if actions, ok := wr.actions[k.caseID]; ok {
				results[i] = &dataloader.Result[[]*model.Action]{Data: actions}
				continue
			}
			results[i] = &dataloader.Result[[]*model.Action]{Data: []*model.Action{}}
		}
		return results
	}
}
