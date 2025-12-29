package model_test

import (
	"testing"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

func TestResponse(t *testing.T) {
	now := time.Now()
	response := &model.Response{
		ID:           1,
		Title:        "Test Response",
		Description:  "Test Description",
		ResponderIDs: []string{"U12345", "U67890"},
		URL:          "https://example.com",
		Status:       types.ResponseStatusTodo,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if response.ID != 1 {
		t.Errorf("Response.ID = %v, want 1", response.ID)
	}

	if response.Title != "Test Response" {
		t.Errorf("Response.Title = %v, want Test Response", response.Title)
	}

	if response.Description != "Test Description" {
		t.Errorf("Response.Description = %v, want Test Description", response.Description)
	}

	if len(response.ResponderIDs) != 2 {
		t.Errorf("len(Response.ResponderIDs) = %v, want 2", len(response.ResponderIDs))
	}

	if response.URL != "https://example.com" {
		t.Errorf("Response.URL = %v, want https://example.com", response.URL)
	}

	if response.Status != types.ResponseStatusTodo {
		t.Errorf("Response.Status = %v, want %v", response.Status, types.ResponseStatusTodo)
	}

	if response.CreatedAt != now {
		t.Errorf("Response.CreatedAt = %v, want %v", response.CreatedAt, now)
	}

	if response.UpdatedAt != now {
		t.Errorf("Response.UpdatedAt = %v, want %v", response.UpdatedAt, now)
	}
}

func TestResponse_EmptyResponderIDs(t *testing.T) {
	response := &model.Response{
		ResponderIDs: []string{},
	}

	if response.ResponderIDs == nil {
		t.Error("Response.ResponderIDs should not be nil")
	}

	if len(response.ResponderIDs) != 0 {
		t.Errorf("len(Response.ResponderIDs) = %v, want 0", len(response.ResponderIDs))
	}
}

func TestResponse_OptionalFields(t *testing.T) {
	response := &model.Response{
		ID:          1,
		Title:       "Minimal Response",
		Description: "",
		URL:         "",
		Status:      types.ResponseStatusBacklog,
	}

	if response.Description != "" {
		t.Errorf("Response.Description = %v, want empty string", response.Description)
	}

	if response.URL != "" {
		t.Errorf("Response.URL = %v, want empty string", response.URL)
	}
}
