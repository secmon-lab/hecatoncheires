package usecase

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
)

// NoAuthnUseCase provides authentication using a specified user (for development/testing)
type NoAuthnUseCase struct {
	repo  interfaces.Repository
	sub   string // Slack User ID
	email string
	name  string
}

// NewNoAuthnUseCase creates a new NoAuthnUseCase instance with specified user info
func NewNoAuthnUseCase(repo interfaces.Repository, sub, email, name string) *NoAuthnUseCase {
	return &NoAuthnUseCase{
		repo:  repo,
		sub:   sub,
		email: email,
		name:  name,
	}
}

// GetAuthURL returns a dummy URL (should not be called in no-auth mode)
func (uc *NoAuthnUseCase) GetAuthURL(state string) string {
	return "/"
}

// HandleCallback handles OAuth callback (should not be called in no-auth mode)
func (uc *NoAuthnUseCase) HandleCallback(ctx context.Context, code string) (*auth.Token, error) {
	// In no-auth mode, return token for the specified user
	return auth.NewToken(uc.sub, uc.email, uc.name), nil
}

// ValidateToken always returns a token for the specified user
func (uc *NoAuthnUseCase) ValidateToken(ctx context.Context, tokenID auth.TokenID, tokenSecret auth.TokenSecret) (*auth.Token, error) {
	// Always return token for specified user
	return auth.NewToken(uc.sub, uc.email, uc.name), nil
}

// Logout does nothing in no-auth mode
func (uc *NoAuthnUseCase) Logout(ctx context.Context, tokenID auth.TokenID) error {
	// No-op in no-auth mode
	return nil
}

// IsNoAuthn returns true for NoAuthnUseCase
func (uc *NoAuthnUseCase) IsNoAuthn() bool {
	return true
}
