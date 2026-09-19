package graphql_test

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	gqlctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// denyingAuthorizer denies exactly one workspace and records which workspaces
// it was asked about.
type denyingAuthorizer struct {
	denied string
	asked  []string
}

func (d *denyingAuthorizer) Authorize(_ context.Context, workspaceID, _ string) error {
	return d.decide(workspaceID)
}

func (d *denyingAuthorizer) AuthorizeCurrentUser(_ context.Context, workspaceID string) error {
	return d.decide(workspaceID)
}

func (d *denyingAuthorizer) decide(workspaceID string) error {
	d.asked = append(d.asked, workspaceID)
	if workspaceID == d.denied {
		return goerr.Wrap(model.ErrWorkspaceAccessDenied, "denied")
	}
	return nil
}

func (d *denyingAuthorizer) FilterAccessible(_ context.Context, entries []*model.WorkspaceEntry, _ string) ([]*model.WorkspaceEntry, error) {
	return entries, nil
}

func (d *denyingAuthorizer) FilterAccessibleForCurrentUser(_ context.Context, entries []*model.WorkspaceEntry) ([]*model.WorkspaceEntry, error) {
	return entries, nil
}

func runField(t *testing.T, access *denyingAuthorizer, fc *graphql.FieldContext) (called bool, err error) {
	t.Helper()
	ctx := graphql.WithFieldContext(context.Background(), fc)
	_, err = gqlctrl.WorkspaceAccessMiddleware(access)(ctx, func(context.Context) (any, error) {
		called = true
		return "ok", nil
	})
	return called, err
}

func TestWorkspaceAccessMiddleware_DeniedRootFieldSkipsResolver(t *testing.T) {
	for _, object := range []string{"Query", "Mutation"} {
		access := &denyingAuthorizer{denied: "b"}
		called, err := runField(t, access, &graphql.FieldContext{
			Object: object,
			Args:   map[string]any{"workspaceId": "b"},
		})
		gt.Error(t, err).Is(model.ErrWorkspaceAccessDenied)
		gt.Value(t, called).Equal(false)
	}
}

func TestWorkspaceAccessMiddleware_AllowedRootFieldRuns(t *testing.T) {
	access := &denyingAuthorizer{denied: "b"}
	called, err := runField(t, access, &graphql.FieldContext{
		Object: "Query",
		Args:   map[string]any{"workspaceId": "a"},
	})
	gt.NoError(t, err)
	gt.Value(t, called).Equal(true)
	gt.Value(t, access.asked).Equal([]string{"a"})
}

func TestWorkspaceAccessMiddleware_NonRootFieldIsNotChecked(t *testing.T) {
	access := &denyingAuthorizer{denied: "b"}
	called, err := runField(t, access, &graphql.FieldContext{
		Object: "Case",
		Args:   map[string]any{"workspaceId": "b"},
	})
	gt.NoError(t, err)
	gt.Value(t, called).Equal(true)
	gt.Value(t, len(access.asked)).Equal(0)
}

func TestWorkspaceAccessMiddleware_RootFieldWithoutWorkspaceArg(t *testing.T) {
	access := &denyingAuthorizer{denied: "b"}
	called, err := runField(t, access, &graphql.FieldContext{
		Object: "Query",
		Args:   map[string]any{"id": "b"},
	})
	gt.NoError(t, err)
	gt.Value(t, called).Equal(true)
	gt.Value(t, len(access.asked)).Equal(0)
}
