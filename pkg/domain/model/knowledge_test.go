package model_test

import (
	"testing"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestNewKnowledgeID(t *testing.T) {
	id := model.NewKnowledgeID()
	if id == "" {
		t.Error("NewKnowledgeID() returned empty string")
	}

	// Verify it's a valid UUID format (36 characters with hyphens)
	if len(id) != 36 {
		t.Errorf("Expected UUID length 36, got %d", len(id))
	}

	// Generate another ID and verify they are different
	id2 := model.NewKnowledgeID()
	if id == id2 {
		t.Error("Two generated IDs should be different")
	}
}

func TestEmbeddingDimension(t *testing.T) {
	// Verify the embedding dimension matches Gemini text-embedding-004 spec
	if model.EmbeddingDimension != 768 {
		t.Errorf("Expected EmbeddingDimension to be 768, got %d", model.EmbeddingDimension)
	}
}

func TestKnowledge(t *testing.T) {
	k := &model.Knowledge{
		ID:        model.NewKnowledgeID(),
		RiskID:    123,
		SourceID:  model.NewSourceID(),
		SourceURL: "https://www.notion.so/page/12345",
		Title:     "Security patch update",
		Summary:   "A new security patch was released for CVE-2024-1234",
		Embedding: make([]float32, model.EmbeddingDimension),
	}

	if k.ID == "" {
		t.Error("Knowledge ID should not be empty")
	}
	if k.RiskID != 123 {
		t.Errorf("Expected RiskID 123, got %d", k.RiskID)
	}
	if k.SourceURL != "https://www.notion.so/page/12345" {
		t.Errorf("Expected SourceURL mismatch, got %s", k.SourceURL)
	}
	if k.Title != "Security patch update" {
		t.Errorf("Expected Title mismatch, got %s", k.Title)
	}
	if k.Summary != "A new security patch was released for CVE-2024-1234" {
		t.Errorf("Expected Summary mismatch, got %s", k.Summary)
	}
	if len(k.Embedding) != model.EmbeddingDimension {
		t.Errorf("Expected Embedding dimension %d, got %d", model.EmbeddingDimension, len(k.Embedding))
	}
}
