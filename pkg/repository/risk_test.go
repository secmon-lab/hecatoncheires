package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	t.Run("Create creates risk with auto-increment ID", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		risk1 := &model.Risk{
			Name:        "SQL Injection Risk",
			Description: "Database vulnerable to SQL injection",
		}

		created1, err := repo.Risk().Create(ctx, risk1)
		if err != nil {
			t.Fatalf("failed to create risk1: %v", err)
		}

		if created1.ID != 1 {
			t.Errorf("expected ID=1, got %d", created1.ID)
		}
		if created1.Name != risk1.Name {
			t.Errorf("expected name=%s, got %s", risk1.Name, created1.Name)
		}
		if created1.Description != risk1.Description {
			t.Errorf("expected description=%s, got %s", risk1.Description, created1.Description)
		}
		if created1.CreatedAt.IsZero() {
			t.Error("expected non-zero CreatedAt")
		}
		if created1.UpdatedAt.IsZero() {
			t.Error("expected non-zero UpdatedAt")
		}

		// Create second risk to test auto-increment
		risk2 := &model.Risk{
			Name:        "XSS Risk",
			Description: "Cross-site scripting vulnerability",
		}

		created2, err := repo.Risk().Create(ctx, risk2)
		if err != nil {
			t.Fatalf("failed to create risk2: %v", err)
		}

		if created2.ID != 2 {
			t.Errorf("expected ID=2, got %d", created2.ID)
		}
	})

	t.Run("Get retrieves existing risk", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Risk().Create(ctx, &model.Risk{
			Name:        "CSRF Risk",
			Description: "Cross-site request forgery",
		})
		if err != nil {
			t.Fatalf("failed to create risk: %v", err)
		}

		retrieved, err := repo.Risk().Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to get risk: %v", err)
		}

		if retrieved.ID != created.ID {
			t.Errorf("expected ID=%d, got %d", created.ID, retrieved.ID)
		}
		if retrieved.Name != created.Name {
			t.Errorf("expected name=%s, got %s", created.Name, retrieved.Name)
		}
		if retrieved.Description != created.Description {
			t.Errorf("expected description=%s, got %s", created.Description, retrieved.Description)
		}
		if !retrieved.CreatedAt.Equal(created.CreatedAt) {
			t.Errorf("expected createdAt=%v, got %v", created.CreatedAt, retrieved.CreatedAt)
		}
		if !retrieved.UpdatedAt.Equal(created.UpdatedAt) {
			t.Errorf("expected updatedAt=%v, got %v", created.UpdatedAt, retrieved.UpdatedAt)
		}
	})

	t.Run("Get returns error for non-existent risk", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		_, err := repo.Risk().Get(ctx, 99999)
		if err == nil {
			t.Error("expected error for non-existent risk")
		}
		if !errors.Is(err, memory.ErrNotFound) && !errors.Is(err, firestore.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("List returns all risks", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// List should be empty initially
		risks, err := repo.Risk().List(ctx)
		if err != nil {
			t.Fatalf("failed to list risks: %v", err)
		}
		if len(risks) != 0 {
			t.Errorf("expected 0 risks, got %d", len(risks))
		}

		// Create multiple risks
		risk1, err := repo.Risk().Create(ctx, &model.Risk{
			Name:        "Risk 1",
			Description: "Description 1",
		})
		if err != nil {
			t.Fatalf("failed to create risk1: %v", err)
		}

		risk2, err := repo.Risk().Create(ctx, &model.Risk{
			Name:        "Risk 2",
			Description: "Description 2",
		})
		if err != nil {
			t.Fatalf("failed to create risk2: %v", err)
		}

		// List should return both risks
		risks, err = repo.Risk().List(ctx)
		if err != nil {
			t.Fatalf("failed to list risks: %v", err)
		}
		if len(risks) != 2 {
			t.Errorf("expected 2 risks, got %d", len(risks))
		}

		// Verify risk data
		foundRisk1 := false
		foundRisk2 := false
		for _, r := range risks {
			if r.ID == risk1.ID && r.Name == risk1.Name && r.Description == risk1.Description {
				foundRisk1 = true
			}
			if r.ID == risk2.ID && r.Name == risk2.Name && r.Description == risk2.Description {
				foundRisk2 = true
			}
		}
		if !foundRisk1 {
			t.Error("risk1 not found in list")
		}
		if !foundRisk2 {
			t.Error("risk2 not found in list")
		}
	})

	t.Run("Update modifies existing risk", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Risk().Create(ctx, &model.Risk{
			Name:        "Original Name",
			Description: "Original Description",
		})
		if err != nil {
			t.Fatalf("failed to create risk: %v", err)
		}

		// Wait a bit to ensure UpdatedAt will be different
		time.Sleep(10 * time.Millisecond)

		updated, err := repo.Risk().Update(ctx, &model.Risk{
			ID:          created.ID,
			Name:        "Updated Name",
			Description: "Updated Description",
		})
		if err != nil {
			t.Fatalf("failed to update risk: %v", err)
		}

		if updated.ID != created.ID {
			t.Errorf("ID should not change, got %d", updated.ID)
		}
		if updated.Name != "Updated Name" {
			t.Errorf("expected name='Updated Name', got %s", updated.Name)
		}
		if updated.Description != "Updated Description" {
			t.Errorf("expected description='Updated Description', got %s", updated.Description)
		}
		if !updated.CreatedAt.Equal(created.CreatedAt) {
			t.Errorf("CreatedAt should not change, got %v", updated.CreatedAt)
		}
		if !updated.UpdatedAt.After(created.UpdatedAt) {
			t.Errorf("UpdatedAt should be after original, got %v", updated.UpdatedAt)
		}

		// Verify via Get
		retrieved, err := repo.Risk().Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to get updated risk: %v", err)
		}
		if retrieved.Name != "Updated Name" {
			t.Errorf("expected name='Updated Name' after retrieval, got %s", retrieved.Name)
		}
	})

	t.Run("Update returns error for non-existent risk", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		_, err := repo.Risk().Update(ctx, &model.Risk{
			ID:          99999,
			Name:        "Non-existent",
			Description: "Should fail",
		})
		if err == nil {
			t.Error("expected error for non-existent risk")
		}
		if !errors.Is(err, memory.ErrNotFound) && !errors.Is(err, firestore.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Delete removes existing risk", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Risk().Create(ctx, &model.Risk{
			Name:        "To Be Deleted",
			Description: "This will be deleted",
		})
		if err != nil {
			t.Fatalf("failed to create risk: %v", err)
		}

		err = repo.Risk().Delete(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to delete risk: %v", err)
		}

		// Verify it's gone
		_, err = repo.Risk().Get(ctx, created.ID)
		if err == nil {
			t.Error("expected error when getting deleted risk")
		}
		if !errors.Is(err, memory.ErrNotFound) && !errors.Is(err, firestore.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Delete returns error for non-existent risk", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		err := repo.Risk().Delete(ctx, 99999)
		if err == nil {
			t.Error("expected error for non-existent risk")
		}
		if !errors.Is(err, memory.ErrNotFound) && !errors.Is(err, firestore.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("SlackChannelID is persisted and retrieved", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create risk with SlackChannelID
		created, err := repo.Risk().Create(ctx, &model.Risk{
			Name:           "Risk with Slack Channel",
			Description:    "This risk has a Slack channel",
			SlackChannelID: "C12345ABCDE",
		})
		if err != nil {
			t.Fatalf("failed to create risk: %v", err)
		}

		if created.SlackChannelID != "C12345ABCDE" {
			t.Errorf("expected SlackChannelID='C12345ABCDE', got %s", created.SlackChannelID)
		}

		// Retrieve and verify SlackChannelID
		retrieved, err := repo.Risk().Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to get risk: %v", err)
		}

		if retrieved.SlackChannelID != "C12345ABCDE" {
			t.Errorf("expected SlackChannelID='C12345ABCDE' after retrieval, got %s", retrieved.SlackChannelID)
		}

		// Update SlackChannelID
		time.Sleep(10 * time.Millisecond)
		updated, err := repo.Risk().Update(ctx, &model.Risk{
			ID:             created.ID,
			Name:           created.Name,
			Description:    created.Description,
			SlackChannelID: "C67890FGHIJ",
		})
		if err != nil {
			t.Fatalf("failed to update risk: %v", err)
		}

		if updated.SlackChannelID != "C67890FGHIJ" {
			t.Errorf("expected SlackChannelID='C67890FGHIJ' after update, got %s", updated.SlackChannelID)
		}

		// Verify updated value via Get
		retrieved2, err := repo.Risk().Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to get updated risk: %v", err)
		}

		if retrieved2.SlackChannelID != "C67890FGHIJ" {
			t.Errorf("expected SlackChannelID='C67890FGHIJ' after update retrieval, got %s", retrieved2.SlackChannelID)
		}
	})

	t.Run("Empty SlackChannelID is handled correctly", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create risk without SlackChannelID
		created, err := repo.Risk().Create(ctx, &model.Risk{
			Name:        "Risk without Slack Channel",
			Description: "This risk has no Slack channel",
		})
		if err != nil {
			t.Fatalf("failed to create risk: %v", err)
		}

		if created.SlackChannelID != "" {
			t.Errorf("expected empty SlackChannelID, got %s", created.SlackChannelID)
		}

		// Retrieve and verify empty SlackChannelID
		retrieved, err := repo.Risk().Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to get risk: %v", err)
		}

		if retrieved.SlackChannelID != "" {
			t.Errorf("expected empty SlackChannelID after retrieval, got %s", retrieved.SlackChannelID)
		}
	})
}

func newFirestoreRepository(t *testing.T) interfaces.Repository {
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
	prefix := fmt.Sprintf("test_%d", time.Now().UnixNano())
	repo, err := firestore.New(ctx, projectID, databaseID, firestore.WithCollectionPrefix(prefix))
	if err != nil {
		t.Fatalf("failed to create firestore repository: %v", err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Errorf("failed to close firestore repository: %v", err)
		}
	})
	return repo
}

func TestMemoryRiskRepository(t *testing.T) {
	runRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestFirestoreRiskRepository(t *testing.T) {
	runRepositoryTest(t, newFirestoreRepository)
}
