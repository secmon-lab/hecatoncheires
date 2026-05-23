package cli

import (
	"context"

	slackgo "github.com/slack-go/slack"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casewriter"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// jobCaseAdapter translates the casewriter.CaseUpdate shape into
// usecase.CaseUpdate. The two structs are intentionally separate so the
// agent tool package does not import usecase (cycle).
type jobCaseAdapter struct {
	uc *usecase.CaseUseCase
}

func newJobCaseAdapter(uc *usecase.CaseUseCase) *jobCaseAdapter {
	if uc == nil {
		return nil
	}
	return &jobCaseAdapter{uc: uc}
}

func (a *jobCaseAdapter) UpdateCase(ctx context.Context, workspaceID string, id int64, patch casewriter.CaseUpdate) (*model.Case, error) {
	if a == nil || a.uc == nil {
		return nil, nil
	}
	in := usecase.CaseUpdate{
		Title:       patch.Title,
		Description: patch.Description,
		Fields:      patch.Fields,
	}
	if patch.HasAssign {
		in.SetAssignees(patch.AssigneeIDs)
	}
	return a.uc.UpdateCase(ctx, workspaceID, id, in)
}

// slackPosterAdapter bridges slackpost.Poster onto the existing
// slacksvc.Service. The Poster interface intentionally exposes only
// PostMessage / PostThreadMessage, so an LLM with the slack_post tool
// cannot reach the broader Slack API surface.
type slackPosterAdapter struct {
	svc slacksvc.Service
}

func (a slackPosterAdapter) PostMessage(ctx context.Context, channelID string, blocks []slackgo.Block, text string) (string, error) {
	if a.svc == nil {
		return "", nil
	}
	return a.svc.PostMessage(ctx, channelID, blocks, text)
}

func (a slackPosterAdapter) PostThreadMessage(ctx context.Context, channelID string, threadTS string, blocks []slackgo.Block, text string) (string, error) {
	if a.svc == nil {
		return "", nil
	}
	return a.svc.PostThreadMessage(ctx, channelID, threadTS, blocks, text)
}
