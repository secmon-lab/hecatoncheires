// Package casemulti exposes a cross-case ("workspace-scoped") gollem tool set
// for agent turns that operate across many Cases in one workspace, rather
// than a single Case pinned at construction time (contrast
// pkg/agent/tool/casewriter and pkg/agent/tool/core, both of which are built
// with one fixed CaseID and never take a case identifier at call time). Every
// tool in this package takes case_id as a call-time argument instead, so one
// turn can read/create/update several Cases and their Actions.
//
// # Access control
//
// The host is REQUIRED to inject an auth token for the mentioning user
// (auth.ContextWithToken(ctx, &auth.Token{Sub: Deps.ActorID})) for the whole
// turn before invoking these tools. CaseUsecase's methods read that token
// internally (see pkg/usecase/case.go, case_access.go) to enforce private-case
// membership on every read and write, so case__list_cases / case__get_case /
// every write tool deny or filter automatically, exactly like every other
// entry point (GraphQL, Slack). Tools MUST surface an access-denied error
// from the usecase back to the model as a tool error, never swallow it.
//
// case__list_cases additionally filters out any Case the usecase returns with
// AccessDenied set (the usecase's ListCases restricts rather than omits, since
// callers like the Cases page want to render a redacted row; this tool set
// wants those Cases invisible instead) and case__get_case turns an
// AccessDenied Case into a tool error rather than returning its stripped
// fields.
//
// Action reads (case__list_actions) go through
// ActionUsecase.GetActionsByCase, whose real implementation
// (usecase.ActionUseCase.GetActionsByCase) already enforces the identical
// ctx-token membership check, so no extra work is needed here. Action
// mutations (case__update_action, case__update_action_status,
// case__add_action_step, case__set_action_step_done) additionally verify —
// via ensureActionBelongsToCase — that the given case_id actually owns
// action_id, both as a load-bearing access check (mirroring case__get_case's
// AccessDenied handling before ever touching the action) and as a safety net
// against the model confusing action IDs across cases when it is tracking
// several at once.
//
// # ActorRef import-cycle note
//
// The real actor-attribution type (usecase.ActorRef / usecase.ActorKind)
// lives in package pkg/usecase, and pkg/usecase already imports
// pkg/agent/tool/core and pkg/agent/tool/casewriter (to adapt onto their
// narrow tool interfaces via pkg/usecase/*_tool_adapter.go). If this package
// imported pkg/usecase too, wiring a casemulti adapter there would close a
// cycle (usecase -> casemulti -> usecase), so casemulti must not import
// pkg/usecase at all — CaseUsecase / ActionUsecase below are purely
// structural interfaces satisfied by an adapter that lives in pkg/usecase
// (mirroring case_tool_adapter.go / action_tool_adapter.go), not by
// *usecase.CaseUseCase / *usecase.ActionUseCase directly (their real
// UpdateCase / UpdateAction methods take usecase-package-local patch/input
// structs — CaseUpdate / UpdateActionInput — that this package cannot name).
//
// ActionUsecase's write methods therefore take a plain actorID string
// (deps.ActorID, the mentioning Slack user) instead of usecase.ActorRef. The
// adapter a host wires in is the piece that turns actorID into
// usecase.ActorRef{Kind: usecase.ActorKindSlackUser, ID: actorID} before
// calling the real *usecase.ActionUseCase / *usecase.ActionStepUseCase
// methods — deliberately NEVER ActorKindSystem: unlike pkg/agent/tool/core's
// sub-agent tools (which pin ActorKindSystem because an investigation
// sub-agent is not a Slack user), every casemulti write acts on behalf of the
// user who mentioned the agent, and ActorKindSlackUser is what makes
// ActionEvent history / Slack change notifications attribute the change to
// that user and enforces private-case access as defense in depth even if a
// future caller ever dispatches the write on a context that lost its auth
// token. case__create_action is the one exception: the real
// usecase.ActionUseCase.CreateAction method has no actor parameter at all —
// its access check and its ActionEvent "created by" attribution are both
// ctx-token-only — so ActionUsecase.CreateAction below carries no actorID
// parameter; the host's ctx-token injection is what attributes it.
package casemulti

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// CaseUsecase is the narrow surface of usecase.CaseUseCase the cross-case
// tools depend on. Every method's access control comes from the ctx auth
// token the host injects (see the package doc); none of them take an actor
// parameter.
type CaseUsecase interface {
	// ListCases is invoked by case__list_cases.
	ListCases(ctx context.Context, workspaceID string, status *types.CaseStatus) ([]*model.Case, error)
	// GetCase is invoked by case__get_case and by ensureActionBelongsToCase's
	// access pre-check ahead of every action read/write below.
	GetCase(ctx context.Context, workspaceID string, id int64) (*model.Case, error)
	// CreateCase is invoked by case__create_case. The real
	// usecase.CaseUseCase.CreateCase signature additionally takes isTest,
	// sourceTeamID and requestKey; the adapter fixes these to false, "" and ""
	// respectively — agent-tool creation has no Slack-modal double-submit to
	// dedup, mirroring the GraphQL mutation's call site.
	CreateCase(ctx context.Context, workspaceID string, title, description string, assigneeIDs []string, fieldValues map[string]model.FieldValue, isPrivate bool) (*model.Case, error)
	// UpdateCase is invoked by case__update_case.
	UpdateCase(ctx context.Context, workspaceID string, id int64, patch CaseUpdate) (*model.Case, error)
	// CloseCase is invoked by case__close_case.
	CloseCase(ctx context.Context, workspaceID string, id int64) (*model.Case, error)
}

// CaseUpdate mirrors the partial-update shape of usecase.CaseUpdate (compare
// pkg/agent/tool/casewriter.CaseUpdate, which exists for the identical
// import-cycle reason). nil means "preserve the existing value".
type CaseUpdate struct {
	Title       *string
	Description *string
	// nil means "preserve all stored field values"; a non-nil map merges its
	// entries on top of the existing ones.
	Fields map[string]model.FieldValue
}

// ActionUsecase is the narrow surface of usecase.ActionUseCase /
// usecase.ActionStepUseCase the cross-case action tools depend on. Write
// methods (other than CreateAction, see the package doc) take actorID — the
// Slack user id that triggered the call, i.e. deps.ActorID — instead of
// usecase.ActorRef.
type ActionUsecase interface {
	// GetActionsByCase is invoked by case__list_actions. The real
	// implementation already enforces ctx-token private-case membership, so
	// no separate access check is needed here.
	GetActionsByCase(ctx context.Context, workspaceID string, caseID int64, opts interfaces.ActionListOptions) ([]*model.Action, error)
	// GetAction is invoked by case__get_action (through
	// ensureActionBelongsToCase). Its real implementation does NOT check
	// parent-Case privacy by itself when called with no opts, so callers here
	// always pair it with a prior CaseUsecase.GetCase access check.
	GetAction(ctx context.Context, workspaceID string, id int64, opts ...interfaces.ActionListOptions) (*model.Action, error)
	// CreateAction is invoked by case__create_action. See the package doc for
	// why this method carries no actorID.
	CreateAction(ctx context.Context, workspaceID string, caseID int64, title, description string) (*model.Action, error)
	// UpdateAction is invoked by both case__update_action and
	// case__update_action_status (the latter sets only patch.Status), mirroring
	// how pkg/agent/tool/core's update_action_status tool reuses the same
	// underlying UpdateAction call.
	UpdateAction(ctx context.Context, workspaceID string, actionID int64, patch ActionUpdate, actorID string) (*model.Action, error)
	// AddActionStep is invoked by case__add_action_step.
	AddActionStep(ctx context.Context, workspaceID string, actionID int64, title string, actorID string) (*model.ActionStep, error)
	// SetActionStepDone is invoked by case__set_action_step_done.
	SetActionStepDone(ctx context.Context, workspaceID string, actionID int64, stepID string, done bool, actorID string) (*model.ActionStep, error)
}

// ActionUpdate is the partial-update shape for ActionUsecase.UpdateAction.
// nil means "preserve the existing value".
type ActionUpdate struct {
	Title       *string
	Description *string
	Status      *types.ActionStatus
}

// Deps groups the dependencies the casemulti tools need. WorkspaceID and
// ActorID are pinned at construction (the agent turn always runs in one
// workspace on behalf of one mentioning user); case_id / action_id are
// supplied by the model at call time.
type Deps struct {
	// WorkspaceID is the workspace every tool call operates in.
	WorkspaceID string
	// ActorID is the mentioning Slack user id. It is used as the actorID
	// argument on every ActionUsecase write method (see the package doc's
	// ActorRef import-cycle note) and is also the identity the host must have
	// already placed in ctx via auth.ContextWithToken for CaseUsecase's
	// access control to apply.
	ActorID string
	// CaseUC backs every case__* read and case write tool. New returns no
	// tools at all when this is nil, so hosts can wire casemulti
	// unconditionally and degrade safely.
	CaseUC CaseUsecase
	// ActionUC backs the case__*_action* tools. nil disables just those
	// tools (a workspace agent that only needs Case-level tools can leave
	// this unset).
	ActionUC ActionUsecase
	// Schema resolves field types for case__create_case / case__update_case's
	// `fields` parameter coercion. nil disables custom-field arguments (they
	// then error out at runtime, matching casewriter's Deps.Schema contract).
	Schema *config.FieldSchema
}

// New returns the cross-case tools. Returns nil (empty) when CaseUC == nil so
// hosts can wire it unconditionally and degrade safely.
func New(deps Deps) []gollem.Tool {
	if deps.CaseUC == nil {
		return nil
	}

	tools := []gollem.Tool{
		&listCasesTool{deps: deps},
		&getCaseTool{deps: deps},
		&createCaseTool{deps: deps},
		&updateCaseTool{deps: deps},
		&closeCaseTool{deps: deps},
	}

	if deps.ActionUC != nil {
		tools = append(tools,
			&listActionsTool{deps: deps},
			&getActionTool{deps: deps},
			&createActionTool{deps: deps},
			&updateActionTool{deps: deps},
			&updateActionStatusTool{deps: deps},
			&addActionStepTool{deps: deps},
			&setActionStepDoneTool{deps: deps},
		)
	}

	return tools
}

// loadAccessibleCase fetches the Case and turns an AccessDenied result into a
// tool error instead of returning the stripped fields, matching
// case__get_case's contract. Used directly by case__get_case and as the
// access pre-check ahead of every per-action tool.
func loadAccessibleCase(ctx context.Context, deps Deps, caseID int64) (*model.Case, error) {
	c, err := deps.CaseUC.GetCase(ctx, deps.WorkspaceID, caseID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get case",
			goerr.V("workspace_id", deps.WorkspaceID),
			goerr.V("case_id", caseID))
	}
	// A CaseUsecase implementation may return (nil, nil) for a missing case;
	// guard before dereferencing so the tool errors cleanly instead of panicking.
	if c == nil {
		return nil, goerr.New("case not found", goerr.V("case_id", caseID))
	}
	if c.AccessDenied {
		return nil, goerr.New("case is private and not accessible to the current user",
			goerr.V("case_id", caseID))
	}
	return c, nil
}

// ensureActionBelongsToCase verifies the caller may access caseID and that
// actionID's parent Case is actually caseID, returning the loaded Action.
// This is both the access gate (the real ActionUsecase.GetAction does not
// check parent-Case privacy by itself) and a safety net against the model
// confusing action IDs across cases while tracking several at once.
func ensureActionBelongsToCase(ctx context.Context, deps Deps, caseID, actionID int64) (*model.Action, error) {
	if _, err := loadAccessibleCase(ctx, deps, caseID); err != nil {
		return nil, err
	}
	a, err := deps.ActionUC.GetAction(ctx, deps.WorkspaceID, actionID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get action",
			goerr.V("workspace_id", deps.WorkspaceID),
			goerr.V("action_id", actionID))
	}
	// Guard against an ActionUsecase implementation returning (nil, nil).
	if a == nil {
		return nil, goerr.New("action not found", goerr.V("action_id", actionID))
	}
	if a.CaseID != caseID {
		return nil, goerr.New("action does not belong to the given case",
			goerr.V("case_id", caseID),
			goerr.V("action_id", actionID),
			goerr.V("actual_case_id", a.CaseID))
	}
	return a, nil
}

// caseToListMap renders a Case as a compact map for case__list_cases entries.
func caseToListMap(c *model.Case) map[string]any {
	return map[string]any{
		"id":           c.ID,
		"title":        c.Title,
		"status":       c.Status.String(),
		"is_private":   c.IsPrivate,
		"assignee_ids": c.AssigneeIDs,
	}
}

// caseToDetailMap renders a Case's full detail for case__get_case /
// case__create_case / case__update_case / case__close_case responses.
func caseToDetailMap(c *model.Case) map[string]any {
	return map[string]any{
		"id":           c.ID,
		"title":        c.Title,
		"description":  c.Description,
		"status":       c.Status.String(),
		"is_private":   c.IsPrivate,
		"reporter_id":  c.ReporterID,
		"assignee_ids": c.AssigneeIDs,
		"field_values": renderFieldValues(c.FieldValues),
	}
}

// actionToListMap renders an Action as a compact map for case__list_actions entries.
func actionToListMap(a *model.Action) map[string]any {
	item := map[string]any{
		"id":          a.ID,
		"case_id":     a.CaseID,
		"title":       a.Title,
		"status":      a.Status.String(),
		"assignee_id": a.AssigneeID,
		"archived":    a.IsArchived(),
	}
	if a.DueDate != nil {
		item["due_date"] = a.DueDate.Format(time.RFC3339)
	}
	return item
}

// actionToDetailMap renders an Action's full detail for case__get_action /
// case__create_action / case__update_action / case__update_action_status
// responses.
func actionToDetailMap(a *model.Action) map[string]any {
	m := map[string]any{
		"id":          a.ID,
		"case_id":     a.CaseID,
		"title":       a.Title,
		"description": a.Description,
		"status":      a.Status.String(),
		"assignee_id": a.AssigneeID,
		"archived":    a.IsArchived(),
		"created_at":  a.CreatedAt.Format(time.RFC3339),
		"updated_at":  a.UpdatedAt.Format(time.RFC3339),
	}
	if a.DueDate != nil {
		m["due_date"] = a.DueDate.Format(time.RFC3339)
	}
	return m
}

// actionStepToMap renders an ActionStep for case__add_action_step /
// case__set_action_step_done responses.
func actionStepToMap(s *model.ActionStep) map[string]any {
	m := map[string]any{
		"id":         s.ID,
		"action_id":  s.ActionID,
		"title":      s.Title,
		"done":       s.IsDone(),
		"created_by": s.CreatedBy,
		"created_at": s.CreatedAt.Format(time.RFC3339),
		"updated_at": s.UpdatedAt.Format(time.RFC3339),
	}
	if s.DoneAt != nil {
		m["done_at"] = s.DoneAt.Format(time.RFC3339)
	}
	if s.DoneBy != "" {
		m["done_by"] = s.DoneBy
	}
	return m
}

// renderFieldValues flattens stored field values into a plain map for tool
// responses (compare pkg/agent/tool/casewriter's identical helper — the two
// packages cannot share unexported helpers across package boundaries and this
// is presentation plumbing, not business logic).
func renderFieldValues(fields map[string]model.FieldValue) map[string]any {
	out := make(map[string]any, len(fields))
	for id, fv := range fields {
		out[id] = fv.Value
	}
	return out
}

// parseFieldInputs converts the gollem-decoded `fields` argument (a []any of
// per-entry maps) into model.FieldInput. gollem decodes arrays as []any and
// objects as map[string]any. Compare
// pkg/agent/tool/casewriter.parseFieldInputs (duplicated for the same reason
// as renderFieldValues above).
func parseFieldInputs(v any) ([]model.FieldInput, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, goerr.New("fields must be an array", goerr.V("type", typeOf(v)))
	}
	out := make([]model.FieldInput, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, goerr.New("each field entry must be an object", goerr.V("type", typeOf(item)))
		}
		fieldID, ok := m["field_id"].(string)
		if !ok || fieldID == "" {
			return nil, goerr.New("each field entry requires a non-empty field_id")
		}
		fi := model.FieldInput{FieldID: fieldID}
		if val, ok := m["value"]; ok && val != nil {
			s, ok := val.(string)
			if !ok {
				return nil, goerr.New("field value must be a string", goerr.V("field_id", fieldID), goerr.V("type", typeOf(val)))
			}
			fi.Value = s
		}
		if vals, ok := m["values"]; ok && vals != nil {
			ss, err := toStringSlice(vals)
			if err != nil {
				return nil, goerr.Wrap(err, "field values invalid", goerr.V("field_id", fieldID))
			}
			fi.Values = ss
		}
		out = append(out, fi)
	}
	return out, nil
}

// toStringSlice coerces a tool argument value into []string. gollem decodes
// arrays as []any, so we accept that shape plus the rare backend that returns
// []string directly.
func toStringSlice(v any) ([]string, error) {
	switch a := v.(type) {
	case []string:
		return a, nil
	case []any:
		out := make([]string, 0, len(a))
		for _, item := range a {
			s, ok := item.(string)
			if !ok {
				return nil, goerr.New("array item must be string", goerr.V("type", typeOf(item)))
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, goerr.New("value must be an array of strings", goerr.V("type", typeOf(v)))
	}
}

func typeOf(v any) string {
	if v == nil {
		return "nil"
	}
	return fmt.Sprintf("%T", v)
}

// fieldsParameter is the shared `fields` gollem.Parameter schema for
// case__create_case / case__update_case.
func fieldsParameter() *gollem.Parameter {
	return &gollem.Parameter{
		Type: gollem.TypeArray,
		Description: "Custom field assignments. Each entry sets one field defined in " +
			"the workspace field schema (see the system prompt for ids, types, and " +
			"option ids).",
		Items: &gollem.Parameter{
			Type: gollem.TypeObject,
			Properties: map[string]*gollem.Parameter{
				"field_id": {Type: gollem.TypeString, Description: "The field id from the workspace schema.", Required: true},
				"value":    {Type: gollem.TypeString, Description: "Scalar value (text / number / url / date / single select option id / single user id)."},
				"values":   {Type: gollem.TypeArray, Description: "Multi value (multi-select option ids / multi-user ids).", Items: &gollem.Parameter{Type: gollem.TypeString}},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Case read tools
// ---------------------------------------------------------------------------

type listCasesTool struct {
	deps Deps
}

func (t *listCasesTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "case__list_cases",
		Description: "List cases in the current workspace. Private cases the " +
			"current user cannot access are omitted entirely.",
		Parameters: map[string]*gollem.Parameter{
			"status": {
				Type:        gollem.TypeString,
				Description: "Optional case status filter.",
				Enum:        []string{types.CaseStatusDraft.String(), types.CaseStatusOpen.String(), types.CaseStatusClosed.String()},
			},
		},
	}
}

func (t *listCasesTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Listing cases...")

	var status *types.CaseStatus
	if v, ok := args["status"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return nil, goerr.New("status must be a string", goerr.V("type", typeOf(v)))
		}
		if s != "" {
			parsed, err := types.ParseCaseStatus(s)
			if err != nil {
				return nil, goerr.Wrap(err, "invalid status", goerr.V("status", s))
			}
			status = &parsed
		}
	}

	cases, err := t.deps.CaseUC.ListCases(ctx, t.deps.WorkspaceID, status)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list cases", goerr.V("workspace_id", t.deps.WorkspaceID))
	}

	items := make([]map[string]any, 0, len(cases))
	for _, c := range cases {
		if c == nil {
			continue
		}
		// FILTER OUT restricted private cases rather than exposing the
		// stripped/redacted row: this tool's LLM caller must never even learn
		// such a case exists.
		if c.AccessDenied {
			continue
		}
		items = append(items, caseToListMap(c))
	}

	return map[string]any{"cases": items}, nil
}

type getCaseTool struct {
	deps Deps
}

func (t *getCaseTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "case__get_case",
		Description: "Get the full detail of a case by its ID, including custom field values.",
		Parameters: map[string]*gollem.Parameter{
			"case_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the case to retrieve.",
				Required:    true,
			},
		},
	}
}

func (t *getCaseTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	caseID, err := tool.ExtractInt64(args, "case_id")
	if err != nil {
		return nil, err
	}

	tool.Update(ctx, fmt.Sprintf("Getting case #%d...", caseID))

	c, err := loadAccessibleCase(ctx, t.deps, caseID)
	if err != nil {
		return nil, err
	}

	return caseToDetailMap(c), nil
}

// ---------------------------------------------------------------------------
// Case write tools
// ---------------------------------------------------------------------------

type createCaseTool struct {
	deps Deps
}

func (t *createCaseTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "case__create_case",
		Description: "Create a new case in the current workspace.",
		Parameters: map[string]*gollem.Parameter{
			"title": {
				Type:        gollem.TypeString,
				Description: "Title of the new case.",
				Required:    true,
			},
			"description": {
				Type:        gollem.TypeString,
				Description: "Description of the new case.",
			},
			"assignees": {
				Type:        gollem.TypeArray,
				Description: "Slack user IDs to assign to the new case.",
				Items:       &gollem.Parameter{Type: gollem.TypeString},
			},
			"fields": fieldsParameter(),
			"is_private": {
				Type:        gollem.TypeBoolean,
				Description: "Whether the new case should be private (visible only to its channel members). Defaults to false.",
			},
		},
	}
}

func (t *createCaseTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	title, _ := args["title"].(string)
	if title == "" {
		return nil, goerr.New("title is required")
	}
	description, _ := args["description"].(string)

	var assigneeIDs []string
	if v, ok := args["assignees"]; ok && v != nil {
		ids, err := toStringSlice(v)
		if err != nil {
			return nil, goerr.Wrap(err, "assignees invalid")
		}
		assigneeIDs = ids
	}

	var fieldValues map[string]model.FieldValue
	if v, ok := args["fields"]; ok && v != nil {
		if t.deps.Schema == nil {
			return nil, goerr.New("this workspace has no custom fields; the fields parameter is not supported")
		}
		inputs, err := parseFieldInputs(v)
		if err != nil {
			return nil, goerr.Wrap(err, "fields invalid")
		}
		coerced, violations := model.CoerceFieldInputs(t.deps.Schema, inputs)
		if len(violations) > 0 {
			return nil, goerr.New("invalid field value(s):\n- " + strings.Join(violations, "\n- "))
		}
		fieldValues = coerced
	}

	isPrivate, _ := args["is_private"].(bool)

	tool.Update(ctx, fmt.Sprintf("Creating case: %s", title))

	created, err := t.deps.CaseUC.CreateCase(ctx, t.deps.WorkspaceID, title, description, assigneeIDs, fieldValues, isPrivate)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create case", goerr.V("workspace_id", t.deps.WorkspaceID))
	}

	return caseToDetailMap(created), nil
}

type updateCaseTool struct {
	deps Deps
}

func (t *updateCaseTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "case__update_case",
		Description: "Update a case's title, description, or custom field values. " +
			"This tool cannot change the case status (use case__close_case) or its " +
			"assignees. Submit only the fields you intend to change; omit the rest " +
			"to preserve them.",
		Parameters: map[string]*gollem.Parameter{
			"case_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the case to update.",
				Required:    true,
			},
			"title": {
				Type:        gollem.TypeString,
				Description: "New title for the case (full replacement). Omit to preserve the existing title.",
			},
			"description": {
				Type:        gollem.TypeString,
				Description: "New description (full replacement). Omit to preserve the existing description.",
			},
			"fields": fieldsParameter(),
		},
	}
}

func (t *updateCaseTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	caseID, err := tool.ExtractInt64(args, "case_id")
	if err != nil {
		return nil, err
	}

	var patch CaseUpdate
	hasUpdate := false

	if v, ok := args["title"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return nil, goerr.New("title must be a string", goerr.V("type", typeOf(v)))
		}
		patch.Title = &s
		hasUpdate = true
	}

	if v, ok := args["description"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return nil, goerr.New("description must be a string", goerr.V("type", typeOf(v)))
		}
		patch.Description = &s
		hasUpdate = true
	}

	if v, ok := args["fields"]; ok && v != nil {
		if t.deps.Schema == nil {
			return nil, goerr.New("this workspace has no custom fields; the fields parameter is not supported")
		}
		inputs, err := parseFieldInputs(v)
		if err != nil {
			return nil, goerr.Wrap(err, "fields invalid")
		}
		coerced, violations := model.CoerceFieldInputs(t.deps.Schema, inputs)
		if len(violations) > 0 {
			return nil, goerr.New("invalid field value(s):\n- " + strings.Join(violations, "\n- "))
		}
		patch.Fields = coerced
		hasUpdate = true
	}

	if !hasUpdate {
		return nil, goerr.New("update_case requires at least one of title, description, fields")
	}

	tool.Update(ctx, fmt.Sprintf("Updating case #%d...", caseID))

	updated, err := t.deps.CaseUC.UpdateCase(ctx, t.deps.WorkspaceID, caseID, patch)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update case",
			goerr.V("workspace_id", t.deps.WorkspaceID),
			goerr.V("case_id", caseID))
	}

	return caseToDetailMap(updated), nil
}

type closeCaseTool struct {
	deps Deps
}

func (t *closeCaseTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "case__close_case",
		Description: "Close a case (lifecycle -> CLOSED). Only do this when the work is genuinely resolved.",
		Parameters: map[string]*gollem.Parameter{
			"case_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the case to close.",
				Required:    true,
			},
		},
	}
}

func (t *closeCaseTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	caseID, err := tool.ExtractInt64(args, "case_id")
	if err != nil {
		return nil, err
	}

	tool.Update(ctx, fmt.Sprintf("Closing case #%d...", caseID))

	updated, err := t.deps.CaseUC.CloseCase(ctx, t.deps.WorkspaceID, caseID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to close case",
			goerr.V("workspace_id", t.deps.WorkspaceID),
			goerr.V("case_id", caseID))
	}

	return map[string]any{
		"id":     updated.ID,
		"status": updated.Status.String(),
	}, nil
}

// ---------------------------------------------------------------------------
// Action read tools
// ---------------------------------------------------------------------------

type listActionsTool struct {
	deps Deps
}

func (t *listActionsTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "case__list_actions",
		Description: "List the (non-archived) actions of a case.",
		Parameters: map[string]*gollem.Parameter{
			"case_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the case whose actions to list.",
				Required:    true,
			},
		},
	}
}

func (t *listActionsTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	caseID, err := tool.ExtractInt64(args, "case_id")
	if err != nil {
		return nil, err
	}

	tool.Update(ctx, fmt.Sprintf("Listing actions of case #%d...", caseID))

	actions, err := t.deps.ActionUC.GetActionsByCase(ctx, t.deps.WorkspaceID, caseID, interfaces.ActionListOptions{})
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list actions",
			goerr.V("workspace_id", t.deps.WorkspaceID),
			goerr.V("case_id", caseID))
	}

	items := make([]map[string]any, 0, len(actions))
	for _, a := range actions {
		if a == nil {
			continue
		}
		items = append(items, actionToListMap(a))
	}
	return map[string]any{"actions": items}, nil
}

type getActionTool struct {
	deps Deps
}

func (t *getActionTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "case__get_action",
		Description: "Get the full detail of an action by its case and action IDs.",
		Parameters: map[string]*gollem.Parameter{
			"case_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the case the action belongs to.",
				Required:    true,
			},
			"action_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the action to retrieve.",
				Required:    true,
			},
		},
	}
}

func (t *getActionTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	caseID, err := tool.ExtractInt64(args, "case_id")
	if err != nil {
		return nil, err
	}
	actionID, err := tool.ExtractInt64(args, "action_id")
	if err != nil {
		return nil, err
	}

	tool.Update(ctx, fmt.Sprintf("Getting action #%d of case #%d...", actionID, caseID))

	a, err := ensureActionBelongsToCase(ctx, t.deps, caseID, actionID)
	if err != nil {
		return nil, err
	}

	return actionToDetailMap(a), nil
}

// ---------------------------------------------------------------------------
// Action write tools
// ---------------------------------------------------------------------------

type createActionTool struct {
	deps Deps
}

func (t *createActionTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "case__create_action",
		Description: "Create a new action under a case.",
		Parameters: map[string]*gollem.Parameter{
			"case_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the case the new action belongs to.",
				Required:    true,
			},
			"title": {
				Type:        gollem.TypeString,
				Description: "Title of the action.",
				Required:    true,
			},
			"description": {
				Type:        gollem.TypeString,
				Description: "Detailed description of the action.",
			},
		},
	}
}

func (t *createActionTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	caseID, err := tool.ExtractInt64(args, "case_id")
	if err != nil {
		return nil, err
	}
	title, _ := args["title"].(string)
	if title == "" {
		return nil, goerr.New("title is required")
	}
	description, _ := args["description"].(string)

	// The parent case must be accessible before creating an action under it,
	// mirroring case__get_action's access-first ordering (CreateAction itself
	// also enforces this via the ctx auth token, but failing loudly here keeps
	// the reported error consistent with the rest of the tool set).
	if _, err := loadAccessibleCase(ctx, t.deps, caseID); err != nil {
		return nil, err
	}

	tool.Update(ctx, fmt.Sprintf("Creating action under case #%d: %s", caseID, title))

	created, err := t.deps.ActionUC.CreateAction(ctx, t.deps.WorkspaceID, caseID, title, description)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create action",
			goerr.V("workspace_id", t.deps.WorkspaceID),
			goerr.V("case_id", caseID))
	}

	return actionToDetailMap(created), nil
}

type updateActionTool struct {
	deps Deps
}

func (t *updateActionTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "case__update_action",
		Description: "Update an existing action's title and/or description. Use case__update_action_status to change its status.",
		Parameters: map[string]*gollem.Parameter{
			"case_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the case the action belongs to.",
				Required:    true,
			},
			"action_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the action to update.",
				Required:    true,
			},
			"title": {
				Type:        gollem.TypeString,
				Description: "New title for the action. Omit to preserve the existing title.",
			},
			"description": {
				Type:        gollem.TypeString,
				Description: "New description for the action. Omit to preserve the existing description.",
			},
		},
	}
}

func (t *updateActionTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	caseID, err := tool.ExtractInt64(args, "case_id")
	if err != nil {
		return nil, err
	}
	actionID, err := tool.ExtractInt64(args, "action_id")
	if err != nil {
		return nil, err
	}

	if _, err := ensureActionBelongsToCase(ctx, t.deps, caseID, actionID); err != nil {
		return nil, err
	}

	var patch ActionUpdate
	hasUpdate := false

	if v, ok := args["title"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return nil, goerr.New("title must be a string", goerr.V("type", typeOf(v)))
		}
		if s != "" {
			patch.Title = &s
			hasUpdate = true
		}
	}

	if v, ok := args["description"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return nil, goerr.New("description must be a string", goerr.V("type", typeOf(v)))
		}
		patch.Description = &s
		hasUpdate = true
	}

	if !hasUpdate {
		return nil, goerr.New("update_action requires at least one of title, description")
	}

	tool.Update(ctx, fmt.Sprintf("Updating action #%d...", actionID))

	updated, err := t.deps.ActionUC.UpdateAction(ctx, t.deps.WorkspaceID, actionID, patch, t.deps.ActorID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update action",
			goerr.V("workspace_id", t.deps.WorkspaceID),
			goerr.V("case_id", caseID),
			goerr.V("action_id", actionID))
	}

	return actionToDetailMap(updated), nil
}

type updateActionStatusTool struct {
	deps Deps
}

func (t *updateActionStatusTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "case__update_action_status",
		Description: "Update the status of an existing action.",
		Parameters: map[string]*gollem.Parameter{
			"case_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the case the action belongs to.",
				Required:    true,
			},
			"action_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the action to update.",
				Required:    true,
			},
			"status": {
				Type:        gollem.TypeString,
				Description: "New status for the action, one of the workspace's configured action statuses.",
				Required:    true,
			},
		},
	}
}

func (t *updateActionStatusTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	caseID, err := tool.ExtractInt64(args, "case_id")
	if err != nil {
		return nil, err
	}
	actionID, err := tool.ExtractInt64(args, "action_id")
	if err != nil {
		return nil, err
	}

	statusStr, _ := args["status"].(string)
	if statusStr == "" {
		return nil, goerr.New("status is required")
	}
	status := types.ActionStatus(statusStr)

	if _, err := ensureActionBelongsToCase(ctx, t.deps, caseID, actionID); err != nil {
		return nil, err
	}

	tool.Update(ctx, fmt.Sprintf("Updating action #%d status -> %s", actionID, statusStr))

	updated, err := t.deps.ActionUC.UpdateAction(ctx, t.deps.WorkspaceID, actionID, ActionUpdate{Status: &status}, t.deps.ActorID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update action status",
			goerr.V("workspace_id", t.deps.WorkspaceID),
			goerr.V("case_id", caseID),
			goerr.V("action_id", actionID),
			goerr.V("status", statusStr))
	}

	return actionToDetailMap(updated), nil
}

type addActionStepTool struct {
	deps Deps
}

func (t *addActionStepTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "case__add_action_step",
		Description: "Add a new step (binary-state work item) under an action.",
		Parameters: map[string]*gollem.Parameter{
			"case_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the case the action belongs to.",
				Required:    true,
			},
			"action_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the parent action.",
				Required:    true,
			},
			"title": {
				Type:        gollem.TypeString,
				Description: "Short title describing the step.",
				Required:    true,
			},
		},
	}
}

func (t *addActionStepTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	caseID, err := tool.ExtractInt64(args, "case_id")
	if err != nil {
		return nil, err
	}
	actionID, err := tool.ExtractInt64(args, "action_id")
	if err != nil {
		return nil, err
	}
	title, _ := args["title"].(string)
	if title == "" {
		return nil, goerr.New("title is required")
	}

	if _, err := ensureActionBelongsToCase(ctx, t.deps, caseID, actionID); err != nil {
		return nil, err
	}

	tool.Update(ctx, fmt.Sprintf("Adding step to action #%d: %s", actionID, title))

	step, err := t.deps.ActionUC.AddActionStep(ctx, t.deps.WorkspaceID, actionID, title, t.deps.ActorID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to add action step",
			goerr.V("workspace_id", t.deps.WorkspaceID),
			goerr.V("case_id", caseID),
			goerr.V("action_id", actionID))
	}

	return actionStepToMap(step), nil
}

type setActionStepDoneTool struct {
	deps Deps
}

func (t *setActionStepDoneTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "case__set_action_step_done",
		Description: "Mark an action step as done or revert it to ongoing.",
		Parameters: map[string]*gollem.Parameter{
			"case_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the case the action belongs to.",
				Required:    true,
			},
			"action_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the parent action.",
				Required:    true,
			},
			"step_id": {
				Type:        gollem.TypeString,
				Description: "The ID of the step (UUID).",
				Required:    true,
			},
			"done": {
				Type:        gollem.TypeBoolean,
				Description: "true to mark as done, false to revert to ongoing.",
				Required:    true,
			},
		},
	}
}

func (t *setActionStepDoneTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	caseID, err := tool.ExtractInt64(args, "case_id")
	if err != nil {
		return nil, err
	}
	actionID, err := tool.ExtractInt64(args, "action_id")
	if err != nil {
		return nil, err
	}
	stepID, _ := args["step_id"].(string)
	if stepID == "" {
		return nil, goerr.New("step_id is required")
	}
	doneVal, ok := args["done"].(bool)
	if !ok {
		return nil, goerr.New("done is required (boolean)")
	}

	if _, err := ensureActionBelongsToCase(ctx, t.deps, caseID, actionID); err != nil {
		return nil, err
	}

	tool.Update(ctx, fmt.Sprintf("Updating step %s on action #%d (done=%v)", stepID, actionID, doneVal))

	step, err := t.deps.ActionUC.SetActionStepDone(ctx, t.deps.WorkspaceID, actionID, stepID, doneVal, t.deps.ActorID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to set action step done state",
			goerr.V("workspace_id", t.deps.WorkspaceID),
			goerr.V("case_id", caseID),
			goerr.V("action_id", actionID),
			goerr.V("step_id", stepID))
	}

	return actionStepToMap(step), nil
}
