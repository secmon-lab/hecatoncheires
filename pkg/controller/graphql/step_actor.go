package graphql

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// resolveStepActor extracts an ActorRef from the request context. When a Slack
// user token is present, it identifies the actor as that user; otherwise the
// caller is treated as a system actor (e.g., AI / background job paths that
// have no user token attached).
func resolveStepActor(ctx context.Context) usecase.ActorRef {
	if token, tokenErr := auth.TokenFromContext(ctx); tokenErr == nil {
		return usecase.ActorRef{Kind: usecase.ActorKindSlackUser, ID: token.Sub}
	}
	return usecase.ActorRef{Kind: usecase.ActorKindSystem}
}
