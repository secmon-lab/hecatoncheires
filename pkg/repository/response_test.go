package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runResponseRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	t.Run("Create creates response with auto-increment ID", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		response1 := &model.Response{
			Title:        "Patch System",
			Description:  "Apply security patches",
			ResponderIDs: []string{"U12345", "U67890"},
			URL:          "https://example.com/patch",
			Status:       types.ResponseStatusTodo,
		}

		created1, err := repo.Response().Create(ctx, response1)
		if err != nil {
			t.Fatalf("failed to create response1: %v", err)
		}

		if created1.ID == 0 {
			t.Error("expected non-zero ID")
		}
		if created1.Title != response1.Title {
			t.Errorf("expected title=%s, got %s", response1.Title, created1.Title)
		}
		if created1.Description != response1.Description {
			t.Errorf("expected description=%s, got %s", response1.Description, created1.Description)
		}
		if len(created1.ResponderIDs) != 2 {
			t.Errorf("expected 2 responders, got %d", len(created1.ResponderIDs))
		}
		if created1.ResponderIDs[0] != "U12345" || created1.ResponderIDs[1] != "U67890" {
			t.Errorf("responder IDs mismatch: got %v", created1.ResponderIDs)
		}
		if created1.URL != response1.URL {
			t.Errorf("expected url=%s, got %s", response1.URL, created1.URL)
		}
		if created1.Status != response1.Status {
			t.Errorf("expected status=%s, got %s", response1.Status, created1.Status)
		}
		if created1.CreatedAt.IsZero() {
			t.Error("expected non-zero CreatedAt")
		}
		if created1.UpdatedAt.IsZero() {
			t.Error("expected non-zero UpdatedAt")
		}

		// Create second response to test auto-increment
		response2 := &model.Response{
			Title:        "Update Firewall",
			Description:  "Block malicious IPs",
			ResponderIDs: []string{"U11111"},
			Status:       types.ResponseStatusInProgress,
		}

		created2, err := repo.Response().Create(ctx, response2)
		if err != nil {
			t.Fatalf("failed to create response2: %v", err)
		}

		if created2.ID <= created1.ID {
			t.Errorf("expected ID > %d, got %d", created1.ID, created2.ID)
		}
	})

	t.Run("Get retrieves existing response", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Response().Create(ctx, &model.Response{
			Title:        "Monitor Logs",
			Description:  "Set up log monitoring",
			ResponderIDs: []string{"U22222"},
			URL:          "https://example.com/logs",
			Status:       types.ResponseStatusBacklog,
		})
		if err != nil {
			t.Fatalf("failed to create response: %v", err)
		}

		retrieved, err := repo.Response().Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to get response: %v", err)
		}

		if retrieved.ID != created.ID {
			t.Errorf("expected ID=%d, got %d", created.ID, retrieved.ID)
		}
		if retrieved.Title != created.Title {
			t.Errorf("expected title=%s, got %s", created.Title, retrieved.Title)
		}
		if retrieved.Description != created.Description {
			t.Errorf("expected description=%s, got %s", created.Description, retrieved.Description)
		}
		if len(retrieved.ResponderIDs) != len(created.ResponderIDs) {
			t.Errorf("expected %d responders, got %d", len(created.ResponderIDs), len(retrieved.ResponderIDs))
		}
		if retrieved.URL != created.URL {
			t.Errorf("expected url=%s, got %s", created.URL, retrieved.URL)
		}
		if retrieved.Status != created.Status {
			t.Errorf("expected status=%s, got %s", created.Status, retrieved.Status)
		}
		if !retrieved.CreatedAt.Equal(created.CreatedAt) {
			t.Errorf("expected createdAt=%v, got %v", created.CreatedAt, retrieved.CreatedAt)
		}
		if !retrieved.UpdatedAt.Equal(created.UpdatedAt) {
			t.Errorf("expected updatedAt=%v, got %v", created.UpdatedAt, retrieved.UpdatedAt)
		}
	})

	t.Run("Get returns error for non-existent response", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		_, err := repo.Response().Get(ctx, 99999)
		if err == nil {
			t.Error("expected error for non-existent response")
		}
		if !errors.Is(err, memory.ErrNotFound) && !errors.Is(err, firestore.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("List returns all responses", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Initially empty
		responses, err := repo.Response().List(ctx)
		if err != nil {
			t.Fatalf("failed to list responses: %v", err)
		}
		initialCount := len(responses)

		// Create responses
		_, err = repo.Response().Create(ctx, &model.Response{
			Title:        "Response 1",
			Description:  "First response",
			ResponderIDs: []string{"U1"},
			Status:       types.ResponseStatusTodo,
		})
		if err != nil {
			t.Fatalf("failed to create response 1: %v", err)
		}

		_, err = repo.Response().Create(ctx, &model.Response{
			Title:        "Response 2",
			Description:  "Second response",
			ResponderIDs: []string{"U2"},
			Status:       types.ResponseStatusCompleted,
		})
		if err != nil {
			t.Fatalf("failed to create response 2: %v", err)
		}

		// List should return both
		responses, err = repo.Response().List(ctx)
		if err != nil {
			t.Fatalf("failed to list responses: %v", err)
		}

		if len(responses) != initialCount+2 {
			t.Errorf("expected %d responses, got %d", initialCount+2, len(responses))
		}
	})

	t.Run("Update modifies existing response", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Response().Create(ctx, &model.Response{
			Title:        "Original Title",
			Description:  "Original Description",
			ResponderIDs: []string{"U1"},
			Status:       types.ResponseStatusBacklog,
		})
		if err != nil {
			t.Fatalf("failed to create response: %v", err)
		}

		// Wait a bit to ensure UpdatedAt changes
		time.Sleep(10 * time.Millisecond)

		created.Title = "Updated Title"
		created.Description = "Updated Description"
		created.ResponderIDs = []string{"U2", "U3"}
		created.Status = types.ResponseStatusInProgress

		updated, err := repo.Response().Update(ctx, created)
		if err != nil {
			t.Fatalf("failed to update response: %v", err)
		}

		if updated.ID != created.ID {
			t.Errorf("ID should not change, got %d", updated.ID)
		}
		if updated.Title != "Updated Title" {
			t.Errorf("expected title='Updated Title', got %s", updated.Title)
		}
		if updated.Description != "Updated Description" {
			t.Errorf("expected description='Updated Description', got %s", updated.Description)
		}
		if len(updated.ResponderIDs) != 2 {
			t.Errorf("expected 2 responders, got %d", len(updated.ResponderIDs))
		}
		if updated.Status != types.ResponseStatusInProgress {
			t.Errorf("expected status=%s, got %s", types.ResponseStatusInProgress, updated.Status)
		}
		if !updated.CreatedAt.Equal(created.CreatedAt) {
			t.Errorf("CreatedAt should not change, got %v", updated.CreatedAt)
		}
		if !updated.UpdatedAt.After(created.UpdatedAt) {
			t.Errorf("UpdatedAt should be after original, got %v", updated.UpdatedAt)
		}

		// Verify via Get
		retrieved, err := repo.Response().Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to get updated response: %v", err)
		}
		if retrieved.Title != "Updated Title" {
			t.Errorf("expected title='Updated Title' after retrieval, got %s", retrieved.Title)
		}
	})

	t.Run("Update returns error for non-existent response", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		_, err := repo.Response().Update(ctx, &model.Response{
			ID:          99999,
			Title:       "Non-existent",
			Description: "Should fail",
			Status:      types.ResponseStatusTodo,
		})
		if err == nil {
			t.Error("expected error for non-existent response")
		}
		if !errors.Is(err, memory.ErrNotFound) && !errors.Is(err, firestore.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Delete removes existing response", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Response().Create(ctx, &model.Response{
			Title:        "To Be Deleted",
			Description:  "This will be deleted",
			ResponderIDs: []string{"U1"},
			Status:       types.ResponseStatusAbandoned,
		})
		if err != nil {
			t.Fatalf("failed to create response: %v", err)
		}

		err = repo.Response().Delete(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to delete response: %v", err)
		}

		// Verify deletion
		_, err = repo.Response().Get(ctx, created.ID)
		if err == nil {
			t.Error("expected error when getting deleted response")
		}
		if !errors.Is(err, memory.ErrNotFound) && !errors.Is(err, firestore.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Delete returns error for non-existent response", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		err := repo.Response().Delete(ctx, 99999)
		if err == nil {
			t.Error("expected error for non-existent response")
		}
		if !errors.Is(err, memory.ErrNotFound) && !errors.Is(err, firestore.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Create with empty responder IDs", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		response := &model.Response{
			Title:        "No Responders",
			Description:  "Response without responders",
			ResponderIDs: []string{},
			Status:       types.ResponseStatusBacklog,
		}

		created, err := repo.Response().Create(ctx, response)
		if err != nil {
			t.Fatalf("failed to create response: %v", err)
		}

		if created.ResponderIDs == nil {
			t.Error("ResponderIDs should not be nil")
		}
		if len(created.ResponderIDs) != 0 {
			t.Errorf("expected 0 responders, got %d", len(created.ResponderIDs))
		}
	})
}

func TestMemoryResponseRepository(t *testing.T) {
	runResponseRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestFirestoreResponseRepository(t *testing.T) {
	runResponseRepositoryTest(t, newFirestoreRepository)
}
