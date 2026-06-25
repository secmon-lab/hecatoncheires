package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	httpctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/http"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/authz"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// fakePolicy is a hand-written PolicyClient that returns a fixed Result and
// records the last input it was queried with, so the MCP tests can drive
// allow / deny / user-injection deterministically without a Rego file.
type fakePolicy struct {
	result    authz.Result
	err       error
	lastInput authz.Input
}

func (f *fakePolicy) Query(_ context.Context, _ string, input, out any) error {
	if in, ok := input.(authz.Input); ok {
		f.lastInput = in
	}
	if f.err != nil {
		return f.err
	}
	if r, ok := out.(*authz.Result); ok {
		*r = f.result
	}
	return nil
}

// mcpTestEnv bundles the fixtures shared across the MCP tool tests.
type mcpTestEnv struct {
	server      *httptest.Server
	policy      *fakePolicy
	publicCase  *model.Case
	privateCase *model.Case
	publicActID int64
	privActID   int64
}

func newMCPTestEnv(t *testing.T, policy *fakePolicy) *mcpTestEnv {
	t.Helper()
	repo := memory.New()
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace:       model.Workspace{ID: testWorkspaceID, Name: "Test Workspace", Description: "for MCP tests"},
		ActionStatusSet: model.DefaultActionStatusSet(),
	})

	caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
	actionUC := usecase.NewActionUseCase(repo, registry, nil, "", nil)

	// "UMEMBER" is a member of the private case below; we still expect the
	// private case + its action to be invisible over MCP.
	memberCtx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UMEMBER"})

	pubCase, err := caseUC.CreateCase(memberCtx, testWorkspaceID, "Public Case", "Desc", []string{}, nil, false, false, "", "")
	gt.NoError(t, err).Required()
	pubAct, err := actionUC.CreateAction(memberCtx, testWorkspaceID, pubCase.ID, "Public Action", "Desc", "", "", types.ActionStatusTodo, nil)
	gt.NoError(t, err).Required()

	privateCase := &model.Case{
		ReporterID:     "UMEMBER",
		Title:          "Private Case",
		Description:    "Secret",
		IsPrivate:      true,
		ChannelUserIDs: []string{"UMEMBER"},
		AssigneeIDs:    []string{},
	}
	privCreated, err := repo.Case().Create(memberCtx, testWorkspaceID, privateCase)
	gt.NoError(t, err).Required()
	privAct, err := actionUC.CreateAction(memberCtx, testWorkspaceID, privCreated.ID, "Private Action", "Desc", "", "", types.ActionStatusTodo, nil)
	gt.NoError(t, err).Required()

	handler := httpctrl.NewMCPHandler(caseUC, actionUC, registry, policy, map[string]string{})
	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &mcpTestEnv{
		server:      srv,
		policy:      policy,
		publicCase:  pubCase,
		privateCase: privCreated,
		publicActID: pubAct.ID,
		privActID:   privAct.ID,
	}
}

func (e *mcpTestEnv) callTool(t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: e.server.URL + "/mcp"}, nil)
	gt.NoError(t, err).Required()
	t.Cleanup(func() { _ = session.Close() })

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	gt.NoError(t, err).Required()
	return res
}

// decodeStructured re-marshals the tool's structured output and decodes it
// into v.
func decodeStructured(t *testing.T, res *mcp.CallToolResult, v any) {
	t.Helper()
	raw, err := json.Marshal(res.StructuredContent)
	gt.NoError(t, err).Required()
	gt.NoError(t, json.Unmarshal(raw, v)).Required()
}

func allowAsMember() *fakePolicy {
	return &fakePolicy{result: authz.Result{Allow: true, User: "UMEMBER"}}
}

func TestMCP_ListWorkspaces(t *testing.T) {
	env := newMCPTestEnv(t, allowAsMember())
	res := env.callTool(t, "hecaton_list_workspaces", map[string]any{})
	gt.Bool(t, res.IsError).False()

	var out struct {
		Workspaces []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"workspaces"`
	}
	decodeStructured(t, res, &out)
	gt.Array(t, out.Workspaces).Length(1)
	gt.Value(t, out.Workspaces[0].ID).Equal(testWorkspaceID)
	gt.Value(t, out.Workspaces[0].Name).Equal("Test Workspace")
}

func TestMCP_ListCases_ExcludesPrivate(t *testing.T) {
	env := newMCPTestEnv(t, allowAsMember())
	res := env.callTool(t, "hecaton_list_cases", map[string]any{"workspace_id": testWorkspaceID})
	gt.Bool(t, res.IsError).False()

	var out struct {
		Cases []struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"cases"`
	}
	decodeStructured(t, res, &out)
	// Only the public case is returned, even though the policy injected a
	// member of the private case.
	gt.Array(t, out.Cases).Length(1)
	gt.Value(t, out.Cases[0].ID).Equal(env.publicCase.ID)
	gt.Value(t, out.Cases[0].Title).Equal("Public Case")
}

func TestMCP_GetCases_OmitsPrivateAndMissing(t *testing.T) {
	env := newMCPTestEnv(t, allowAsMember())
	res := env.callTool(t, "hecaton_get_cases", map[string]any{
		"workspace_id": testWorkspaceID,
		"ids":          []int64{env.publicCase.ID, env.privateCase.ID, 999999},
	})
	gt.Bool(t, res.IsError).False()

	var out struct {
		Cases []struct {
			ID          int64  `json:"id"`
			Description string `json:"description"`
		} `json:"cases"`
	}
	decodeStructured(t, res, &out)
	gt.Array(t, out.Cases).Length(1)
	gt.Value(t, out.Cases[0].ID).Equal(env.publicCase.ID)
	gt.Value(t, out.Cases[0].Description).Equal("Desc")
}

func TestMCP_ListActions_ExcludesPrivateCaseActions(t *testing.T) {
	env := newMCPTestEnv(t, allowAsMember())
	res := env.callTool(t, "hecaton_list_actions", map[string]any{"workspace_id": testWorkspaceID})
	gt.Bool(t, res.IsError).False()

	var out struct {
		Actions []struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"actions"`
	}
	decodeStructured(t, res, &out)
	gt.Array(t, out.Actions).Length(1)
	gt.Value(t, out.Actions[0].ID).Equal(env.publicActID)
	gt.Value(t, out.Actions[0].Title).Equal("Public Action")
}

func TestMCP_ListActions_ByPrivateCaseIsEmpty(t *testing.T) {
	env := newMCPTestEnv(t, allowAsMember())
	res := env.callTool(t, "hecaton_list_actions", map[string]any{
		"workspace_id": testWorkspaceID,
		"case_id":      env.privateCase.ID,
	})
	gt.Bool(t, res.IsError).False()

	var out struct {
		Actions []struct {
			ID int64 `json:"id"`
		} `json:"actions"`
	}
	decodeStructured(t, res, &out)
	gt.Array(t, out.Actions).Length(0)
}

func TestMCP_GetActions_OmitsPrivateCaseActions(t *testing.T) {
	env := newMCPTestEnv(t, allowAsMember())
	res := env.callTool(t, "hecaton_get_actions", map[string]any{
		"workspace_id": testWorkspaceID,
		"ids":          []int64{env.publicActID, env.privActID},
	})
	gt.Bool(t, res.IsError).False()

	var out struct {
		Actions []struct {
			ID int64 `json:"id"`
		} `json:"actions"`
	}
	decodeStructured(t, res, &out)
	gt.Array(t, out.Actions).Length(1)
	gt.Value(t, out.Actions[0].ID).Equal(env.publicActID)
}

func TestMCP_DenyReturnsToolError(t *testing.T) {
	env := newMCPTestEnv(t, &fakePolicy{result: authz.Result{Allow: false}})
	res := env.callTool(t, "hecaton_list_cases", map[string]any{"workspace_id": testWorkspaceID})
	// A policy denial surfaces as a tool error, not as decodable data.
	gt.Bool(t, res.IsError).True()
}

func TestMCP_PolicyReceivesToolNameAndWorkspace(t *testing.T) {
	policy := allowAsMember()
	env := newMCPTestEnv(t, policy)
	env.callTool(t, "hecaton_list_cases", map[string]any{"workspace_id": testWorkspaceID})

	gt.Value(t, policy.lastInput.Tool).NotNil()
	gt.Value(t, policy.lastInput.Tool.Name).Equal("hecaton_list_cases")
	gt.Value(t, policy.lastInput.Tool.WorkspaceID).Equal(testWorkspaceID)
	gt.Value(t, policy.lastInput.Req).NotNil()
	gt.Value(t, policy.lastInput.Req.Path).Equal("/mcp")
}

func TestMCP_PolicyErrorSurfacesAsToolError(t *testing.T) {
	env := newMCPTestEnv(t, &fakePolicy{err: goerr.New("boom")})
	res := env.callTool(t, "hecaton_list_workspaces", map[string]any{})
	gt.Bool(t, res.IsError).True()
}
