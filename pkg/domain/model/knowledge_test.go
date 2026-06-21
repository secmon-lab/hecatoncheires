package model_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestNewKnowledgeID(t *testing.T) {
	a := model.NewKnowledgeID()
	b := model.NewKnowledgeID()
	gt.Value(t, a).NotEqual(model.KnowledgeID(""))
	gt.Value(t, a).NotEqual(b)
	gt.String(t, a.String()).Equal(string(a))
}

func TestKnowledgeValidate(t *testing.T) {
	now := time.Now()
	valid := func() *model.Knowledge {
		return &model.Knowledge{
			ID:          model.NewKnowledgeID(),
			WorkspaceID: "ws-1",
			Title:       "knowledge title",
			Claim:       "## heading\n\n- a claim body in markdown",
			Tags:        []string{"ops", "github"},
			CreatedAt:   now,
			UpdatedAt:   now,
		}
	}

	t.Run("valid", func(t *testing.T) {
		gt.NoError(t, valid().Validate())
	})

	t.Run("valid with embedding of correct dimension", func(t *testing.T) {
		k := valid()
		k.Embedding = make([]float64, model.EmbeddingDimension)
		gt.NoError(t, k.Validate())
	})

	t.Run("nil receiver", func(t *testing.T) {
		var k *model.Knowledge
		gt.Error(t, k.Validate()).Is(model.ErrKnowledgeValidation)
	})

	cases := []struct {
		name   string
		mutate func(k *model.Knowledge)
	}{
		{"empty ID", func(k *model.Knowledge) { k.ID = "" }},
		{"empty workspace", func(k *model.Knowledge) { k.WorkspaceID = "" }},
		{"empty title", func(k *model.Knowledge) { k.Title = "" }},
		{"no tags", func(k *model.Knowledge) { k.Tags = nil }},
		{"empty tags slice", func(k *model.Knowledge) { k.Tags = []string{} }},
		{"empty tag string", func(k *model.Knowledge) { k.Tags = []string{"ok", ""} }},
		{"tag too long", func(k *model.Knowledge) { k.Tags = []string{strings.Repeat("x", model.MaxTagLength+1)} }},
		{"too many tags", func(k *model.Knowledge) {
			tags := make([]string, model.MaxTags+1)
			for i := range tags {
				tags[i] = "tag" + strconv.Itoa(i)
			}
			k.Tags = tags
		}},
		{"claim too long", func(k *model.Knowledge) { k.Claim = strings.Repeat("あ", model.MaxClaimLength+1) }},
		{"wrong embedding dimension", func(k *model.Knowledge) { k.Embedding = []float64{0.1, 0.2} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k := valid()
			tc.mutate(k)
			gt.Error(t, k.Validate()).Is(model.ErrKnowledgeValidation)
		})
	}

	t.Run("claim at max length is allowed", func(t *testing.T) {
		k := valid()
		k.Claim = strings.Repeat("あ", model.MaxClaimLength)
		gt.NoError(t, k.Validate())
	})
}

func TestNormalizeTags(t *testing.T) {
	t.Run("trims, drops empty, dedupes, preserves order", func(t *testing.T) {
		got := model.NormalizeTags([]string{" ops ", "github", "", "ops", "  ", "security"})
		gt.Array(t, got).Length(3).Required()
		gt.Value(t, got[0]).Equal("ops")
		gt.Value(t, got[1]).Equal("github")
		gt.Value(t, got[2]).Equal("security")
	})

	t.Run("case sensitive dedupe", func(t *testing.T) {
		got := model.NormalizeTags([]string{"Ops", "ops"})
		gt.Array(t, got).Length(2)
	})

	t.Run("nil input yields empty", func(t *testing.T) {
		got := model.NormalizeTags(nil)
		gt.Array(t, got).Length(0)
	})
}
