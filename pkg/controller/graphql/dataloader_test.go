package graphql_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	graphqlctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

// TestSlackUserLoader_NormalizesCompositeSubClaim pins the rescue path
// for legacy reporter IDs that were persisted as the composite
// "Uxxx-Txxx" OIDC sub form before the auth-side fix. The bare user
// ID is what the SlackUser repository keys on, so the loader must
// strip the team suffix before lookup; otherwise the reporter resolver
// returns nil and the UI shows an empty cell even though identity is
// known.
func TestSlackUserLoader_NormalizesCompositeSubClaim(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()

	gt.NoError(t, repo.SlackUser().SaveMany(ctx, []*model.SlackUser{
		{ID: "U01ABC", Name: "alice", RealName: "Alice"},
		{ID: "W04ENT", Name: "bob", RealName: "Bob"},
	})).Required()

	loader := graphqlctrl.NewSlackUserLoader(repo)

	users, err := loader.Load(ctx, []string{
		"U01ABC-T02XYZ", // composite, user-first
		"T02XYZ-W04ENT", // composite, team-first, W prefix
	})
	gt.NoError(t, err).Required()
	gt.Array(t, users).Length(2).Required()
	gt.Value(t, users[0].ID).Equal("U01ABC")
	gt.Value(t, users[0].RealName).Equal("Alice")
	gt.Value(t, users[1].ID).Equal("W04ENT")
	gt.Value(t, users[1].RealName).Equal("Bob")
}

// TestSlackUserLoader_PreservesBareIDs guards that the normalisation
// path does not regress the common case where the caller passes the
// already-bare user ID (assignee IDs picked through the Slack picker,
// modern reporter IDs persisted after the auth fix).
func TestSlackUserLoader_PreservesBareIDs(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()

	gt.NoError(t, repo.SlackUser().SaveMany(ctx, []*model.SlackUser{
		{ID: "U01ABC", Name: "alice", RealName: "Alice"},
	})).Required()

	loader := graphqlctrl.NewSlackUserLoader(repo)

	users, err := loader.Load(ctx, []string{"U01ABC"})
	gt.NoError(t, err).Required()
	gt.Array(t, users).Length(1).Required()
	gt.Value(t, users[0].ID).Equal("U01ABC")
}

// TestSlackUserLoader_SkipsUnknownIDs guards that IDs missing from the
// SlackUser repository are silently dropped (the schema declares the
// list elements as non-null, so the loader MUST NOT emit nils).
func TestSlackUserLoader_SkipsUnknownIDs(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()

	gt.NoError(t, repo.SlackUser().SaveMany(ctx, []*model.SlackUser{
		{ID: "U01ABC", Name: "alice", RealName: "Alice"},
	})).Required()

	loader := graphqlctrl.NewSlackUserLoader(repo)

	users, err := loader.Load(ctx, []string{"U01ABC", "UNOPE"})
	gt.NoError(t, err).Required()
	gt.Array(t, users).Length(1).Required()
	gt.Value(t, users[0].ID).Equal("U01ABC")
}
