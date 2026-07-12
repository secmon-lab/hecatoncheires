package graphql_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	graphqlctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// TestCaseResolver_Reporter pins the reporter resolution contract: the
// frontend prioritises display, so a reporter whose SlackUser is missing from
// the repository (an unsynced thread-mode poster, a deleted account, ...)
// resolves to null WITHOUT a field-level GraphQL error that would fail the
// whole `cases` query (Sentry ARGUS-7S). Ops visibility is preserved by the
// SlackUser dataloader, which reports missing IDs via errutil.Handle.
func TestCaseResolver_Reporter(t *testing.T) {
	repo := memory.New()
	reporterID := "U02K73K8NB1"

	t.Run("missing reporter resolves to nil without error (thread-mode)", func(t *testing.T) {
		resolver := graphqlctrl.NewResolver(repo, nil)
		ctx := graphqlctrl.WithDataLoaders(context.Background(), graphqlctrl.NewDataLoaders(repo, nil))
		user, err := resolver.Case().Reporter(ctx, &graphql1.Case{ID: 1, ReporterID: &reporterID, IsThreadBound: true})
		gt.NoError(t, err)
		gt.Value(t, user).Nil()
	})

	t.Run("missing reporter resolves to nil without error (channel-mode)", func(t *testing.T) {
		resolver := graphqlctrl.NewResolver(repo, nil)
		ctx := graphqlctrl.WithDataLoaders(context.Background(), graphqlctrl.NewDataLoaders(repo, nil))
		user, err := resolver.Case().Reporter(ctx, &graphql1.Case{ID: 2, ReporterID: &reporterID, IsThreadBound: false})
		gt.NoError(t, err)
		gt.Value(t, user).Nil()
	})

	t.Run("known reporter resolves to the SlackUser", func(t *testing.T) {
		seeded := memory.New()
		gt.NoError(t, seeded.SlackUser().SaveMany(context.Background(), []*model.SlackUser{
			{ID: model.SlackUserID(reporterID), Name: "mizu", RealName: "Mizu San"},
		})).Required()
		resolver := graphqlctrl.NewResolver(seeded, nil)
		ctx := graphqlctrl.WithDataLoaders(context.Background(), graphqlctrl.NewDataLoaders(seeded, nil))
		user, err := resolver.Case().Reporter(ctx, &graphql1.Case{ID: 3, ReporterID: &reporterID, IsThreadBound: true})
		gt.NoError(t, err).Required()
		gt.Value(t, user).NotNil().Required()
		gt.Value(t, user.ID).Equal(reporterID)
		gt.Value(t, user.RealName).Equal("Mizu San")
	})

	t.Run("empty reporter id resolves to nil without error", func(t *testing.T) {
		resolver := graphqlctrl.NewResolver(repo, nil)
		ctx := graphqlctrl.WithDataLoaders(context.Background(), graphqlctrl.NewDataLoaders(repo, nil))
		user, err := resolver.Case().Reporter(ctx, &graphql1.Case{ID: 4, ReporterID: nil})
		gt.NoError(t, err)
		gt.Value(t, user).Nil()
	})
}

// buildGroupResolver wires a resolver whose UseCases carry the given workspace
// and group registries, so WorkspaceGroups() resolves real member workspaces.
func buildGroupResolver(t *testing.T, wsReg *model.WorkspaceRegistry, groupReg *model.WorkspaceGroupRegistry) *graphqlctrl.Resolver {
	t.Helper()
	repo := memory.New()
	uc := usecase.New(repo, wsReg, usecase.WithWorkspaceGroups(groupReg))
	return graphqlctrl.NewResolver(repo, uc)
}

func TestQueryResolver_WorkspaceGroups(t *testing.T) {
	ctx := context.Background()

	wsReg := model.NewWorkspaceRegistry()
	wsReg.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: "risk", Name: "Risk Management"}})
	wsReg.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: "incident", Name: "Incident"}})
	wsReg.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: "legal", Name: "Legal"}})

	t.Run("resolves groups with their member workspaces", func(t *testing.T) {
		groupReg := model.NewWorkspaceGroupRegistry()
		groupReg.Register(&model.WorkspaceGroup{
			ID: "security", Name: "Security", Description: "Security workspaces",
			MemberIDs: []string{"risk", "incident"},
		})
		groupReg.Register(&model.WorkspaceGroup{
			ID: "audit", Name: "Audit", MemberIDs: []string{"risk", "legal"},
		})

		resolver := buildGroupResolver(t, wsReg, groupReg)
		groups, err := resolver.Query().WorkspaceGroups(ctx)
		gt.NoError(t, err).Required()
		gt.Array(t, groups).Length(2).Required()

		// Registration order preserved.
		gt.Value(t, groups[0].ID).Equal("security")
		gt.Value(t, groups[0].Name).Equal("Security")
		gt.Value(t, groups[0].Description).NotNil().Required()
		gt.Value(t, *groups[0].Description).Equal("Security workspaces")
		gt.Array(t, groups[0].Workspaces).Length(2).Required()
		gt.Value(t, groups[0].Workspaces[0].ID).Equal("risk")
		gt.Value(t, groups[0].Workspaces[0].Name).Equal("Risk Management")
		gt.Value(t, groups[0].Workspaces[1].ID).Equal("incident")

		// A workspace can appear in multiple groups.
		gt.Value(t, groups[1].ID).Equal("audit")
		gt.Value(t, groups[1].Description).Nil() // empty description -> null
		gt.Array(t, groups[1].Workspaces).Length(2).Required()
		gt.Value(t, groups[1].Workspaces[0].ID).Equal("risk")
		gt.Value(t, groups[1].Workspaces[1].ID).Equal("legal")
	})

	t.Run("no groups configured returns an empty list", func(t *testing.T) {
		resolver := buildGroupResolver(t, wsReg, model.NewWorkspaceGroupRegistry())
		groups, err := resolver.Query().WorkspaceGroups(ctx)
		gt.NoError(t, err).Required()
		gt.Array(t, groups).Length(0)
	})

	t.Run("unresolvable member is skipped, keeping the list non-null", func(t *testing.T) {
		groupReg := model.NewWorkspaceGroupRegistry()
		// "ghost" is not in the workspace registry; the resolver skips it.
		groupReg.Register(&model.WorkspaceGroup{ID: "mixed", Name: "Mixed", MemberIDs: []string{"risk", "ghost"}})

		resolver := buildGroupResolver(t, wsReg, groupReg)
		groups, err := resolver.Query().WorkspaceGroups(ctx)
		gt.NoError(t, err).Required()
		gt.Array(t, groups).Length(1).Required()
		gt.Array(t, groups[0].Workspaces).Length(1).Required()
		gt.Value(t, groups[0].Workspaces[0].ID).Equal("risk")
	})
}
