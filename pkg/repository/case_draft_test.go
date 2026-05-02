package repository_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runCaseDraftRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Run("Save and Get", func(t *testing.T) {
		repo := newRepo(t).CaseDraft()
		ctx := context.Background()

		now := time.Now().UTC()
		d := model.NewCaseDraft(now, "U_creator")
		d.MentionText = "please look at this"
		d.SelectedWorkspaceID = "ws-1"
		d.Source = model.DraftSource{
			TeamID:    "T1",
			ChannelID: "C1",
			ThreadTS:  "1700000000.000100",
			MentionTS: "1700000001.000200",
		}
		d.RawMessages = []model.DraftMessage{
			{UserID: "U1", Text: "hi", TS: "1700000000.000001", Permalink: "https://slack/p1"},
			{UserID: "U2", Text: "yo", TS: "1700000000.000002", Permalink: "https://slack/p2"},
		}
		d.EphemeralChannelID = "C1"
		d.EphemeralMessageTS = "1700000005.000900"

		gt.NoError(t, repo.Save(ctx, d)).Required()

		got, err := repo.Get(ctx, d.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, got.ID).Equal(d.ID)
		gt.Value(t, got.CreatedBy).Equal(d.CreatedBy)
		gt.Value(t, got.MentionText).Equal(d.MentionText)
		gt.Value(t, got.SelectedWorkspaceID).Equal(d.SelectedWorkspaceID)
		gt.Value(t, got.Source.TeamID).Equal(d.Source.TeamID)
		gt.Value(t, got.Source.ChannelID).Equal(d.Source.ChannelID)
		gt.Value(t, got.Source.ThreadTS).Equal(d.Source.ThreadTS)
		gt.Value(t, got.Source.MentionTS).Equal(d.Source.MentionTS)
		gt.Value(t, got.EphemeralChannelID).Equal(d.EphemeralChannelID)
		gt.Value(t, got.EphemeralMessageTS).Equal(d.EphemeralMessageTS)
		gt.Array(t, got.RawMessages).Length(2)
		gt.Value(t, got.RawMessages[0].Text).Equal("hi")
		gt.Value(t, got.RawMessages[1].Permalink).Equal("https://slack/p2")
		gt.Bool(t, got.CreatedAt.Sub(d.CreatedAt).Abs() < time.Second).True()
		gt.Bool(t, got.ExpiresAt.Sub(d.ExpiresAt).Abs() < time.Second).True()
		gt.Value(t, got.Materialization).Nil()
		gt.Bool(t, got.InferenceInProgress).False()
	})

	t.Run("Get not found", func(t *testing.T) {
		repo := newRepo(t).CaseDraft()
		ctx := context.Background()

		_, err := repo.Get(ctx, model.NewCaseDraftID())
		gt.Value(t, err).NotNil().Required()
		gt.Bool(t, errors.Is(err, firestore.ErrNotFound) || errors.Is(err, memory.ErrNotFound)).True()
	})

	t.Run("SetMaterialization sets and overwrites", func(t *testing.T) {
		repo := newRepo(t).CaseDraft()
		ctx := context.Background()

		d := model.NewCaseDraft(time.Now().UTC(), "U_x")
		d.SelectedWorkspaceID = "ws-A"
		gt.NoError(t, repo.Save(ctx, d)).Required()

		// First mark inference in progress (no materialization yet).
		gt.NoError(t, repo.SetMaterialization(ctx, d.ID, "ws-A", nil, true)).Required()

		got, err := repo.Get(ctx, d.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.SelectedWorkspaceID).Equal("ws-A")
		gt.Bool(t, got.InferenceInProgress).True()
		gt.Value(t, got.Materialization).Nil()

		// Then put a materialization (inference completed).
		mat := &model.WorkspaceMaterialization{
			GeneratedAt: time.Now().UTC(),
			Title:       "Investigate suspicious login",
			Description: "Multiple failed sign-ins from new ASN.",
			CustomFieldValues: map[string]model.FieldValue{
				"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "high"},
				"count":    {FieldID: "count", Type: types.FieldTypeNumber, Value: int64(7)},
			},
		}
		gt.NoError(t, repo.SetMaterialization(ctx, d.ID, "ws-A", mat, false)).Required()

		got, err = repo.Get(ctx, d.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.SelectedWorkspaceID).Equal("ws-A")
		gt.Bool(t, got.InferenceInProgress).False()
		gt.Value(t, got.Materialization).NotNil().Required()
		gt.Value(t, got.Materialization.Title).Equal(mat.Title)
		gt.Value(t, got.Materialization.Description).Equal(mat.Description)
		gt.Map(t, got.Materialization.CustomFieldValues).HasKey("severity")
		gt.Map(t, got.Materialization.CustomFieldValues).HasKey("count")
		gt.Value(t, got.Materialization.CustomFieldValues["severity"].Value).Equal("high")

		// Overwrite by switching workspace.
		mat2 := &model.WorkspaceMaterialization{
			GeneratedAt: time.Now().UTC(),
			Title:       "別ワークスペース版",
			Description: "different schema content",
			CustomFieldValues: map[string]model.FieldValue{
				"category": {FieldID: "category", Type: types.FieldTypeText, Value: "security"},
			},
		}
		gt.NoError(t, repo.SetMaterialization(ctx, d.ID, "ws-B", mat2, false)).Required()

		got, err = repo.Get(ctx, d.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.SelectedWorkspaceID).Equal("ws-B")
		gt.Value(t, got.Materialization).NotNil().Required()
		gt.Value(t, got.Materialization.Title).Equal("別ワークスペース版")
		gt.Map(t, got.Materialization.CustomFieldValues).HasKey("category")
		_, hadOldKey := got.Materialization.CustomFieldValues["severity"]
		gt.Bool(t, hadOldKey).False()
	})

	t.Run("SetMaterialization not found", func(t *testing.T) {
		repo := newRepo(t).CaseDraft()
		ctx := context.Background()

		err := repo.SetMaterialization(ctx, model.NewCaseDraftID(), "ws", nil, false)
		gt.Value(t, err).NotNil().Required()
		gt.Bool(t, errors.Is(err, firestore.ErrNotFound) || errors.Is(err, memory.ErrNotFound)).True()
	})

	t.Run("Delete", func(t *testing.T) {
		repo := newRepo(t).CaseDraft()
		ctx := context.Background()

		d := model.NewCaseDraft(time.Now().UTC(), "U_d")
		gt.NoError(t, repo.Save(ctx, d)).Required()

		gt.NoError(t, repo.Delete(ctx, d.ID)).Required()

		_, err := repo.Get(ctx, d.ID)
		gt.Value(t, err).NotNil().Required()
		gt.Bool(t, errors.Is(err, firestore.ErrNotFound) || errors.Is(err, memory.ErrNotFound)).True()

		err = repo.Delete(ctx, d.ID)
		gt.Value(t, err).NotNil().Required()
		gt.Bool(t, errors.Is(err, firestore.ErrNotFound) || errors.Is(err, memory.ErrNotFound)).True()
	})

	t.Run("Expired draft is treated as not found", func(t *testing.T) {
		repo := newRepo(t).CaseDraft()
		ctx := context.Background()

		// Create a draft with an already-past ExpiresAt.
		past := time.Now().UTC().Add(-2 * model.CaseDraftTTL)
		d := &model.CaseDraft{
			ID:        model.NewCaseDraftID(),
			CreatedBy: "U_old",
			CreatedAt: past,
			ExpiresAt: past.Add(time.Hour), // still in the past
		}
		gt.NoError(t, repo.Save(ctx, d)).Required()

		_, err := repo.Get(ctx, d.ID)
		gt.Value(t, err).NotNil().Required()
		gt.Bool(t, errors.Is(err, firestore.ErrNotFound) || errors.Is(err, memory.ErrNotFound)).True()
	})

	t.Run("Save with empty ID rejected", func(t *testing.T) {
		repo := newRepo(t).CaseDraft()
		ctx := context.Background()

		bad := &model.CaseDraft{CreatedBy: "U", CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour)}
		err := repo.Save(ctx, bad)
		gt.Value(t, err).NotNil().Required()
	})
}

func TestMemoryCaseDraftRepository(t *testing.T) {
	runCaseDraftRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestFirestoreCaseDraftRepository(t *testing.T) {
	runCaseDraftRepositoryTest(t, func(t *testing.T) interfaces.Repository {
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
		t.Cleanup(func() { gt.NoError(t, repo.Close()) })
		return repo
	})
}
