package graphql

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func resolveStepActor(ctx context.Context) usecase.ActorRef {
	if token, tokenErr := auth.TokenFromContext(ctx); tokenErr == nil {
		return usecase.ActorRef{Kind: usecase.ActorKindSlackUser, ID: token.Sub}
	}
	return usecase.ActorRef{Kind: usecase.ActorKindSystem}
}
