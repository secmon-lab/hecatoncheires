package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runSlackUserRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	t.Run("SaveMany and GetAll with empty list", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Save empty list
		if err := repo.SlackUser().SaveMany(ctx, []*model.SlackUser{}); err != nil {
			t.Fatalf("failed to save empty list: %v", err)
		}

		// GetAll should return empty list
		users, err := repo.SlackUser().GetAll(ctx)
		if err != nil {
			t.Fatalf("failed to get all users: %v", err)
		}

		if len(users) != 0 {
			t.Errorf("expected 0 users, got %d", len(users))
		}
	})

	t.Run("SaveMany and GetAll with single user", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		// Use random ID to avoid conflicts
		userID := model.SlackUserID(fmt.Sprintf("U%d", now.UnixNano()))

		user := &model.SlackUser{
			ID:        userID,
			Name:      "john.doe",
			RealName:  "John Doe",
			Email:     "john.doe@example.com",
			ImageURL:  "https://example.com/avatar.jpg",
			UpdatedAt: now,
		}

		// Save single user
		if err := repo.SlackUser().SaveMany(ctx, []*model.SlackUser{user}); err != nil {
			t.Fatalf("failed to save user: %v", err)
		}

		// GetAll should return 1 user
		users, err := repo.SlackUser().GetAll(ctx)
		if err != nil {
			t.Fatalf("failed to get all users: %v", err)
		}

		if len(users) != 1 {
			t.Fatalf("expected 1 user, got %d", len(users))
		}

		// Verify all fields
		got := users[0]
		if got.ID != user.ID {
			t.Errorf("ID mismatch: expected %q, got %q", user.ID, got.ID)
		}
		if got.Name != user.Name {
			t.Errorf("Name mismatch: expected %q, got %q", user.Name, got.Name)
		}
		if got.RealName != user.RealName {
			t.Errorf("RealName mismatch: expected %q, got %q", user.RealName, got.RealName)
		}
		if got.Email != user.Email {
			t.Errorf("Email mismatch: expected %q, got %q", user.Email, got.Email)
		}
		if got.ImageURL != user.ImageURL {
			t.Errorf("ImageURL mismatch: expected %q, got %q", user.ImageURL, got.ImageURL)
		}
		if got.UpdatedAt.Sub(user.UpdatedAt).Abs() > time.Second {
			t.Errorf("UpdatedAt mismatch: expected %v, got %v", user.UpdatedAt, got.UpdatedAt)
		}
	})

	t.Run("SaveMany and GetAll with multiple users", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		// Create multiple users with random IDs
		users := []*model.SlackUser{
			{
				ID:        model.SlackUserID(fmt.Sprintf("U%d_1", now.UnixNano())),
				Name:      "user1",
				RealName:  "User One",
				Email:     "user1@example.com",
				ImageURL:  "https://example.com/avatar1.jpg",
				UpdatedAt: now,
			},
			{
				ID:        model.SlackUserID(fmt.Sprintf("U%d_2", now.UnixNano())),
				Name:      "user2",
				RealName:  "User Two",
				Email:     "user2@example.com",
				ImageURL:  "https://example.com/avatar2.jpg",
				UpdatedAt: now,
			},
			{
				ID:        model.SlackUserID(fmt.Sprintf("U%d_3", now.UnixNano())),
				Name:      "user3",
				RealName:  "User Three",
				Email:     "user3@example.com",
				ImageURL:  "",
				UpdatedAt: now,
			},
		}

		// Save multiple users
		if err := repo.SlackUser().SaveMany(ctx, users); err != nil {
			t.Fatalf("failed to save users: %v", err)
		}

		// GetAll should return 3 users
		gotUsers, err := repo.SlackUser().GetAll(ctx)
		if err != nil {
			t.Fatalf("failed to get all users: %v", err)
		}

		if len(gotUsers) != 3 {
			t.Fatalf("expected 3 users, got %d", len(gotUsers))
		}

		// Verify all users are present (order may vary)
		userMap := make(map[model.SlackUserID]*model.SlackUser)
		for _, u := range gotUsers {
			userMap[u.ID] = u
		}

		for _, expected := range users {
			got, ok := userMap[expected.ID]
			if !ok {
				t.Errorf("user %q not found", expected.ID)
				continue
			}

			if got.Name != expected.Name {
				t.Errorf("Name mismatch for %q: expected %q, got %q", expected.ID, expected.Name, got.Name)
			}
			if got.RealName != expected.RealName {
				t.Errorf("RealName mismatch for %q: expected %q, got %q", expected.ID, expected.RealName, got.RealName)
			}
			if got.Email != expected.Email {
				t.Errorf("Email mismatch for %q: expected %q, got %q", expected.ID, expected.Email, got.Email)
			}
			if got.ImageURL != expected.ImageURL {
				t.Errorf("ImageURL mismatch for %q: expected %q, got %q", expected.ID, expected.ImageURL, got.ImageURL)
			}
		}
	})

	t.Run("SaveMany with >500 users", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		// Create 600 users to test Firestore batch limit
		users := make([]*model.SlackUser, 600)
		for i := 0; i < 600; i++ {
			users[i] = &model.SlackUser{
				ID:        model.SlackUserID(fmt.Sprintf("U%d_%d", now.UnixNano(), i)),
				Name:      fmt.Sprintf("user%d", i),
				RealName:  fmt.Sprintf("User %d", i),
				Email:     fmt.Sprintf("user%d@example.com", i),
				ImageURL:  "",
				UpdatedAt: now,
			}
		}

		// Save 600 users
		if err := repo.SlackUser().SaveMany(ctx, users); err != nil {
			t.Fatalf("failed to save 600 users: %v", err)
		}

		// GetAll should return 600 users
		gotUsers, err := repo.SlackUser().GetAll(ctx)
		if err != nil {
			t.Fatalf("failed to get all users: %v", err)
		}

		if len(gotUsers) != 600 {
			t.Errorf("expected 600 users, got %d", len(gotUsers))
		}
	})

	t.Run("GetByID returns user", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		userID := model.SlackUserID(fmt.Sprintf("U%d", now.UnixNano()))
		user := &model.SlackUser{
			ID:        userID,
			Name:      "alice",
			RealName:  "Alice Smith",
			Email:     "alice@example.com",
			ImageURL:  "https://example.com/alice.jpg",
			UpdatedAt: now,
		}

		// Save user
		if err := repo.SlackUser().SaveMany(ctx, []*model.SlackUser{user}); err != nil {
			t.Fatalf("failed to save user: %v", err)
		}

		// GetByID should return the user
		got, err := repo.SlackUser().GetByID(ctx, userID)
		if err != nil {
			t.Fatalf("failed to get user by ID: %v", err)
		}

		if got.ID != user.ID {
			t.Errorf("ID mismatch: expected %q, got %q", user.ID, got.ID)
		}
		if got.Name != user.Name {
			t.Errorf("Name mismatch: expected %q, got %q", user.Name, got.Name)
		}
		if got.RealName != user.RealName {
			t.Errorf("RealName mismatch: expected %q, got %q", user.RealName, got.RealName)
		}
	})

	t.Run("GetByID returns NotFound for missing user", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		missingID := model.SlackUserID(fmt.Sprintf("U_MISSING_%d", now.UnixNano()))

		// GetByID should return NotFound error
		_, err := repo.SlackUser().GetByID(ctx, missingID)
		if err == nil {
			t.Fatal("expected error for missing user, got nil")
		}

		// Check if error is ErrNotFound (implementation-specific check)
		// The error should contain "not found" in the message
		if err.Error() == "" {
			t.Error("expected non-empty error message")
		}
	})

	t.Run("GetByIDs batches with <10 users", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		// Create 5 users
		users := make([]*model.SlackUser, 5)
		ids := make([]model.SlackUserID, 5)
		for i := 0; i < 5; i++ {
			id := model.SlackUserID(fmt.Sprintf("U%d_%d", now.UnixNano(), i))
			users[i] = &model.SlackUser{
				ID:        id,
				Name:      fmt.Sprintf("user%d", i),
				RealName:  fmt.Sprintf("User %d", i),
				Email:     fmt.Sprintf("user%d@example.com", i),
				ImageURL:  "",
				UpdatedAt: now,
			}
			ids[i] = id
		}

		// Save users
		if err := repo.SlackUser().SaveMany(ctx, users); err != nil {
			t.Fatalf("failed to save users: %v", err)
		}

		// GetByIDs should return all 5 users
		gotMap, err := repo.SlackUser().GetByIDs(ctx, ids)
		if err != nil {
			t.Fatalf("failed to get users by IDs: %v", err)
		}

		if len(gotMap) != 5 {
			t.Errorf("expected 5 users, got %d", len(gotMap))
		}

		for _, id := range ids {
			if _, ok := gotMap[id]; !ok {
				t.Errorf("user %q not found in result", id)
			}
		}
	})

	t.Run("GetByIDs batches with >10 users", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		// Create 15 users to test Firestore GetAll limit (10)
		users := make([]*model.SlackUser, 15)
		ids := make([]model.SlackUserID, 15)
		for i := 0; i < 15; i++ {
			id := model.SlackUserID(fmt.Sprintf("U%d_%d", now.UnixNano(), i))
			users[i] = &model.SlackUser{
				ID:        id,
				Name:      fmt.Sprintf("user%d", i),
				RealName:  fmt.Sprintf("User %d", i),
				Email:     fmt.Sprintf("user%d@example.com", i),
				ImageURL:  "",
				UpdatedAt: now,
			}
			ids[i] = id
		}

		// Save users
		if err := repo.SlackUser().SaveMany(ctx, users); err != nil {
			t.Fatalf("failed to save users: %v", err)
		}

		// GetByIDs should return all 15 users
		gotMap, err := repo.SlackUser().GetByIDs(ctx, ids)
		if err != nil {
			t.Fatalf("failed to get users by IDs: %v", err)
		}

		if len(gotMap) != 15 {
			t.Errorf("expected 15 users, got %d", len(gotMap))
		}

		for _, id := range ids {
			if _, ok := gotMap[id]; !ok {
				t.Errorf("user %q not found in result", id)
			}
		}
	})

	t.Run("GetByIDs batches with >20 users", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		// Create 25 users to test multiple batches
		users := make([]*model.SlackUser, 25)
		ids := make([]model.SlackUserID, 25)
		for i := 0; i < 25; i++ {
			id := model.SlackUserID(fmt.Sprintf("U%d_%d", now.UnixNano(), i))
			users[i] = &model.SlackUser{
				ID:        id,
				Name:      fmt.Sprintf("user%d", i),
				RealName:  fmt.Sprintf("User %d", i),
				Email:     fmt.Sprintf("user%d@example.com", i),
				ImageURL:  "",
				UpdatedAt: now,
			}
			ids[i] = id
		}

		// Save users
		if err := repo.SlackUser().SaveMany(ctx, users); err != nil {
			t.Fatalf("failed to save users: %v", err)
		}

		// GetByIDs should return all 25 users
		gotMap, err := repo.SlackUser().GetByIDs(ctx, ids)
		if err != nil {
			t.Fatalf("failed to get users by IDs: %v", err)
		}

		if len(gotMap) != 25 {
			t.Errorf("expected 25 users, got %d", len(gotMap))
		}

		for _, id := range ids {
			if _, ok := gotMap[id]; !ok {
				t.Errorf("user %q not found in result", id)
			}
		}
	})

	t.Run("GetByIDs with missing users", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		// Create 3 users
		users := make([]*model.SlackUser, 3)
		existingIDs := make([]model.SlackUserID, 3)
		for i := 0; i < 3; i++ {
			id := model.SlackUserID(fmt.Sprintf("U%d_%d", now.UnixNano(), i))
			users[i] = &model.SlackUser{
				ID:        id,
				Name:      fmt.Sprintf("user%d", i),
				RealName:  fmt.Sprintf("User %d", i),
				Email:     fmt.Sprintf("user%d@example.com", i),
				ImageURL:  "",
				UpdatedAt: now,
			}
			existingIDs[i] = id
		}

		// Save users
		if err := repo.SlackUser().SaveMany(ctx, users); err != nil {
			t.Fatalf("failed to save users: %v", err)
		}

		// Request existing + missing users
		missingID := model.SlackUserID(fmt.Sprintf("U_MISSING_%d", now.UnixNano()))
		requestedIDs := append(existingIDs, missingID)

		// GetByIDs should return only existing users (missing users not included)
		gotMap, err := repo.SlackUser().GetByIDs(ctx, requestedIDs)
		if err != nil {
			t.Fatalf("failed to get users by IDs: %v", err)
		}

		if len(gotMap) != 3 {
			t.Errorf("expected 3 users (missing excluded), got %d", len(gotMap))
		}

		for _, id := range existingIDs {
			if _, ok := gotMap[id]; !ok {
				t.Errorf("existing user %q not found in result", id)
			}
		}

		if _, ok := gotMap[missingID]; ok {
			t.Errorf("missing user %q should not be in result", missingID)
		}
	})

	t.Run("DeleteAll removes all users", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		// Create and save 3 users
		users := make([]*model.SlackUser, 3)
		for i := 0; i < 3; i++ {
			users[i] = &model.SlackUser{
				ID:        model.SlackUserID(fmt.Sprintf("U%d_%d", now.UnixNano(), i)),
				Name:      fmt.Sprintf("user%d", i),
				RealName:  fmt.Sprintf("User %d", i),
				Email:     fmt.Sprintf("user%d@example.com", i),
				ImageURL:  "",
				UpdatedAt: now,
			}
		}

		if err := repo.SlackUser().SaveMany(ctx, users); err != nil {
			t.Fatalf("failed to save users: %v", err)
		}

		// Verify users exist
		gotUsers, err := repo.SlackUser().GetAll(ctx)
		if err != nil {
			t.Fatalf("failed to get all users: %v", err)
		}
		if len(gotUsers) != 3 {
			t.Fatalf("expected 3 users before delete, got %d", len(gotUsers))
		}

		// DeleteAll
		if err := repo.SlackUser().DeleteAll(ctx); err != nil {
			t.Fatalf("failed to delete all users: %v", err)
		}

		// Verify all users are deleted
		gotUsers, err = repo.SlackUser().GetAll(ctx)
		if err != nil {
			t.Fatalf("failed to get all users after delete: %v", err)
		}
		if len(gotUsers) != 0 {
			t.Errorf("expected 0 users after delete, got %d", len(gotUsers))
		}
	})

	t.Run("DeleteAll with >500 users", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		// Create 600 users to test Firestore batch delete limit
		users := make([]*model.SlackUser, 600)
		for i := 0; i < 600; i++ {
			users[i] = &model.SlackUser{
				ID:        model.SlackUserID(fmt.Sprintf("U%d_%d", now.UnixNano(), i)),
				Name:      fmt.Sprintf("user%d", i),
				RealName:  fmt.Sprintf("User %d", i),
				Email:     fmt.Sprintf("user%d@example.com", i),
				ImageURL:  "",
				UpdatedAt: now,
			}
		}

		if err := repo.SlackUser().SaveMany(ctx, users); err != nil {
			t.Fatalf("failed to save users: %v", err)
		}

		// DeleteAll
		if err := repo.SlackUser().DeleteAll(ctx); err != nil {
			t.Fatalf("failed to delete all users: %v", err)
		}

		// Verify all users are deleted
		gotUsers, err := repo.SlackUser().GetAll(ctx)
		if err != nil {
			t.Fatalf("failed to get all users after delete: %v", err)
		}
		if len(gotUsers) != 0 {
			t.Errorf("expected 0 users after delete, got %d", len(gotUsers))
		}
	})

	t.Run("SaveMany overwrites existing users", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		userID := model.SlackUserID(fmt.Sprintf("U%d", now.UnixNano()))

		// Save initial user
		user1 := &model.SlackUser{
			ID:        userID,
			Name:      "alice.old",
			RealName:  "Alice Old",
			Email:     "alice.old@example.com",
			ImageURL:  "https://example.com/old.jpg",
			UpdatedAt: now,
		}

		if err := repo.SlackUser().SaveMany(ctx, []*model.SlackUser{user1}); err != nil {
			t.Fatalf("failed to save initial user: %v", err)
		}

		// Overwrite with updated user
		user2 := &model.SlackUser{
			ID:        userID,
			Name:      "alice.new",
			RealName:  "Alice New",
			Email:     "alice.new@example.com",
			ImageURL:  "https://example.com/new.jpg",
			UpdatedAt: now.Add(time.Hour),
		}

		if err := repo.SlackUser().SaveMany(ctx, []*model.SlackUser{user2}); err != nil {
			t.Fatalf("failed to overwrite user: %v", err)
		}

		// GetByID should return updated user
		got, err := repo.SlackUser().GetByID(ctx, userID)
		if err != nil {
			t.Fatalf("failed to get user by ID: %v", err)
		}

		if got.Name != "alice.new" {
			t.Errorf("Name not updated: expected %q, got %q", "alice.new", got.Name)
		}
		if got.RealName != "Alice New" {
			t.Errorf("RealName not updated: expected %q, got %q", "Alice New", got.RealName)
		}
		if got.Email != "alice.new@example.com" {
			t.Errorf("Email not updated: expected %q, got %q", "alice.new@example.com", got.Email)
		}
		if got.ImageURL != "https://example.com/new.jpg" {
			t.Errorf("ImageURL not updated: expected %q, got %q", "https://example.com/new.jpg", got.ImageURL)
		}
	})

	t.Run("Metadata operations", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		// GetMetadata should return zero value when no metadata exists
		metadata, err := repo.SlackUser().GetMetadata(ctx)
		if err != nil {
			t.Fatalf("failed to get metadata: %v", err)
		}

		if !metadata.LastRefreshSuccess.IsZero() && metadata.LastRefreshSuccess.Unix() != 0 {
			t.Errorf("expected zero LastRefreshSuccess, got %v", metadata.LastRefreshSuccess)
		}
		if !metadata.LastRefreshAttempt.IsZero() && metadata.LastRefreshAttempt.Unix() != 0 {
			t.Errorf("expected zero LastRefreshAttempt, got %v", metadata.LastRefreshAttempt)
		}
		if metadata.UserCount != 0 {
			t.Errorf("expected zero UserCount, got %d", metadata.UserCount)
		}

		// Save metadata
		newMetadata := &model.SlackUserMetadata{
			LastRefreshSuccess: now,
			LastRefreshAttempt: now.Add(time.Minute),
			UserCount:          356,
		}

		if err := repo.SlackUser().SaveMetadata(ctx, newMetadata); err != nil {
			t.Fatalf("failed to save metadata: %v", err)
		}

		// Get metadata should return saved values
		got, err := repo.SlackUser().GetMetadata(ctx)
		if err != nil {
			t.Fatalf("failed to get metadata after save: %v", err)
		}

		if got.LastRefreshSuccess.Sub(newMetadata.LastRefreshSuccess).Abs() > time.Second {
			t.Errorf("LastRefreshSuccess mismatch: expected %v, got %v", newMetadata.LastRefreshSuccess, got.LastRefreshSuccess)
		}
		if got.LastRefreshAttempt.Sub(newMetadata.LastRefreshAttempt).Abs() > time.Second {
			t.Errorf("LastRefreshAttempt mismatch: expected %v, got %v", newMetadata.LastRefreshAttempt, got.LastRefreshAttempt)
		}
		if got.UserCount != newMetadata.UserCount {
			t.Errorf("UserCount mismatch: expected %d, got %d", newMetadata.UserCount, got.UserCount)
		}

		// Update metadata
		updatedMetadata := &model.SlackUserMetadata{
			LastRefreshSuccess: now.Add(time.Hour),
			LastRefreshAttempt: now.Add(time.Hour + time.Minute),
			UserCount:          400,
		}

		if err := repo.SlackUser().SaveMetadata(ctx, updatedMetadata); err != nil {
			t.Fatalf("failed to update metadata: %v", err)
		}

		// Get metadata should return updated values
		got, err = repo.SlackUser().GetMetadata(ctx)
		if err != nil {
			t.Fatalf("failed to get metadata after update: %v", err)
		}

		if got.UserCount != 400 {
			t.Errorf("UserCount not updated: expected %d, got %d", 400, got.UserCount)
		}
	})
}

func TestMemorySlackUserRepository(t *testing.T) {
	runSlackUserRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestFirestoreSlackUserRepository(t *testing.T) {
	runSlackUserRepositoryTest(t, newFirestoreRepository)
}
