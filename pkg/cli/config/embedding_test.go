package config_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
)

func TestEmbedding_Disabled(t *testing.T) {
	cfg := config.NewEmbeddingForTest("", "global", "")
	gt.Bool(t, cfg.IsEnabled()).False()

	_, err := cfg.NewClient(context.Background())
	gt.Error(t, err)
	gt.String(t, err.Error()).Contains("embedding-gemini-project-id")
}

func TestEmbedding_RequiresLocation(t *testing.T) {
	cfg := config.NewEmbeddingForTest("proj", "", "")

	_, err := cfg.NewClient(context.Background())
	gt.Error(t, err)
	gt.String(t, err.Error()).Contains("embedding-gemini-location")
}

func TestEmbedding_IsEnabled(t *testing.T) {
	gt.Bool(t, config.NewEmbeddingForTest("", "", "").IsEnabled()).False()
	gt.Bool(t, config.NewEmbeddingForTest("proj", "global", "").IsEnabled()).True()
}

func TestEmbedding_LogAttrs_DefaultModel(t *testing.T) {
	cfg := config.NewEmbeddingForTest("proj", "us-central1", "")
	attrs := cfg.LogAttrs()

	hasProject, hasLocation, hasModel := false, false, false
	for _, a := range attrs {
		switch a.Key {
		case "gcp_project_id":
			gt.String(t, a.Value.String()).Equal("proj")
			hasProject = true
		case "gcp_location":
			gt.String(t, a.Value.String()).Equal("us-central1")
			hasLocation = true
		case "model":
			gt.String(t, a.Value.String()).Equal(config.DefaultEmbeddingModel)
			hasModel = true
		}
	}
	gt.Bool(t, hasProject).True()
	gt.Bool(t, hasLocation).True()
	gt.Bool(t, hasModel).True()
}

func TestEmbedding_LogAttrs_OverrideModel(t *testing.T) {
	cfg := config.NewEmbeddingForTest("proj", "global", "text-embedding-004")
	attrs := cfg.LogAttrs()

	for _, a := range attrs {
		if a.Key == "model" {
			gt.String(t, a.Value.String()).Equal("text-embedding-004")
			return
		}
	}
	gt.Bool(t, false).True()
}

func TestEmbedding_DefaultModelConstant(t *testing.T) {
	gt.String(t, config.DefaultEmbeddingModel).Equal("gemini-embedding-2")
}
