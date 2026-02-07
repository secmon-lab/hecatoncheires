package repository_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
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
		gt.NoError(t, repo.PutToken(ctx, token)).Required()

		// Get token
		retrieved, err := repo.GetToken(ctx, token.ID)
		gt.NoError(t, err).Required()

		// Verify all fields
		gt.Value(t, retrieved.ID).Equal(token.ID)
		gt.Value(t, retrieved.Secret).Equal(token.Secret)
		gt.Value(t, retrieved.Sub).Equal(token.Sub)
		gt.Value(t, retrieved.Email).Equal(token.Email)
		gt.Value(t, retrieved.Name).Equal(token.Name)

		// Compare timestamps with tolerance for Firestore precision
		gt.Bool(t, retrieved.ExpiresAt.Sub(token.ExpiresAt).Abs() < time.Second).True()
		gt.Bool(t, retrieved.CreatedAt.Sub(token.CreatedAt).Abs() < time.Second).True()
	})

	t.Run("GetToken not found", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		nonExistentID := auth.NewTokenID()
		_, err := repo.GetToken(ctx, nonExistentID)
		gt.Value(t, err).NotNil().Required()

		// Check if it's a NotFound error
		gt.Bool(t, errors.Is(err, firestore.ErrNotFound) || errors.Is(err, memory.ErrNotFound)).True()
	})

	t.Run("DeleteToken", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		token := auth.NewToken("user-456", "delete@example.com", "Delete User")

		// Put token
		gt.NoError(t, repo.PutToken(ctx, token)).Required()

		// Delete token
		gt.NoError(t, repo.DeleteToken(ctx, token.ID)).Required()

		// Verify it's deleted
		_, err := repo.GetToken(ctx, token.ID)
		gt.Value(t, err).NotNil().Required()
		gt.Bool(t, errors.Is(err, firestore.ErrNotFound) || errors.Is(err, memory.ErrNotFound)).True()
	})

	t.Run("DeleteToken not found", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		nonExistentID := auth.NewTokenID()
		err := repo.DeleteToken(ctx, nonExistentID)
		gt.Value(t, err).NotNil().Required()
		gt.Bool(t, errors.Is(err, firestore.ErrNotFound) || errors.Is(err, memory.ErrNotFound)).True()
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
		gt.Value(t, err).NotNil().Required()
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
		gt.NoError(t, err).Required()

		t.Cleanup(func() {
			gt.NoError(t, repo.Close())
		})

		return repo
	})
}
