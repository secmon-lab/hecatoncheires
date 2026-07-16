package usecase

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casemulti"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// casemulti_tool_adapter bridges the cross-case workspace-agent tool set
// (pkg/agent/tool/casemulti) onto the concrete usecases, mirroring
// case_tool_adapter.go / action_tool_adapter.go. It exists for the same
// import-cycle reason: casemulti must not import pkg/usecase, so its
// CaseUsecase / ActionUsecase interfaces use package-local patch structs and a
// plain actorID string, which these adapters translate into the usecase's
// CaseUpdate / UpdateActionInput / ActorRef shapes.
//
// Unlike the single-case sub-agent adapters (which pin ActorKindSystem because
// an investigation sub-agent is not a Slack user), every workspace-agent write
// acts on behalf of the human who mentioned the agent, so writes are attributed
// to ActorKindSlackUser with the mentioning user's id. This makes change
// history / Slack notifications name that user and enforces private-case access
// as defense in depth even if a future caller lost its ctx auth token.

// caseMultiCaseAdapter wraps a CaseUseCase as a casemulti.CaseUsecase.
type caseMultiCaseAdapter struct {
	uc *CaseUseCase
}

// NewCaseMultiCaseAdapter returns a casemulti.CaseUsecase backed by uc, or nil
// when uc is nil so hosts can wire casemulti unconditionally (a nil CaseUC
// makes casemulti.New return no tools).
func NewCaseMultiCaseAdapter(uc *CaseUseCase) casemulti.CaseUsecase {
	if uc == nil {
		return nil
	}
	return &caseMultiCaseAdapter{uc: uc}
}

func (a *caseMultiCaseAdapter) ListCases(ctx context.Context, workspaceID string, status *types.CaseStatus) ([]*model.Case, error) {
	return a.uc.ListCases(ctx, workspaceID, status)
}

func (a *caseMultiCaseAdapter) GetCase(ctx context.Context, workspaceID string, id int64) (*model.Case, error) {
	return a.uc.GetCase(ctx, workspaceID, id)
}

// CreateCase fixes isTest / sourceTeamID / requestKey to false / "" / "":
// agent-tool creation has no Slack-modal double-submit to dedup, mirroring the
// GraphQL mutation's call site. The reporter is derived from the ctx auth token
// the host injects.
func (a *caseMultiCaseAdapter) CreateCase(ctx context.Context, workspaceID string, title, description string, assigneeIDs []string, fieldValues map[string]model.FieldValue, isPrivate bool) (*model.Case, error) {
	return a.uc.CreateCase(ctx, workspaceID, title, description, assigneeIDs, fieldValues, isPrivate, false, "", "")
}

func (a *caseMultiCaseAdapter) UpdateCase(ctx context.Context, workspaceID string, id int64, patch casemulti.CaseUpdate) (*model.Case, error) {
	return a.uc.UpdateCase(ctx, workspaceID, id, CaseUpdate{
		Title:       patch.Title,
		Description: patch.Description,
		Fields:      patch.Fields,
	})
}

func (a *caseMultiCaseAdapter) CloseCase(ctx context.Context, workspaceID string, id int64) (*model.Case, error) {
	return a.uc.CloseCase(ctx, workspaceID, id)
}

// caseMultiActionAdapter wraps ActionUseCase + ActionStepUseCase as a
// casemulti.ActionUsecase, attributing writes to the mentioning Slack user.
type caseMultiActionAdapter struct {
	action *ActionUseCase
	step   *ActionStepUseCase
}

// NewCaseMultiActionAdapter returns a casemulti.ActionUsecase backed by the
// action + action-step usecases, or nil when either is nil (which disables the
// casemulti action tools while leaving the case-level tools working).
func NewCaseMultiActionAdapter(action *ActionUseCase, step *ActionStepUseCase) casemulti.ActionUsecase {
	if action == nil || step == nil {
		return nil
	}
	return &caseMultiActionAdapter{action: action, step: step}
}

func (a *caseMultiActionAdapter) GetActionsByCase(ctx context.Context, workspaceID string, caseID int64, opts interfaces.ActionListOptions) ([]*model.Action, error) {
	return a.action.GetActionsByCase(ctx, workspaceID, caseID, opts)
}

func (a *caseMultiActionAdapter) GetAction(ctx context.Context, workspaceID string, id int64, opts ...interfaces.ActionListOptions) (*model.Action, error) {
	return a.action.GetAction(ctx, workspaceID, id, opts...)
}

// CreateAction fills the assignee / slackMessageTS / status / dueDate the agent
// tool does not collect: an empty status is normalized to the workspace's
// initial action status by CreateAction. Access + "created by" attribution are
// ctx-token-only (no actor parameter), matching the real usecase.
func (a *caseMultiActionAdapter) CreateAction(ctx context.Context, workspaceID string, caseID int64, title, description string) (*model.Action, error) {
	return a.action.CreateAction(ctx, workspaceID, caseID, title, description, "", "", types.ActionStatus(""), nil)
}

func (a *caseMultiActionAdapter) UpdateAction(ctx context.Context, workspaceID string, actionID int64, patch casemulti.ActionUpdate, actorID string) (*model.Action, error) {
	return a.action.UpdateAction(ctx, workspaceID, UpdateActionInput{
		ID:          actionID,
		Title:       patch.Title,
		Description: patch.Description,
		Status:      patch.Status,
		Actor:       ActorRef{Kind: ActorKindSlackUser, ID: actorID},
		SlackSync:   SlackSyncFull,
	})
}

func (a *caseMultiActionAdapter) AddActionStep(ctx context.Context, workspaceID string, actionID int64, title string, actorID string) (*model.ActionStep, error) {
	return a.step.Add(ctx, AddActionStepInput{
		WorkspaceID: workspaceID,
		ActionID:    actionID,
		Title:       title,
		Actor:       ActorRef{Kind: ActorKindSlackUser, ID: actorID},
	})
}

func (a *caseMultiActionAdapter) SetActionStepDone(ctx context.Context, workspaceID string, actionID int64, stepID string, done bool, actorID string) (*model.ActionStep, error) {
	return a.step.SetDone(ctx, SetActionStepDoneInput{
		WorkspaceID: workspaceID,
		ActionID:    actionID,
		StepID:      stepID,
		Done:        done,
		Actor:       ActorRef{Kind: ActorKindSlackUser, ID: actorID},
	})
}
