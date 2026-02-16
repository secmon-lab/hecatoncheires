package model_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestNewKnowledgeID(t *testing.T) {
	id := model.NewKnowledgeID()
	gt.String(t, string(id)).NotEqual("")

	// Verify it's a valid UUID format (36 characters with hyphens)
	gt.Value(t, len(id)).Equal(36)

	// Generate another ID and verify they are different
	id2 := model.NewKnowledgeID()
	gt.Value(t, id).NotEqual(id2)
}

func TestEmbeddingDimension(t *testing.T) {
	// Verify the embedding dimension matches Gemini text-embedding-004 spec
	gt.Value(t, model.EmbeddingDimension).Equal(768)
}

func TestKnowledge(t *testing.T) {
	k := &model.Knowledge{
		ID:         model.NewKnowledgeID(),
		CaseID:     123,
		SourceID:   model.NewSourceID(),
		SourceURLs: []string{"https://www.notion.so/page/12345"},
		Title:      "Security patch update",
		Summary:    "A new security patch was released for CVE-2024-1234",
		Embedding:  make([]float32, model.EmbeddingDimension),
	}

	gt.String(t, string(k.ID)).NotEqual("")
	gt.Value(t, k.CaseID).Equal(123)
	gt.Array(t, k.SourceURLs).Length(1)
	gt.Value(t, k.Title).Equal("Security patch update")
	gt.Value(t, k.Summary).Equal("A new security patch was released for CVE-2024-1234")
	gt.Array(t, k.Embedding).Length(model.EmbeddingDimension)
}
