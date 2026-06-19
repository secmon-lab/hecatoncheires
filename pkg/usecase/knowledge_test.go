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

func TestKnowledgeUseCase_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	embed := &fakeEmbedClient{}
	uc := usecase.NewKnowledgeUseCase(repo, embed)
	ws := newWS()

	created, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{
		Title: "GitHub policy",
		Claim: "use github actions with sha pin",
		Tags:  []string{" ops ", "github", "ops", ""},
	})
	gt.NoError(t, err).Required()
	gt.Value(t, created.ID).NotEqual(model.KnowledgeID(""))

	// Tags normalized (trim/dedupe/drop-empty), order preserved.
	gt.Array(t, created.Tags).Length(2).Required()
	gt.Value(t, created.Tags[0]).Equal("ops")
	gt.Value(t, created.Tags[1]).Equal("github")

	// Embedding generated from title+claim ("github" present -> axis 0).
	gt.Array(t, created.Embedding).Length(model.EmbeddingDimension).Required()
	gt.Value(t, created.Embedding[0]).Equal(float64(1))
	gt.Number(t, embed.calls).Equal(1)

	// Persisted and readable back.
	got, err := uc.GetKnowledge(ctx, ws, created.ID)
	gt.NoError(t, err).Required()
	gt.String(t, got.Title).Equal("GitHub policy")
	gt.String(t, got.Claim).Equal("use github actions with sha pin")
}

func TestKnowledgeUseCase_CreateInputValidation(t *testing.T) {
	ctx := context.Background()
	uc := usecase.NewKnowledgeUseCase(memory.New(), nil)
	ws := newWS()

	t.Run("empty title", func(t *testing.T) {
		_, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "  ", Tags: []string{"ops"}})
		gt.Error(t, err).Is(usecase.ErrKnowledgeInput)
	})
	t.Run("no tags", func(t *testing.T) {
		_, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "x", Tags: []string{" ", ""}})
		gt.Error(t, err).Is(usecase.ErrKnowledgeInput)
	})
}

func TestKnowledgeUseCase_CreateFailOpenWithoutEmbedClient(t *testing.T) {
	ctx := context.Background()
	uc := usecase.NewKnowledgeUseCase(memory.New(), nil)
	ws := newWS()

	created, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{
		Title: "no embed",
		Claim: "body",
		Tags:  []string{"ops"},
	})
	gt.NoError(t, err).Required()
	gt.Array(t, created.Embedding).Length(0)
}

func TestKnowledgeUseCase_Update(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	embed := &fakeEmbedClient{}
	uc := usecase.NewKnowledgeUseCase(repo, embed)
	ws := newWS()

	created, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{
		Title: "npm policy", Claim: "min release age", Tags: []string{"npm"},
	})
	gt.NoError(t, err).Required()
	gt.Value(t, created.Embedding[2]).Equal(float64(1)) // npm axis

	newTitle := "github policy"
	newTags := []string{"github", "ops"}
	updated, err := uc.UpdateKnowledge(ctx, ws, usecase.UpdateKnowledgeInput{
		ID:    created.ID,
		Title: &newTitle,
		Tags:  &newTags,
	})
	gt.NoError(t, err).Required()
	gt.String(t, updated.Title).Equal("github policy")
	gt.Array(t, updated.Tags).Length(2).Required()
	// Title changed -> re-embedded; "github" now present (axis 0), npm gone.
	gt.Value(t, updated.Embedding[0]).Equal(float64(1))
	gt.Value(t, updated.Embedding[2]).Equal(float64(0))
	gt.Bool(t, updated.UpdatedAt.After(created.UpdatedAt) || updated.UpdatedAt.Equal(created.UpdatedAt)).True()
}

func TestKnowledgeUseCase_SearchSemanticRanking(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	embed := &fakeEmbedClient{}
	uc := usecase.NewKnowledgeUseCase(repo, embed)
	ws := newWS()

	k1, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "GitHub policy", Claim: "github actions", Tags: []string{"ops", "github"}})
	gt.NoError(t, err).Required()
	_, err = uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "Secret handling", Claim: "rotate secret tokens", Tags: []string{"ops", "secret"}})
	gt.NoError(t, err).Required()
	_, err = uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "npm policy", Claim: "min release age", Tags: []string{"npm"}})
	gt.NoError(t, err).Required()

	res, err := uc.SearchKnowledge(ctx, ws, usecase.SearchKnowledgeInput{Query: "github actions"})
	gt.NoError(t, err).Required()
	gt.Array(t, res).Length(3).Required()
	gt.Value(t, res[0].ID).Equal(k1.ID) // highest cosine

	// Tag pre-filter narrows the candidate set.
	resOps, err := uc.SearchKnowledge(ctx, ws, usecase.SearchKnowledgeInput{Query: "github", Tags: []string{"ops"}})
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
	uc := usecase.NewKnowledgeUseCase(memory.New(), nil) // no embed client
	ws := newWS()

	hit, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "GitHub policy", Claim: "body", Tags: []string{"ops"}})
	gt.NoError(t, err).Required()
	_, err = uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "unrelated", Claim: "nothing here", Tags: []string{"ops"}})
	gt.NoError(t, err).Required()

	res, err := uc.SearchKnowledge(ctx, ws, usecase.SearchKnowledgeInput{Query: "github"})
	gt.NoError(t, err).Required()
	gt.Array(t, res).Length(2).Required()
	gt.Value(t, res[0].ID).Equal(hit.ID) // substring match on title ranks first
}

func TestKnowledgeUseCase_ListTags(t *testing.T) {
	ctx := context.Background()
	uc := usecase.NewKnowledgeUseCase(memory.New(), nil)
	ws := newWS()

	_, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "a", Tags: []string{"ops", "github"}})
	gt.NoError(t, err).Required()
	_, err = uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "b", Tags: []string{"github", "security"}})
	gt.NoError(t, err).Required()

	tags, err := uc.ListTags(ctx, ws)
	gt.NoError(t, err).Required()
	// distinct + sorted
	gt.Array(t, tags).Length(3).Required()
	gt.Value(t, tags[0]).Equal("github")
	gt.Value(t, tags[1]).Equal("ops")
	gt.Value(t, tags[2]).Equal("security")
}

func TestKnowledgeUseCase_Delete(t *testing.T) {
	ctx := context.Background()
	uc := usecase.NewKnowledgeUseCase(memory.New(), nil)
	ws := newWS()

	created, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "x", Tags: []string{"ops"}})
	gt.NoError(t, err).Required()

	gt.NoError(t, uc.DeleteKnowledge(ctx, ws, created.ID)).Required()

	_, err = uc.GetKnowledge(ctx, ws, created.ID)
	gt.Error(t, err)

	// List reflects deletion.
	items, err := uc.ListKnowledge(ctx, ws, interfaces.KnowledgeListOptions{})
	gt.NoError(t, err).Required()
	gt.Array(t, items).Length(0)
}
