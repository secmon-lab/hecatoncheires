package graphql

import (
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

type Resolver struct {
	repo interfaces.Repository
	uc   *usecase.UseCases
}

func NewResolver(repo interfaces.Repository, uc *usecase.UseCases) *Resolver {
	return &Resolver{
		repo: repo,
		uc:   uc,
	}
}
