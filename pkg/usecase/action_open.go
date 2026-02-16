package usecase

import (
	"context"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// ListOpenCaseActions returns all actions belonging to cases with OPEN status.
func (uc *ActionUseCase) ListOpenCaseActions(ctx context.Context, workspaceID string) ([]*model.Action, error) {
	openStatus := types.CaseStatusOpen
	cases, err := uc.repo.Case().List(ctx, workspaceID, interfaces.WithStatus(openStatus))
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list open cases")
	}

	if len(cases) == 0 {
		return []*model.Action{}, nil
	}

	type result struct {
		actions []*model.Action
		err     error
	}

	results := make([]result, len(cases))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // Limit concurrent DB queries

	for i, c := range cases {
		wg.Add(1)
		go func(idx int, caseID int64) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			actions, err := uc.repo.Action().GetByCase(ctx, workspaceID, caseID)
			results[idx] = result{actions: actions, err: err}
		}(i, c.ID)
	}

	wg.Wait()

	var allActions []*model.Action
	for _, r := range results {
		if r.err != nil {
			return nil, goerr.Wrap(r.err, "failed to get actions for open case")
		}
		allActions = append(allActions, r.actions...)
	}

	if allActions == nil {
		allActions = []*model.Action{}
	}

	return allActions, nil
}
