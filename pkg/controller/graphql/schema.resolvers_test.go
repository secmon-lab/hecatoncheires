package graphql_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	graphqlctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
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
