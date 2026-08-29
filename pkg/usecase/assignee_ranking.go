package usecase

import (
	"context"
	"sort"
	"time"

	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

// The ranking exists only to order a picker, so both values are product
// decisions fixed here rather than exposed as configuration (the same stance as
// homeMessageFreshWindow in dashboard.go).
const (
	// assigneeRankingFreshWindow is how long a computed ranking is served
	// before a recompute is triggered. Assignment habits move slowly, so an
	// hour of lag costs nothing and keeps the recompute rare.
	assigneeRankingFreshWindow = time.Hour

	// assigneeRankingSize is how many user IDs the ranking keeps — roughly the
	// number of picker rows visible without scrolling.
	assigneeRankingSize = 12
)

// ListFrequentAssignees returns the Slack user IDs most often assigned to the
// workspace's non-private Cases, most assigned first. The result is a hint for
// ordering the assignee picker, not an exact statistic: it is served from a
// cached ranking that lags the Cases by up to assigneeRankingFreshWindow.
//
// The request path costs one document read. When the cached ranking is stale or
// absent, the recompute is dispatched to the async tail and the caller gets
// whatever is on hand — an empty slice on a cold cache, which clients fall back
// from to their own default ordering.
func (uc *CaseUseCase) ListFrequentAssignees(ctx context.Context, workspaceID string) ([]string, error) {
	// Reject an unconfigured workspace before touching storage. The other
	// workspace-scoped reads can skip this because a bogus id simply returns
	// nothing, but a cold cache here WRITES a ranking document, and Firestore
	// creates workspaces/{anything}/rankings/assignee happily even when no such
	// workspace exists — so an unchecked id lets a caller leave one junk
	// document behind per id it invents. The registry is in-process
	// configuration, so the check costs no read.
	//
	// A nil registry means no workspace configuration was supplied at all
	// (tests, and callers constructed without one); skipping the check there
	// matches fieldValidatorForWorkspace and MemoUseCase.MemoConfiguration.
	if uc.workspaceRegistry != nil {
		if _, err := uc.workspaceRegistry.Get(workspaceID); err != nil {
			return nil, goerr.Wrap(err, "failed to resolve workspace for assignee ranking",
				goerr.V("workspace_id", workspaceID))
		}
	}

	now := time.Now()

	ranking, err := uc.repo.AssigneeRanking().Get(ctx, workspaceID)
	if err != nil {
		if !isRepoNotFound(err) {
			return nil, goerr.Wrap(err, "failed to get assignee ranking",
				goerr.V("workspace_id", workspaceID))
		}
		ranking = &model.AssigneeRanking{WorkspaceID: workspaceID}
	}

	if !ranking.IsFresh(now, assigneeRankingFreshWindow) {
		// Deliberately unsynchronised: several instances crossing the freshness
		// boundary at once each recompute and overwrite one another with
		// near-identical rankings. Coordinating them would cost a claim
		// document and its expiry handling to save a handful of reads per hour.
		async.Dispatch(ctx, func(ctx context.Context) error {
			return uc.refreshAssigneeRanking(ctx, workspaceID, now)
		})
	}

	if ranking.UserIDs == nil {
		return []string{}, nil
	}
	return ranking.UserIDs, nil
}

// refreshAssigneeRanking recomputes the workspace's ranking and stores it. It
// runs in the async tail, so its error is reported by async.Dispatch through
// errutil.Handle and never reaches the user's request.
func (uc *CaseUseCase) refreshAssigneeRanking(ctx context.Context, workspaceID string, now time.Time) error {
	// Passing no options excludes DRAFT cases and archived cases
	// (CaseRepository.List's defaults). The ranking is a frequency signal that
	// orders the assignee picker, so cases the team has put away should stop
	// influencing it.
	cases, err := uc.repo.Case().List(ctx, workspaceID)
	if err != nil {
		return goerr.Wrap(err, "failed to list cases for assignee ranking",
			goerr.V("workspace_id", workspaceID))
	}

	ranking := &model.AssigneeRanking{
		WorkspaceID: workspaceID,
		UserIDs:     rankAssignees(cases, assigneeRankingSize),
		ComputedAt:  now,
	}
	if err := uc.repo.AssigneeRanking().Set(ctx, ranking); err != nil {
		return goerr.Wrap(err, "failed to store assignee ranking",
			goerr.V("workspace_id", workspaceID))
	}
	return nil
}

// rankAssignees counts how many Cases each user is assigned to and returns at
// most topN user IDs, most assigned first. Ties break on user ID so the result
// is deterministic.
//
// Private Cases are skipped: one ranking document is served to every caller, so
// counting a private Case's assignees would let a non-member infer who is
// working inside it.
func rankAssignees(cases []*model.Case, topN int) []string {
	counts := make(map[string]int)
	for _, c := range cases {
		if c == nil || c.IsPrivate {
			continue
		}
		for _, id := range c.AssigneeIDs {
			if id == "" {
				continue
			}
			counts[id]++
		}
	}

	ids := make([]string, 0, len(counts))
	for id := range counts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if counts[ids[i]] != counts[ids[j]] {
			return counts[ids[i]] > counts[ids[j]]
		}
		return ids[i] < ids[j]
	})

	if len(ids) > topN {
		ids = ids[:topN]
	}
	return ids
}
