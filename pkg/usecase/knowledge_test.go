package usecase_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// fakeEmbedClient produces a deterministic 3-axis embedding: index 0 = "github",
// 1 = "secret", 2 = "npm". This makes cosine ranking testable without a real
// embedding model.
type fakeEmbedClient struct{ calls int }

func (f *fakeEmbedClient) GenerateEmbedding(ctx context.Context, dimension int, input []string) ([][]float64, error) {
	f.calls++
	out := make([][]float64, len(input))
	for i, s := range input {
		v := make([]float64, dimension)
		ls := strings.ToLower(s)
		if strings.Contains(ls, "github") {
			v[0] = 1
		}
		if strings.Contains(ls, "secret") {
			v[1] = 1
		}
		if strings.Contains(ls, "npm") {
			v[2] = 1
		}
		out[i] = v
	}
	return out, nil
}

func newWS() string { return fmt.Sprintf("ws-%d", time.Now().UnixNano()) }

// createTestTag is a helper to create a tag via TagUseCase and return its ID.
func createTestTag(t *testing.T, ctx context.Context, tagUC *usecase.TagUseCase, ws, name string) model.TagID {
	t.Helper()
	tag, err := tagUC.CreateTag(ctx, ws, name)
	gt.NoError(t, err).Required()
	return tag.ID
}

func TestKnowledgeUseCase_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	embed := &fakeEmbedClient{}
	uc := usecase.NewKnowledgeUseCase(repo, embed)
	tagUC := usecase.NewTagUseCase(repo)
	ws := newWS()

	opsID := createTestTag(t, ctx, tagUC, ws, "ops")
	githubID := createTestTag(t, ctx, tagUC, ws, "github")

	created, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{
		Title:  "GitHub policy",
		Claim:  "use github actions with sha pin",
		TagIDs: []model.TagID{opsID, githubID, opsID, ""}, // opsID duped, empty dropped
	})
	gt.NoError(t, err).Required()
	gt.Value(t, created.ID).NotEqual(model.KnowledgeID(""))

	// TagIDs normalized (dedupe/drop-empty), order preserved.
	gt.Array(t, created.TagIDs).Length(2).Required()
	gt.Value(t, created.TagIDs[0]).Equal(opsID)
	gt.Value(t, created.TagIDs[1]).Equal(githubID)

	// Embedding generated from title+claim ("github" present -> axis 0).
	gt.Array(t, created.Embedding).Length(model.EmbeddingDimension).Required()
	gt.Value(t, created.Embedding[0]).Equal(float64(1))
	gt.Number(t, embed.calls).Equal(1)

	// Persisted and readable back.
	got, err := uc.GetKnowledge(ctx, ws, created.ID)
	gt.NoError(t, err).Required()
	gt.String(t, got.Title).Equal("GitHub policy")
	gt.String(t, got.Claim).Equal("use github actions with sha pin")
	gt.Array(t, got.TagIDs).Length(2).Required()
	gt.Value(t, got.TagIDs[0]).Equal(opsID)
	gt.Value(t, got.TagIDs[1]).Equal(githubID)
}

func TestKnowledgeUseCase_CreateInputValidation(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewKnowledgeUseCase(repo, nil)
	tagUC := usecase.NewTagUseCase(repo)
	ws := newWS()

	opsID := createTestTag(t, ctx, tagUC, ws, "ops")

	t.Run("empty title", func(t *testing.T) {
		_, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "  ", TagIDs: []model.TagID{opsID}})
		gt.Error(t, err).Is(usecase.ErrKnowledgeInput)
	})
	t.Run("no tags", func(t *testing.T) {
		_, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "x", TagIDs: []model.TagID{"", ""}})
		gt.Error(t, err).Is(usecase.ErrKnowledgeInput)
	})
}

func TestKnowledgeUseCase_CreateUnknownTagFails(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewKnowledgeUseCase(repo, nil)
	ws := newWS()

	// Pass a TagID that was never created in the workspace.
	nonExistent := model.NewTagID()
	_, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{
		Title:  "some title",
		Claim:  "some body",
		TagIDs: []model.TagID{nonExistent},
	})
	gt.Error(t, err).Is(usecase.ErrUnknownTag)
}

func TestKnowledgeUseCase_CreateFailOpenWithoutEmbedClient(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewKnowledgeUseCase(repo, nil)
	tagUC := usecase.NewTagUseCase(repo)
	ws := newWS()

	opsID := createTestTag(t, ctx, tagUC, ws, "ops")

	created, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{
		Title:  "no embed",
		Claim:  "body",
		TagIDs: []model.TagID{opsID},
	})
	gt.NoError(t, err).Required()
	gt.Array(t, created.Embedding).Length(0)
}

func TestKnowledgeUseCase_Update(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	embed := &fakeEmbedClient{}
	uc := usecase.NewKnowledgeUseCase(repo, embed)
	tagUC := usecase.NewTagUseCase(repo)
	ws := newWS()

	npmID := createTestTag(t, ctx, tagUC, ws, "npm")
	githubID := createTestTag(t, ctx, tagUC, ws, "github")
	opsID := createTestTag(t, ctx, tagUC, ws, "ops")

	created, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{
		Title:  "npm policy",
		Claim:  "min release age",
		TagIDs: []model.TagID{npmID},
	})
	gt.NoError(t, err).Required()
	gt.Value(t, created.Embedding[2]).Equal(float64(1)) // npm axis

	newTitle := "github policy"
	newTagIDs := []model.TagID{githubID, opsID}
	updated, err := uc.UpdateKnowledge(ctx, ws, usecase.UpdateKnowledgeInput{
		ID:     created.ID,
		Title:  &newTitle,
		TagIDs: &newTagIDs,
	})
	gt.NoError(t, err).Required()
	gt.String(t, updated.Title).Equal("github policy")
	gt.Array(t, updated.TagIDs).Length(2).Required()
	gt.Value(t, updated.TagIDs[0]).Equal(githubID)
	gt.Value(t, updated.TagIDs[1]).Equal(opsID)
	// Title changed -> re-embedded; "github" now present (axis 0), npm gone.
	gt.Value(t, updated.Embedding[0]).Equal(float64(1))
	gt.Value(t, updated.Embedding[2]).Equal(float64(0))
	gt.Bool(t, updated.UpdatedAt.After(created.UpdatedAt) || updated.UpdatedAt.Equal(created.UpdatedAt)).True()
}

func TestKnowledgeUseCase_UpdateUnknownTagFails(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewKnowledgeUseCase(repo, nil)
	tagUC := usecase.NewTagUseCase(repo)
	ws := newWS()

	opsID := createTestTag(t, ctx, tagUC, ws, "ops")

	created, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{
		Title:  "some knowledge",
		Claim:  "body",
		TagIDs: []model.TagID{opsID},
	})
	gt.NoError(t, err).Required()

	nonExistent := model.NewTagID()
	newTagIDs := []model.TagID{nonExistent}
	_, err = uc.UpdateKnowledge(ctx, ws, usecase.UpdateKnowledgeInput{
		ID:     created.ID,
		TagIDs: &newTagIDs,
	})
	gt.Error(t, err).Is(usecase.ErrUnknownTag)
}

func TestKnowledgeUseCase_SearchSemanticRanking(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	embed := &fakeEmbedClient{}
	uc := usecase.NewKnowledgeUseCase(repo, embed)
	tagUC := usecase.NewTagUseCase(repo)
	ws := newWS()

	opsID := createTestTag(t, ctx, tagUC, ws, "ops")
	githubID := createTestTag(t, ctx, tagUC, ws, "github")
	secretID := createTestTag(t, ctx, tagUC, ws, "secret")
	npmID := createTestTag(t, ctx, tagUC, ws, "npm")

	k1, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "GitHub policy", Claim: "github actions", TagIDs: []model.TagID{opsID, githubID}})
	gt.NoError(t, err).Required()
	_, err = uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "Secret handling", Claim: "rotate secret tokens", TagIDs: []model.TagID{opsID, secretID}})
	gt.NoError(t, err).Required()
	_, err = uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "npm policy", Claim: "min release age", TagIDs: []model.TagID{npmID}})
	gt.NoError(t, err).Required()

	res, err := uc.SearchKnowledge(ctx, ws, usecase.SearchKnowledgeInput{Query: "github actions"})
	gt.NoError(t, err).Required()
	gt.Array(t, res).Length(3).Required()
	gt.Value(t, res[0].ID).Equal(k1.ID) // highest cosine

	// Tag pre-filter narrows the candidate set.
	resOps, err := uc.SearchKnowledge(ctx, ws, usecase.SearchKnowledgeInput{Query: "github", TagIDs: []model.TagID{opsID}})
	gt.NoError(t, err).Required()
	gt.Array(t, resOps).Length(2).Required()
	gt.Value(t, resOps[0].ID).Equal(k1.ID)

	// Limit caps results.
	resLimited, err := uc.SearchKnowledge(ctx, ws, usecase.SearchKnowledgeInput{Query: "github", Limit: 1})
	gt.NoError(t, err).Required()
	gt.Array(t, resLimited).Length(1)
}

func TestKnowledgeUseCase_SearchSubstringFallback(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewKnowledgeUseCase(repo, nil) // no embed client
	tagUC := usecase.NewTagUseCase(repo)
	ws := newWS()

	opsID := createTestTag(t, ctx, tagUC, ws, "ops")

	hit, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "GitHub policy", Claim: "body", TagIDs: []model.TagID{opsID}})
	gt.NoError(t, err).Required()
	_, err = uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "unrelated", Claim: "nothing here", TagIDs: []model.TagID{opsID}})
	gt.NoError(t, err).Required()

	res, err := uc.SearchKnowledge(ctx, ws, usecase.SearchKnowledgeInput{Query: "github"})
	gt.NoError(t, err).Required()
	gt.Array(t, res).Length(2).Required()
	gt.Value(t, res[0].ID).Equal(hit.ID) // substring match on title ranks first
}

func TestKnowledgeUseCase_Delete(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewKnowledgeUseCase(repo, nil)
	tagUC := usecase.NewTagUseCase(repo)
	ws := newWS()

	opsID := createTestTag(t, ctx, tagUC, ws, "ops")

	created, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "x", TagIDs: []model.TagID{opsID}})
	gt.NoError(t, err).Required()

	gt.NoError(t, uc.DeleteKnowledge(ctx, ws, created.ID)).Required()

	_, err = uc.GetKnowledge(ctx, ws, created.ID)
	gt.Error(t, err)

	// List reflects deletion.
	items, err := uc.ListKnowledge(ctx, ws, interfaces.KnowledgeListOptions{})
	gt.NoError(t, err).Required()
	gt.Array(t, items).Length(0)
}
