package usecase_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestNoAuthnUseCase(t *testing.T) {
	repo := memory.New()
	sub := "U1234567890"
	email := "test@example.com"
	name := "Test User"

	uc := usecase.NewNoAuthnUseCase(repo, sub, email, name)

	t.Run("ValidateToken returns specified user token", func(t *testing.T) {
		ctx := context.Background()
		token, err := uc.ValidateToken(ctx, "", "")
		gt.NoError(t, err).Required()

		gt.Value(t, token.Sub).Equal(sub)
		gt.Value(t, token.Email).Equal(email)
		gt.Value(t, token.Name).Equal(name)
	})

	t.Run("HandleCallback returns specified user token", func(t *testing.T) {
		ctx := context.Background()
		token, err := uc.HandleCallback(ctx, "dummy-code")
		gt.NoError(t, err).Required()

		gt.Value(t, token.Sub).Equal(sub)
		gt.Value(t, token.Email).Equal(email)
		gt.Value(t, token.Name).Equal(name)
	})

	t.Run("IsNoAuthn returns true", func(t *testing.T) {
		gt.Bool(t, uc.IsNoAuthn()).True()
	})

	t.Run("GetAuthURL returns root path", func(t *testing.T) {
		url := uc.GetAuthURL("state")
		gt.Value(t, url).Equal("/")
	})

	t.Run("Logout does nothing", func(t *testing.T) {
		ctx := context.Background()
		err := uc.Logout(ctx, "token-id")
		gt.NoError(t, err).Required()
	})
}

func TestNoAuthnUseCaseImplementsInterface(t *testing.T) {
	repo := memory.New()
	uc := usecase.NewNoAuthnUseCase(repo, "sub", "email", "name")

	// This test verifies that NoAuthnUseCase implements AuthUseCaseInterface
	// If it doesn't compile, the interface is not satisfied
	var _ usecase.AuthUseCaseInterface = uc
}
