package usecase

import (
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
)

type UseCases struct {
	repo interfaces.Repository
}

func New(repo interfaces.Repository) *UseCases {
	return &UseCases{
		repo: repo,
	}
}
