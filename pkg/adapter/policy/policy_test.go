package policy_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/adapter/policy"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/authz"
)

func newClient(t *testing.T) interface {
	Query(ctx context.Context, query string, input, out any) error
} {
	t.Helper()
	c, err := policy.New([]string{"testdata/sample.rego"})
	gt.NoError(t, err)
	return c
}

func TestNew_RequiresPath(t *testing.T) {
	_, err := policy.New(nil)
	gt.Error(t, err)
}

func TestNew_InvalidPolicyFailsToCompile(t *testing.T) {
	_, err := policy.New([]string{"testdata/does-not-exist-dir-xyz"})
	// A non-existent path makes opaq's directory walk fail at compile time.
	gt.Error(t, err)
}

func TestQuery_AllowWithMatchingToken(t *testing.T) {
	c := newClient(t)
	req := &authz.HTTPRequest{
		Method: "POST",
		Path:   "/mcp",
		Header: map[string][]string{"Authorization": {"Bearer s3cr3t"}},
	}
	ctx := authz.ContextWithRequest(context.Background(), req)
	in := authz.BuildInput(ctx, map[string]string{"MCP_TOKEN": "s3cr3t"}, &authz.ToolCall{Name: "hecaton_list_workspaces"})

	var result authz.Result
	gt.NoError(t, c.Query(ctx, "data.auth.mcp", in, &result))
	gt.Value(t, result.Allow).Equal(true)
	gt.Value(t, result.User).Equal("U0TESTUSER")
}

func TestQuery_DenyWithWrongToken(t *testing.T) {
	c := newClient(t)
	req := &authz.HTTPRequest{
		Method: "POST",
		Path:   "/mcp",
		Header: map[string][]string{"Authorization": {"Bearer wrong"}},
	}
	ctx := authz.ContextWithRequest(context.Background(), req)
	in := authz.BuildInput(ctx, map[string]string{"MCP_TOKEN": "s3cr3t"}, &authz.ToolCall{Name: "hecaton_list_workspaces"})

	var result authz.Result
	gt.NoError(t, c.Query(ctx, "data.auth.mcp", in, &result))
	gt.Value(t, result.Allow).Equal(false)
	gt.Value(t, result.User).Equal("")
}

func TestQuery_DenyWithoutAuthorizationHeader(t *testing.T) {
	c := newClient(t)
	req := &authz.HTTPRequest{Method: "POST", Path: "/mcp", Header: map[string][]string{}}
	ctx := authz.ContextWithRequest(context.Background(), req)
	in := authz.BuildInput(ctx, map[string]string{"MCP_TOKEN": "s3cr3t"}, &authz.ToolCall{Name: "hecaton_list_cases"})

	var result authz.Result
	gt.NoError(t, c.Query(ctx, "data.auth.mcp", in, &result))
	gt.Value(t, result.Allow).Equal(false)
}
