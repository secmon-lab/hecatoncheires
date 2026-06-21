package authz_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/authz"
)

func TestBuildInput_WithRequestEnvAndTool(t *testing.T) {
	req := &authz.HTTPRequest{
		Method: "POST",
		Path:   "/mcp",
		Header: map[string][]string{"Authorization": {"Bearer secret-token"}},
	}
	ctx := authz.ContextWithRequest(context.Background(), req)

	env := map[string]string{"MCP_TOKEN": "secret-token"}
	tool := &authz.ToolCall{
		Name:        "hecaton_list_cases",
		WorkspaceID: "ws1",
		Args:        map[string]any{"workspace_id": "ws1", "status": "OPEN"},
	}

	in := authz.BuildInput(ctx, env, tool)

	gt.Value(t, in.Req).Equal(req)
	gt.Value(t, in.Env["MCP_TOKEN"]).Equal("secret-token")
	gt.Value(t, in.Tool.Name).Equal("hecaton_list_cases")
	gt.Value(t, in.Tool.WorkspaceID).Equal("ws1")
	gt.Value(t, in.Tool.Args["status"]).Equal("OPEN")
}

func TestBuildInput_NilEnvBecomesEmptyMap(t *testing.T) {
	in := authz.BuildInput(context.Background(), nil, nil)

	gt.Value(t, in.Env).NotNil()
	gt.Number(t, len(in.Env)).Equal(0)
	// Without a request stored in context, Req is nil and Tool is nil.
	gt.Value(t, in.Req).Nil()
	gt.Value(t, in.Tool).Nil()
}

func TestRequestFromContext_Missing(t *testing.T) {
	gt.Value(t, authz.RequestFromContext(context.Background())).Nil()
}

func TestContextWithRequest_RoundTrip(t *testing.T) {
	req := &authz.HTTPRequest{Method: "GET", Path: "/mcp"}
	ctx := authz.ContextWithRequest(context.Background(), req)
	gt.Value(t, authz.RequestFromContext(ctx)).Equal(req)
}
