package usecase_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestTagUseCase_CreateTag(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewTagUseCase(repo)
	ws := newWS()

	tag, err := uc.CreateTag(ctx, ws, "ops")
	gt.NoError(t, err).Required()

	// ID assigned, non-empty.
	gt.Value(t, tag.ID).NotEqual(model.TagID(""))
	gt.Value(t, tag.WorkspaceID).Equal(ws)
	gt.String(t, tag.Name).Equal("ops")
	gt.Bool(t, !tag.CreatedAt.IsZero()).True()
	gt.Bool(t, !tag.UpdatedAt.IsZero()).True()

	// Persisted and readable via repo.
	got, err := repo.Tag().Get(ctx, ws, tag.ID)
	gt.NoError(t, err).Required()
	gt.Value(t, got.ID).Equal(tag.ID)
	gt.String(t, got.Name).Equal("ops")
}

func TestTagUseCase_CreateTagTrimsName(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewTagUseCase(repo)
	ws := newWS()

	tag, err := uc.CreateTag(ctx, ws, "  ops  ")
	gt.NoError(t, err).Required()
	gt.String(t, tag.Name).Equal("ops")
}

func TestTagUseCase_CreateTagEmptyName(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewTagUseCase(repo)
	ws := newWS()

	// An empty name is valid; Name is optional.
	tag, err := uc.CreateTag(ctx, ws, "")
	gt.NoError(t, err).Required()
	gt.String(t, tag.Name).Equal("")
	gt.Value(t, tag.ID).NotEqual(model.TagID(""))
}

func TestTagUseCase_GetTag(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewTagUseCase(repo)
	ws := newWS()

	created, err := uc.CreateTag(ctx, ws, "github")
	gt.NoError(t, err).Required()

	got, err := uc.GetTag(ctx, ws, created.ID)
	gt.NoError(t, err).Required()
	gt.Value(t, got.ID).Equal(created.ID)
	gt.String(t, got.Name).Equal("github")
	gt.Value(t, got.WorkspaceID).Equal(ws)
}

func TestTagUseCase_ListTags(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewTagUseCase(repo)
	ws := newWS()

	a, err := uc.CreateTag(ctx, ws, "aaa")
	gt.NoError(t, err).Required()
	b, err := uc.CreateTag(ctx, ws, "bbb")
	gt.NoError(t, err).Required()
	c, err := uc.CreateTag(ctx, ws, "ccc")
	gt.NoError(t, err).Required()

	tags, err := uc.ListTags(ctx, ws)
	gt.NoError(t, err).Required()
	gt.Array(t, tags).Length(3).Required()

	// Sorted by CreatedAt ascending (creation order preserved because each
	// call stamps a fresh time.Now()).
	gt.Value(t, tags[0].ID).Equal(a.ID)
	gt.Value(t, tags[1].ID).Equal(b.ID)
	gt.Value(t, tags[2].ID).Equal(c.ID)
}

func TestTagUseCase_UpdateTag(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewTagUseCase(repo)
	ws := newWS()

	created, err := uc.CreateTag(ctx, ws, "before")
	gt.NoError(t, err).Required()

	updated, err := uc.UpdateTag(ctx, ws, created.ID, "after")
	gt.NoError(t, err).Required()
	gt.Value(t, updated.ID).Equal(created.ID) // ID immutable
	gt.String(t, updated.Name).Equal("after")
	gt.Bool(t, !updated.UpdatedAt.Before(created.UpdatedAt)).True() // UpdatedAt advanced

	// Read back confirms persistence.
	got, err := uc.GetTag(ctx, ws, created.ID)
	gt.NoError(t, err).Required()
	gt.String(t, got.Name).Equal("after")
}

func TestTagUseCase_DeleteUnusedTag(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewTagUseCase(repo)
	ws := newWS()

	tag, err := uc.CreateTag(ctx, ws, "unused")
	gt.NoError(t, err).Required()

	gt.NoError(t, uc.DeleteTag(ctx, ws, tag.ID)).Required()

	// Tag is gone from repo.
	tags, err := uc.ListTags(ctx, ws)
	gt.NoError(t, err).Required()
	gt.Array(t, tags).Length(0)
}

func TestTagUseCase_DeleteTagInUseReturnsErrTagInUse(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	tagUC := usecase.NewTagUseCase(repo)
	knowledgeUC := usecase.NewKnowledgeUseCase(repo, nil)
	ws := newWS()

	// Create a tag and a knowledge entry that references it.
	tag, err := tagUC.CreateTag(ctx, ws, "in-use")
	gt.NoError(t, err).Required()

	_, err = knowledgeUC.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{
		Title:  "knowledge referencing tag",
		Claim:  "body",
		TagIDs: []model.TagID{tag.ID},
	})
	gt.NoError(t, err).Required()

	// Attempt to delete the tag; must fail with ErrTagInUse.
	err = tagUC.DeleteTag(ctx, ws, tag.ID)
	gt.Error(t, err).Is(usecase.ErrTagInUse)
}

func TestTagUseCase_IDs_AreUnique(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewTagUseCase(repo)
	ws := newWS()

	a, err := uc.CreateTag(ctx, ws, "a")
	gt.NoError(t, err).Required()
	b, err := uc.CreateTag(ctx, ws, "b")
	gt.NoError(t, err).Required()

	gt.Value(t, a.ID).NotEqual(b.ID)
}
