package usecase_test

import (
	"context"
	"testing"

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
		if err != nil {
			t.Fatalf("ValidateToken failed: %v", err)
		}

		if token.Sub != sub {
			t.Errorf("Sub mismatch: got %v, want %v", token.Sub, sub)
		}
		if token.Email != email {
			t.Errorf("Email mismatch: got %v, want %v", token.Email, email)
		}
		if token.Name != name {
			t.Errorf("Name mismatch: got %v, want %v", token.Name, name)
		}
	})

	t.Run("HandleCallback returns specified user token", func(t *testing.T) {
		ctx := context.Background()
		token, err := uc.HandleCallback(ctx, "dummy-code")
		if err != nil {
			t.Fatalf("HandleCallback failed: %v", err)
		}

		if token.Sub != sub {
			t.Errorf("Sub mismatch: got %v, want %v", token.Sub, sub)
		}
		if token.Email != email {
			t.Errorf("Email mismatch: got %v, want %v", token.Email, email)
		}
		if token.Name != name {
			t.Errorf("Name mismatch: got %v, want %v", token.Name, name)
		}
	})

	t.Run("IsNoAuthn returns true", func(t *testing.T) {
		if !uc.IsNoAuthn() {
			t.Error("IsNoAuthn should return true")
		}
	})

	t.Run("GetAuthURL returns root path", func(t *testing.T) {
		url := uc.GetAuthURL("state")
		if url != "/" {
			t.Errorf("GetAuthURL should return /, got %v", url)
		}
	})

	t.Run("Logout does nothing", func(t *testing.T) {
		ctx := context.Background()
		err := uc.Logout(ctx, "token-id")
		if err != nil {
			t.Fatalf("Logout should not return error: %v", err)
		}
	})
}

func TestNoAuthnUseCaseImplementsInterface(t *testing.T) {
	repo := memory.New()
	uc := usecase.NewNoAuthnUseCase(repo, "sub", "email", "name")

	// This test verifies that NoAuthnUseCase implements AuthUseCaseInterface
	// If it doesn't compile, the interface is not satisfied
	var _ usecase.AuthUseCaseInterface = uc
}
