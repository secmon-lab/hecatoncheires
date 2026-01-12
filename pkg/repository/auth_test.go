package repository_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runAuthRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Run("PutToken and GetToken", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		token := auth.NewToken("user-123", "test@example.com", "Test User")

		// Put token
		if err := repo.PutToken(ctx, token); err != nil {
			t.Fatalf("PutToken failed: %v", err)
		}

		// Get token
		retrieved, err := repo.GetToken(ctx, token.ID)
		if err != nil {
			t.Fatalf("GetToken failed: %v", err)
		}

		// Verify all fields
		if retrieved.ID != token.ID {
			t.Errorf("ID mismatch: got %v, want %v", retrieved.ID, token.ID)
		}
		if retrieved.Secret != token.Secret {
			t.Errorf("Secret mismatch: got %v, want %v", retrieved.Secret, token.Secret)
		}
		if retrieved.Sub != token.Sub {
			t.Errorf("Sub mismatch: got %v, want %v", retrieved.Sub, token.Sub)
		}
		if retrieved.Email != token.Email {
			t.Errorf("Email mismatch: got %v, want %v", retrieved.Email, token.Email)
		}
		if retrieved.Name != token.Name {
			t.Errorf("Name mismatch: got %v, want %v", retrieved.Name, token.Name)
		}

		// Compare timestamps with tolerance for Firestore precision
		if diff := retrieved.ExpiresAt.Sub(token.ExpiresAt); diff > time.Second || diff < -time.Second {
			t.Errorf("ExpiresAt mismatch: got %v, want %v, diff %v", retrieved.ExpiresAt, token.ExpiresAt, diff)
		}
		if diff := retrieved.CreatedAt.Sub(token.CreatedAt); diff > time.Second || diff < -time.Second {
			t.Errorf("CreatedAt mismatch: got %v, want %v, diff %v", retrieved.CreatedAt, token.CreatedAt, diff)
		}
	})

	t.Run("GetToken not found", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		nonExistentID := auth.NewTokenID()
		_, err := repo.GetToken(ctx, nonExistentID)
		if err == nil {
			t.Fatal("Expected error for non-existent token, got nil")
		}

		// Check if it's a NotFound error
		if !errors.Is(err, firestore.ErrNotFound) && !errors.Is(err, memory.ErrNotFound) {
			t.Errorf("Expected NotFound error, got: %v", err)
		}
	})

	t.Run("DeleteToken", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		token := auth.NewToken("user-456", "delete@example.com", "Delete User")

		// Put token
		if err := repo.PutToken(ctx, token); err != nil {
			t.Fatalf("PutToken failed: %v", err)
		}

		// Delete token
		if err := repo.DeleteToken(ctx, token.ID); err != nil {
			t.Fatalf("DeleteToken failed: %v", err)
		}

		// Verify it's deleted
		_, err := repo.GetToken(ctx, token.ID)
		if err == nil {
			t.Fatal("Expected error after deletion, got nil")
		}
		if !errors.Is(err, firestore.ErrNotFound) && !errors.Is(err, memory.ErrNotFound) {
			t.Errorf("Expected NotFound error after deletion, got: %v", err)
		}
	})

	t.Run("DeleteToken not found", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		nonExistentID := auth.NewTokenID()
		err := repo.DeleteToken(ctx, nonExistentID)
		if err == nil {
			t.Fatal("Expected error for deleting non-existent token, got nil")
		}
		if !errors.Is(err, firestore.ErrNotFound) && !errors.Is(err, memory.ErrNotFound) {
			t.Errorf("Expected NotFound error, got: %v", err)
		}
	})

	t.Run("Token validation on Put", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create invalid token (empty sub)
		invalidToken := &auth.Token{
			ID:        auth.NewTokenID(),
			Secret:    auth.NewTokenSecret(),
			Sub:       "", // Invalid: empty
			Email:     "test@example.com",
			Name:      "Test",
			ExpiresAt: time.Now().Add(time.Hour),
			CreatedAt: time.Now(),
		}

		err := repo.PutToken(ctx, invalidToken)
		if err == nil {
			t.Fatal("Expected validation error for invalid token, got nil")
		}
	})
}

func TestMemoryRepository(t *testing.T) {
	runAuthRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestFirestoreRepository(t *testing.T) {
	runAuthRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		t.Helper()

		projectID := os.Getenv("TEST_FIRESTORE_PROJECT_ID")
		if projectID == "" {
			t.Skip("TEST_FIRESTORE_PROJECT_ID not set")
		}

		databaseID := os.Getenv("TEST_FIRESTORE_DATABASE_ID")
		if databaseID == "" {
			t.Skip("TEST_FIRESTORE_DATABASE_ID not set")
		}

		ctx := context.Background()
		repo, err := firestore.New(ctx, projectID, databaseID)
		if err != nil {
			t.Fatalf("failed to create firestore repository: %v", err)
		}

		t.Cleanup(func() {
			if err := repo.Close(); err != nil {
				t.Errorf("failed to close firestore repository: %v", err)
			}
		})

		return repo
	})
}
