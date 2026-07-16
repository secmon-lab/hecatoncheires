package casemulti_test

import (
	"context"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casemulti"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

type createCaseCall struct {
	title, description string
	assigneeIDs        []string
	fieldValues        map[string]model.FieldValue
	isPrivate          bool
}

type updateCaseCall struct {
	id    int64
	patch casemulti.CaseUpdate
}

type fakeCaseUC struct {
	casesByID map[int64]*model.Case
	getErr    error

	listResp []*model.Case
	listErr  error
	listArg  *types.CaseStatus

	createCalls []createCaseCall
	createResp  *model.Case
	createErr   error

	updateCalls []updateCaseCall
	updateResp  *model.Case
	updateErr   error

	closeCalls []int64
	closeResp  *model.Case
	closeErr   error
}

func (f *fakeCaseUC) ListCases(_ context.Context, _ string, status *types.CaseStatus) ([]*model.Case, error) {
	f.listArg = status
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listResp, nil
}

func (f *fakeCaseUC) GetCase(_ context.Context, _ string, id int64) (*model.Case, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	c, ok := f.casesByID[id]
	if !ok {
		return nil, goerr.New("case not found")
	}
	return c, nil
}

func (f *fakeCaseUC) CreateCase(_ context.Context, _ string, title, description string, assigneeIDs []string, fieldValues map[string]model.FieldValue, isPrivate bool) (*model.Case, error) {
	f.createCalls = append(f.createCalls, createCaseCall{title: title, description: description, assigneeIDs: assigneeIDs, fieldValues: fieldValues, isPrivate: isPrivate})
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.createResp, nil
}

func (f *fakeCaseUC) UpdateCase(_ context.Context, _ string, id int64, patch casemulti.CaseUpdate) (*model.Case, error) {
	f.updateCalls = append(f.updateCalls, updateCaseCall{id: id, patch: patch})
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.updateResp, nil
}

func (f *fakeCaseUC) CloseCase(_ context.Context, _ string, id int64) (*model.Case, error) {
	f.closeCalls = append(f.closeCalls, id)
	if f.closeErr != nil {
		return nil, f.closeErr
	}
	return f.closeResp, nil
}

type createActionCall struct {
	caseID             int64
	title, description string
}

type updateActionCall struct {
	actionID int64
	patch    casemulti.ActionUpdate
	actorID  string
}

type addStepCall struct {
	actionID int64
	title    string
	actorID  string
}

type setStepDoneCall struct {
	actionID int64
	stepID   string
	done     bool
	actorID  string
}

type fakeActionUC struct {
	actionsByID map[int64]*model.Action
	getErr      error

	listResp []*model.Action
	listErr  error

	createCalls []createActionCall
	createResp  *model.Action
	createErr   error

	updateCalls []updateActionCall
	updateResp  *model.Action
	updateErr   error

	addStepCalls []addStepCall
	addStepResp  *model.ActionStep
	addStepErr   error

	setDoneCalls []setStepDoneCall
	setDoneResp  *model.ActionStep
	setDoneErr   error
}

func (f *fakeActionUC) GetActionsByCase(_ context.Context, _ string, _ int64, _ interfaces.ActionListOptions) ([]*model.Action, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listResp, nil
}

func (f *fakeActionUC) GetAction(_ context.Context, _ string, id int64, _ ...interfaces.ActionListOptions) (*model.Action, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	a, ok := f.actionsByID[id]
	if !ok {
		return nil, goerr.New("action not found")
	}
	return a, nil
}

func (f *fakeActionUC) CreateAction(_ context.Context, _ string, caseID int64, title, description string) (*model.Action, error) {
	f.createCalls = append(f.createCalls, createActionCall{caseID: caseID, title: title, description: description})
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.createResp, nil
}

func (f *fakeActionUC) UpdateAction(_ context.Context, _ string, actionID int64, patch casemulti.ActionUpdate, actorID string) (*model.Action, error) {
	f.updateCalls = append(f.updateCalls, updateActionCall{actionID: actionID, patch: patch, actorID: actorID})
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.updateResp, nil
}

func (f *fakeActionUC) AddActionStep(_ context.Context, _ string, actionID int64, title string, actorID string) (*model.ActionStep, error) {
	f.addStepCalls = append(f.addStepCalls, addStepCall{actionID: actionID, title: title, actorID: actorID})
	if f.addStepErr != nil {
		return nil, f.addStepErr
	}
	return f.addStepResp, nil
}

func (f *fakeActionUC) SetActionStepDone(_ context.Context, _ string, actionID int64, stepID string, done bool, actorID string) (*model.ActionStep, error) {
	f.setDoneCalls = append(f.setDoneCalls, setStepDoneCall{actionID: actionID, stepID: stepID, done: done, actorID: actorID})
	if f.setDoneErr != nil {
		return nil, f.setDoneErr
	}
	return f.setDoneResp, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func toolByName(t *testing.T, tools []gollem.Tool, name string) gollem.Tool {
	t.Helper()
	for _, tl := range tools {
		if tl.Spec().Name == name {
			return tl
		}
	}
	return nil
}

func testSchema() *config.FieldSchema {
	return &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{ID: "summary", Name: "Summary", Type: types.FieldTypeText},
			{ID: "score", Name: "Score", Type: types.FieldTypeNumber},
		},
	}
}

// ---------------------------------------------------------------------------
// New
// ---------------------------------------------------------------------------

func TestNew_NilCaseUC(t *testing.T) {
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws"})
	gt.Array(t, tools).Length(0)
}

func TestNew_CaseOnly(t *testing.T) {
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: &fakeCaseUC{}})
	// list_cases, get_case, create_case, update_case, close_case
	gt.Array(t, tools).Length(5).Required()
	gt.Value(t, toolByName(t, tools, "case__list_actions")).Nil()
}

func TestNew_CaseAndAction(t *testing.T) {
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: &fakeCaseUC{}, ActionUC: &fakeActionUC{}})
	gt.Array(t, tools).Length(12).Required()
	for _, name := range []string{
		"case__list_cases", "case__get_case", "case__create_case", "case__update_case", "case__close_case",
		"case__list_actions", "case__get_action", "case__create_action", "case__update_action",
		"case__update_action_status", "case__add_action_step", "case__set_action_step_done",
	} {
		gt.Value(t, toolByName(t, tools, name)).NotNil()
	}
}

// ---------------------------------------------------------------------------
// case__list_cases
// ---------------------------------------------------------------------------

func TestListCasesTool_FiltersAccessDenied(t *testing.T) {
	uc := &fakeCaseUC{listResp: []*model.Case{
		{ID: 1, Title: "visible", Status: types.CaseStatusOpen, AssigneeIDs: []string{"U1"}},
		{ID: 2, Title: "hidden", Status: types.CaseStatusOpen, AccessDenied: true},
	}}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: uc})
	lc := toolByName(t, tools, "case__list_cases")
	gt.Value(t, lc).NotNil().Required()

	out, err := lc.Run(context.Background(), map[string]any{})
	gt.NoError(t, err).Required()

	items, ok := out["cases"].([]map[string]any)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, items).Length(1).Required()
	gt.Number(t, items[0]["id"].(int64)).Equal(int64(1))
	gt.String(t, items[0]["title"].(string)).Equal("visible")
	gt.Value(t, items[0]["assignee_ids"]).Equal([]string{"U1"})
}

func TestListCasesTool_StatusFilter(t *testing.T) {
	uc := &fakeCaseUC{listResp: []*model.Case{}}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: uc})
	lc := toolByName(t, tools, "case__list_cases")
	gt.Value(t, lc).NotNil().Required()

	_, err := lc.Run(context.Background(), map[string]any{"status": "OPEN"})
	gt.NoError(t, err).Required()
	gt.Value(t, uc.listArg).NotNil().Required()
	gt.Value(t, *uc.listArg).Equal(types.CaseStatusOpen)
}

func TestListCasesTool_InvalidStatus(t *testing.T) {
	uc := &fakeCaseUC{}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: uc})
	lc := toolByName(t, tools, "case__list_cases")
	gt.Value(t, lc).NotNil().Required()

	_, err := lc.Run(context.Background(), map[string]any{"status": "NOT_A_STATUS"})
	gt.Error(t, err)
}

// ---------------------------------------------------------------------------
// case__get_case
// ---------------------------------------------------------------------------

func TestGetCaseTool_Success(t *testing.T) {
	uc := &fakeCaseUC{casesByID: map[int64]*model.Case{
		42: {ID: 42, Title: "t", Description: "d", Status: types.CaseStatusOpen, ReporterID: "U1", AssigneeIDs: []string{"U2"},
			FieldValues: map[string]model.FieldValue{"summary": {FieldID: "summary", Type: types.FieldTypeText, Value: "done"}}},
	}}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: uc})
	gc := toolByName(t, tools, "case__get_case")
	gt.Value(t, gc).NotNil().Required()

	out, err := gc.Run(context.Background(), map[string]any{"case_id": int64(42)})
	gt.NoError(t, err).Required()
	gt.Number(t, out["id"].(int64)).Equal(int64(42))
	gt.String(t, out["title"].(string)).Equal("t")
	gt.String(t, out["description"].(string)).Equal("d")
	gt.String(t, out["reporter_id"].(string)).Equal("U1")
	gt.Value(t, out["assignee_ids"]).Equal([]string{"U2"})
	fv, ok := out["field_values"].(map[string]any)
	gt.Bool(t, ok).True().Required()
	gt.Value(t, fv["summary"]).Equal("done")
}

func TestGetCaseTool_AccessDenied(t *testing.T) {
	uc := &fakeCaseUC{casesByID: map[int64]*model.Case{
		42: {ID: 42, AccessDenied: true},
	}}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: uc})
	gc := toolByName(t, tools, "case__get_case")
	gt.Value(t, gc).NotNil().Required()

	out, err := gc.Run(context.Background(), map[string]any{"case_id": int64(42)})
	gt.Error(t, err)
	gt.Value(t, out).Nil()
}

func TestGetCaseTool_NotFound(t *testing.T) {
	uc := &fakeCaseUC{casesByID: map[int64]*model.Case{}}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: uc})
	gc := toolByName(t, tools, "case__get_case")
	gt.Value(t, gc).NotNil().Required()

	_, err := gc.Run(context.Background(), map[string]any{"case_id": int64(99)})
	gt.Error(t, err)
}

// ---------------------------------------------------------------------------
// case__create_case
// ---------------------------------------------------------------------------

func TestCreateCaseTool(t *testing.T) {
	uc := &fakeCaseUC{createResp: &model.Case{ID: 7, Title: "new case", Status: types.CaseStatusOpen}}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: uc, Schema: testSchema()})
	cc := toolByName(t, tools, "case__create_case")
	gt.Value(t, cc).NotNil().Required()

	out, err := cc.Run(context.Background(), map[string]any{
		"title":       "new case",
		"description": "desc",
		"assignees":   []any{"U1", "U2"},
		"fields": []any{
			map[string]any{"field_id": "summary", "value": "hello"},
		},
		"is_private": true,
	})
	gt.NoError(t, err).Required()

	gt.Array(t, uc.createCalls).Length(1).Required()
	call := uc.createCalls[0]
	gt.String(t, call.title).Equal("new case")
	gt.String(t, call.description).Equal("desc")
	gt.Array(t, call.assigneeIDs).Length(2).Required()
	gt.String(t, call.assigneeIDs[0]).Equal("U1")
	gt.String(t, call.assigneeIDs[1]).Equal("U2")
	gt.Bool(t, call.isPrivate).True()
	gt.Value(t, call.fieldValues["summary"].Value).Equal("hello")

	gt.Number(t, out["id"].(int64)).Equal(int64(7))
}

func TestCreateCaseTool_MissingTitle(t *testing.T) {
	uc := &fakeCaseUC{}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: uc})
	cc := toolByName(t, tools, "case__create_case")
	gt.Value(t, cc).NotNil().Required()

	_, err := cc.Run(context.Background(), map[string]any{})
	gt.Error(t, err)
	gt.Array(t, uc.createCalls).Length(0)
}

func TestCreateCaseTool_FieldsWithoutSchema(t *testing.T) {
	uc := &fakeCaseUC{}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: uc})
	cc := toolByName(t, tools, "case__create_case")
	gt.Value(t, cc).NotNil().Required()

	_, err := cc.Run(context.Background(), map[string]any{
		"title":  "x",
		"fields": []any{map[string]any{"field_id": "summary", "value": "y"}},
	})
	gt.Error(t, err)
	gt.Array(t, uc.createCalls).Length(0)
}

// ---------------------------------------------------------------------------
// case__update_case
// ---------------------------------------------------------------------------

func TestUpdateCaseTool(t *testing.T) {
	uc := &fakeCaseUC{updateResp: &model.Case{ID: 5, Title: "updated", Status: types.CaseStatusOpen}}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: uc, Schema: testSchema()})
	uct := toolByName(t, tools, "case__update_case")
	gt.Value(t, uct).NotNil().Required()

	out, err := uct.Run(context.Background(), map[string]any{
		"case_id": int64(5),
		"title":   "updated",
		"fields": []any{
			map[string]any{"field_id": "score", "value": "42"},
		},
	})
	gt.NoError(t, err).Required()

	gt.Array(t, uc.updateCalls).Length(1).Required()
	call := uc.updateCalls[0]
	gt.Number(t, call.id).Equal(int64(5))
	gt.Value(t, call.patch.Title).NotNil().Required()
	gt.String(t, *call.patch.Title).Equal("updated")
	gt.Value(t, call.patch.Description).Nil()
	gt.Value(t, call.patch.Fields["score"].Value).Equal(float64(42))

	gt.Number(t, out["id"].(int64)).Equal(int64(5))
	gt.String(t, out["title"].(string)).Equal("updated")
}

func TestUpdateCaseTool_EmptyPatch(t *testing.T) {
	uc := &fakeCaseUC{}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: uc})
	uct := toolByName(t, tools, "case__update_case")
	gt.Value(t, uct).NotNil().Required()

	_, err := uct.Run(context.Background(), map[string]any{"case_id": int64(1)})
	gt.Error(t, err)
	gt.Array(t, uc.updateCalls).Length(0)
}

// ---------------------------------------------------------------------------
// case__close_case
// ---------------------------------------------------------------------------

func TestCloseCaseTool(t *testing.T) {
	uc := &fakeCaseUC{closeResp: &model.Case{ID: 9, Status: types.CaseStatusClosed}}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: uc})
	cl := toolByName(t, tools, "case__close_case")
	gt.Value(t, cl).NotNil().Required()

	out, err := cl.Run(context.Background(), map[string]any{"case_id": int64(9)})
	gt.NoError(t, err).Required()
	gt.Array(t, uc.closeCalls).Length(1).Required()
	gt.Number(t, uc.closeCalls[0]).Equal(int64(9))
	gt.String(t, out["status"].(string)).Equal(types.CaseStatusClosed.String())
}

// ---------------------------------------------------------------------------
// case__list_actions
// ---------------------------------------------------------------------------

func TestListActionsTool(t *testing.T) {
	caseUC := &fakeCaseUC{}
	actionUC := &fakeActionUC{listResp: []*model.Action{
		{ID: 1, CaseID: 3, Title: "a1", Status: types.ActionStatus("open")},
	}}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: caseUC, ActionUC: actionUC})
	la := toolByName(t, tools, "case__list_actions")
	gt.Value(t, la).NotNil().Required()

	out, err := la.Run(context.Background(), map[string]any{"case_id": int64(3)})
	gt.NoError(t, err).Required()
	items, ok := out["actions"].([]map[string]any)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, items).Length(1).Required()
	gt.Number(t, items[0]["id"].(int64)).Equal(int64(1))
	gt.Number(t, items[0]["case_id"].(int64)).Equal(int64(3))
	gt.String(t, items[0]["title"].(string)).Equal("a1")
}

// ---------------------------------------------------------------------------
// case__get_action
// ---------------------------------------------------------------------------

func TestGetActionTool_Success(t *testing.T) {
	caseUC := &fakeCaseUC{casesByID: map[int64]*model.Case{3: {ID: 3, Status: types.CaseStatusOpen}}}
	actionUC := &fakeActionUC{actionsByID: map[int64]*model.Action{
		1: {ID: 1, CaseID: 3, Title: "a1", Description: "d1", Status: types.ActionStatus("open")},
	}}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: caseUC, ActionUC: actionUC})
	ga := toolByName(t, tools, "case__get_action")
	gt.Value(t, ga).NotNil().Required()

	out, err := ga.Run(context.Background(), map[string]any{"case_id": int64(3), "action_id": int64(1)})
	gt.NoError(t, err).Required()
	gt.Number(t, out["id"].(int64)).Equal(int64(1))
	gt.Number(t, out["case_id"].(int64)).Equal(int64(3))
	gt.String(t, out["title"].(string)).Equal("a1")
	gt.String(t, out["description"].(string)).Equal("d1")
}

func TestGetActionTool_CaseMismatch(t *testing.T) {
	caseUC := &fakeCaseUC{casesByID: map[int64]*model.Case{3: {ID: 3, Status: types.CaseStatusOpen}}}
	actionUC := &fakeActionUC{actionsByID: map[int64]*model.Action{
		1: {ID: 1, CaseID: 99, Title: "a1"},
	}}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: caseUC, ActionUC: actionUC})
	ga := toolByName(t, tools, "case__get_action")
	gt.Value(t, ga).NotNil().Required()

	_, err := ga.Run(context.Background(), map[string]any{"case_id": int64(3), "action_id": int64(1)})
	gt.Error(t, err)
}

func TestGetActionTool_ParentCaseAccessDenied(t *testing.T) {
	caseUC := &fakeCaseUC{casesByID: map[int64]*model.Case{3: {ID: 3, AccessDenied: true}}}
	actionUC := &fakeActionUC{actionsByID: map[int64]*model.Action{
		1: {ID: 1, CaseID: 3, Title: "a1"},
	}}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: caseUC, ActionUC: actionUC})
	ga := toolByName(t, tools, "case__get_action")
	gt.Value(t, ga).NotNil().Required()

	_, err := ga.Run(context.Background(), map[string]any{"case_id": int64(3), "action_id": int64(1)})
	gt.Error(t, err)
	// GetAction must not even be reached: the fake would return the action
	// even though its parent case is inaccessible, so the tool itself must be
	// the one refusing.
}

// ---------------------------------------------------------------------------
// case__create_action
// ---------------------------------------------------------------------------

func TestCreateActionTool(t *testing.T) {
	caseUC := &fakeCaseUC{casesByID: map[int64]*model.Case{3: {ID: 3, Status: types.CaseStatusOpen}}}
	actionUC := &fakeActionUC{createResp: &model.Action{ID: 10, CaseID: 3, Title: "new action"}}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: caseUC, ActionUC: actionUC})
	ca := toolByName(t, tools, "case__create_action")
	gt.Value(t, ca).NotNil().Required()

	out, err := ca.Run(context.Background(), map[string]any{
		"case_id":     int64(3),
		"title":       "new action",
		"description": "desc",
	})
	gt.NoError(t, err).Required()
	gt.Array(t, actionUC.createCalls).Length(1).Required()
	gt.Number(t, actionUC.createCalls[0].caseID).Equal(int64(3))
	gt.String(t, actionUC.createCalls[0].title).Equal("new action")
	gt.String(t, actionUC.createCalls[0].description).Equal("desc")
	gt.Number(t, out["id"].(int64)).Equal(int64(10))
}

func TestCreateActionTool_ParentCaseAccessDenied(t *testing.T) {
	caseUC := &fakeCaseUC{casesByID: map[int64]*model.Case{3: {ID: 3, AccessDenied: true}}}
	actionUC := &fakeActionUC{createResp: &model.Action{ID: 10}}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: caseUC, ActionUC: actionUC})
	ca := toolByName(t, tools, "case__create_action")
	gt.Value(t, ca).NotNil().Required()

	_, err := ca.Run(context.Background(), map[string]any{"case_id": int64(3), "title": "x"})
	gt.Error(t, err)
	gt.Array(t, actionUC.createCalls).Length(0)
}

// ---------------------------------------------------------------------------
// case__update_action / case__update_action_status
// ---------------------------------------------------------------------------

func TestUpdateActionTool(t *testing.T) {
	caseUC := &fakeCaseUC{casesByID: map[int64]*model.Case{3: {ID: 3, Status: types.CaseStatusOpen}}}
	actionUC := &fakeActionUC{
		actionsByID: map[int64]*model.Action{1: {ID: 1, CaseID: 3, Title: "old"}},
		updateResp:  &model.Action{ID: 1, CaseID: 3, Title: "new title"},
	}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", ActorID: "U1", CaseUC: caseUC, ActionUC: actionUC})
	ua := toolByName(t, tools, "case__update_action")
	gt.Value(t, ua).NotNil().Required()

	out, err := ua.Run(context.Background(), map[string]any{
		"case_id":   int64(3),
		"action_id": int64(1),
		"title":     "new title",
	})
	gt.NoError(t, err).Required()
	gt.Array(t, actionUC.updateCalls).Length(1).Required()
	call := actionUC.updateCalls[0]
	gt.Number(t, call.actionID).Equal(int64(1))
	gt.Value(t, call.patch.Title).NotNil().Required()
	gt.String(t, *call.patch.Title).Equal("new title")
	gt.Value(t, call.patch.Status).Nil()
	gt.String(t, call.actorID).Equal("U1")
	gt.String(t, out["title"].(string)).Equal("new title")
}

func TestUpdateActionTool_CaseMismatch(t *testing.T) {
	caseUC := &fakeCaseUC{casesByID: map[int64]*model.Case{3: {ID: 3, Status: types.CaseStatusOpen}}}
	actionUC := &fakeActionUC{actionsByID: map[int64]*model.Action{1: {ID: 1, CaseID: 99}}}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", ActorID: "U1", CaseUC: caseUC, ActionUC: actionUC})
	ua := toolByName(t, tools, "case__update_action")
	gt.Value(t, ua).NotNil().Required()

	_, err := ua.Run(context.Background(), map[string]any{"case_id": int64(3), "action_id": int64(1), "title": "x"})
	gt.Error(t, err)
	gt.Array(t, actionUC.updateCalls).Length(0)
}

func TestUpdateActionStatusTool(t *testing.T) {
	caseUC := &fakeCaseUC{casesByID: map[int64]*model.Case{3: {ID: 3, Status: types.CaseStatusOpen}}}
	actionUC := &fakeActionUC{
		actionsByID: map[int64]*model.Action{1: {ID: 1, CaseID: 3}},
		updateResp:  &model.Action{ID: 1, CaseID: 3, Status: types.ActionStatus("done")},
	}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", ActorID: "U2", CaseUC: caseUC, ActionUC: actionUC})
	uas := toolByName(t, tools, "case__update_action_status")
	gt.Value(t, uas).NotNil().Required()

	out, err := uas.Run(context.Background(), map[string]any{
		"case_id":   int64(3),
		"action_id": int64(1),
		"status":    "done",
	})
	gt.NoError(t, err).Required()
	gt.Array(t, actionUC.updateCalls).Length(1).Required()
	call := actionUC.updateCalls[0]
	gt.Value(t, call.patch.Status).NotNil().Required()
	gt.Value(t, *call.patch.Status).Equal(types.ActionStatus("done"))
	gt.Value(t, call.patch.Title).Nil()
	gt.String(t, call.actorID).Equal("U2")
	gt.String(t, out["status"].(string)).Equal("done")
}

func TestUpdateActionStatusTool_MissingStatus(t *testing.T) {
	caseUC := &fakeCaseUC{casesByID: map[int64]*model.Case{3: {ID: 3, Status: types.CaseStatusOpen}}}
	actionUC := &fakeActionUC{actionsByID: map[int64]*model.Action{1: {ID: 1, CaseID: 3}}}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: caseUC, ActionUC: actionUC})
	uas := toolByName(t, tools, "case__update_action_status")
	gt.Value(t, uas).NotNil().Required()

	_, err := uas.Run(context.Background(), map[string]any{"case_id": int64(3), "action_id": int64(1)})
	gt.Error(t, err)
	gt.Array(t, actionUC.updateCalls).Length(0)
}

// ---------------------------------------------------------------------------
// case__add_action_step
// ---------------------------------------------------------------------------

func TestAddActionStepTool(t *testing.T) {
	caseUC := &fakeCaseUC{casesByID: map[int64]*model.Case{3: {ID: 3, Status: types.CaseStatusOpen}}}
	actionUC := &fakeActionUC{
		actionsByID: map[int64]*model.Action{1: {ID: 1, CaseID: 3}},
		addStepResp: &model.ActionStep{ID: "step-1", ActionID: 1, Title: "do the thing"},
	}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", ActorID: "U3", CaseUC: caseUC, ActionUC: actionUC})
	as := toolByName(t, tools, "case__add_action_step")
	gt.Value(t, as).NotNil().Required()

	out, err := as.Run(context.Background(), map[string]any{
		"case_id":   int64(3),
		"action_id": int64(1),
		"title":     "do the thing",
	})
	gt.NoError(t, err).Required()
	gt.Array(t, actionUC.addStepCalls).Length(1).Required()
	call := actionUC.addStepCalls[0]
	gt.Number(t, call.actionID).Equal(int64(1))
	gt.String(t, call.title).Equal("do the thing")
	gt.String(t, call.actorID).Equal("U3")
	gt.String(t, out["id"].(string)).Equal("step-1")
	gt.Bool(t, out["done"].(bool)).False()
}

func TestAddActionStepTool_CaseMismatch(t *testing.T) {
	caseUC := &fakeCaseUC{casesByID: map[int64]*model.Case{3: {ID: 3, Status: types.CaseStatusOpen}}}
	actionUC := &fakeActionUC{actionsByID: map[int64]*model.Action{1: {ID: 1, CaseID: 4}}}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: caseUC, ActionUC: actionUC})
	as := toolByName(t, tools, "case__add_action_step")
	gt.Value(t, as).NotNil().Required()

	_, err := as.Run(context.Background(), map[string]any{"case_id": int64(3), "action_id": int64(1), "title": "x"})
	gt.Error(t, err)
	gt.Array(t, actionUC.addStepCalls).Length(0)
}

// ---------------------------------------------------------------------------
// case__set_action_step_done
// ---------------------------------------------------------------------------

func TestSetActionStepDoneTool(t *testing.T) {
	caseUC := &fakeCaseUC{casesByID: map[int64]*model.Case{3: {ID: 3, Status: types.CaseStatusOpen}}}
	actionUC := &fakeActionUC{
		actionsByID: map[int64]*model.Action{1: {ID: 1, CaseID: 3}},
		setDoneResp: &model.ActionStep{ID: "step-1", ActionID: 1, Title: "do the thing"},
	}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", ActorID: "U4", CaseUC: caseUC, ActionUC: actionUC})
	sd := toolByName(t, tools, "case__set_action_step_done")
	gt.Value(t, sd).NotNil().Required()

	_, err := sd.Run(context.Background(), map[string]any{
		"case_id":   int64(3),
		"action_id": int64(1),
		"step_id":   "step-1",
		"done":      true,
	})
	gt.NoError(t, err).Required()
	gt.Array(t, actionUC.setDoneCalls).Length(1).Required()
	call := actionUC.setDoneCalls[0]
	gt.Number(t, call.actionID).Equal(int64(1))
	gt.String(t, call.stepID).Equal("step-1")
	gt.Bool(t, call.done).True()
	gt.String(t, call.actorID).Equal("U4")
}

func TestSetActionStepDoneTool_MissingStepID(t *testing.T) {
	caseUC := &fakeCaseUC{casesByID: map[int64]*model.Case{3: {ID: 3, Status: types.CaseStatusOpen}}}
	actionUC := &fakeActionUC{actionsByID: map[int64]*model.Action{1: {ID: 1, CaseID: 3}}}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: caseUC, ActionUC: actionUC})
	sd := toolByName(t, tools, "case__set_action_step_done")
	gt.Value(t, sd).NotNil().Required()

	_, err := sd.Run(context.Background(), map[string]any{"case_id": int64(3), "action_id": int64(1), "done": true})
	gt.Error(t, err)
	gt.Array(t, actionUC.setDoneCalls).Length(0)
}

// ---------------------------------------------------------------------------
// Error propagation
// ---------------------------------------------------------------------------

func TestUpdateCaseTool_PropagatesUseCaseError(t *testing.T) {
	sentinel := goerr.New("boom")
	uc := &fakeCaseUC{updateErr: sentinel}
	tools := casemulti.New(casemulti.Deps{WorkspaceID: "ws", CaseUC: uc})
	uct := toolByName(t, tools, "case__update_case")
	gt.Value(t, uct).NotNil().Required()

	_, err := uct.Run(context.Background(), map[string]any{"case_id": int64(1), "title": "x"})
	gt.Error(t, err).Is(sentinel)
}
