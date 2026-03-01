package usecase

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
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

	// Access control: filter out inaccessible private cases
	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil {
		accessible := make([]*model.Case, 0, len(cases))
		for _, c := range cases {
			if model.IsCaseAccessible(c, token.Sub) {
				accessible = append(accessible, c)
			}
		}
		cases = accessible
	}

	if len(cases) == 0 {
		return []*model.Action{}, nil
	}

	caseIDs := make([]int64, len(cases))
	for i, c := range cases {
		caseIDs[i] = c.ID
	}

	actionsByCase, err := uc.repo.Action().GetByCases(ctx, workspaceID, caseIDs)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get actions for open cases")
	}

	var allActions []*model.Action
	for _, actions := range actionsByCase {
		allActions = append(allActions, actions...)
	}

	if allActions == nil {
		allActions = []*model.Action{}
	}

	return allActions, nil
}
