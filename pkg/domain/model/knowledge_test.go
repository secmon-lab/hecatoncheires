package model_test

import (
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

	tagID1 := model.NewTagID()
	tagID2 := model.NewTagID()

	valid := func() *model.Knowledge {
		return &model.Knowledge{
			ID:          model.NewKnowledgeID(),
			WorkspaceID: "ws-1",
			Title:       "knowledge title",
			Claim:       "## heading\n\n- a claim body in markdown",
			TagIDs:      []model.TagID{tagID1, tagID2},
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
		{"no tags", func(k *model.Knowledge) { k.TagIDs = nil }},
		{"empty tags slice", func(k *model.Knowledge) { k.TagIDs = []model.TagID{} }},
		{"empty tag id in slice", func(k *model.Knowledge) { k.TagIDs = []model.TagID{tagID1, ""} }},
		{"too many tags", func(k *model.Knowledge) {
			tags := make([]model.TagID, model.MaxTags+1)
			for i := range tags {
				tags[i] = model.NewTagID()
			}
			k.TagIDs = tags
		}},
		{"claim too long", func(k *model.Knowledge) { k.Claim = strings.Repeat("a", model.MaxClaimLength+1) }},
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
		k.Claim = strings.Repeat("a", model.MaxClaimLength)
		gt.NoError(t, k.Validate())
	})

	t.Run("exactly max tags is valid", func(t *testing.T) {
		k := valid()
		tags := make([]model.TagID, model.MaxTags)
		for i := range tags {
			tags[i] = model.NewTagID()
		}
		k.TagIDs = tags
		gt.NoError(t, k.Validate())
	})
}

func TestNormalizeTagIDs(t *testing.T) {
	id1 := model.NewTagID()
	id2 := model.NewTagID()
	id3 := model.NewTagID()

	t.Run("drops empty, dedupes, preserves order", func(t *testing.T) {
		got := model.NormalizeTagIDs([]model.TagID{id1, id2, "", id1, id3})
		gt.Array(t, got).Length(3).Required()
		gt.Value(t, got[0]).Equal(id1)
		gt.Value(t, got[1]).Equal(id2)
		gt.Value(t, got[2]).Equal(id3)
	})

	t.Run("nil input yields empty", func(t *testing.T) {
		got := model.NormalizeTagIDs(nil)
		gt.Array(t, got).Length(0)
	})

	t.Run("all empty yields empty", func(t *testing.T) {
		got := model.NormalizeTagIDs([]model.TagID{"", ""})
		gt.Array(t, got).Length(0)
	})

	t.Run("single unique id preserved", func(t *testing.T) {
		got := model.NormalizeTagIDs([]model.TagID{id1})
		gt.Array(t, got).Length(1).Required()
		gt.Value(t, got[0]).Equal(id1)
	})

	t.Run("duplicate removal preserves first occurrence", func(t *testing.T) {
		got := model.NormalizeTagIDs([]model.TagID{id2, id1, id2})
		gt.Array(t, got).Length(2).Required()
		gt.Value(t, got[0]).Equal(id2)
		gt.Value(t, got[1]).Equal(id1)
	})
}
