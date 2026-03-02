package model_test

import (
	"testing"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

func TestIsCaseAccessible(t *testing.T) {
	tests := []struct {
		name     string
		c        *model.Case
		userID   string
		expected bool
	}{
		{
			name:     "public case is always accessible",
			c:        &model.Case{IsPrivate: false},
			userID:   "U001",
			expected: true,
		},
		{
			name: "private case accessible to channel member",
			c: &model.Case{
				IsPrivate:      true,
				ChannelUserIDs: []string{"U001", "U002", "U003"},
			},
			userID:   "U002",
			expected: true,
		},
		{
			name: "private case not accessible to non-member",
			c: &model.Case{
				IsPrivate:      true,
				ChannelUserIDs: []string{"U001", "U002"},
			},
			userID:   "U999",
			expected: false,
		},
		{
			name: "private case with empty members is not accessible",
			c: &model.Case{
				IsPrivate:      true,
				ChannelUserIDs: []string{},
			},
			userID:   "U001",
			expected: false,
		},
		{
			name: "private case with nil members is not accessible",
			c: &model.Case{
				IsPrivate:      true,
				ChannelUserIDs: nil,
			},
			userID:   "U001",
			expected: false,
		},
		{
			name:     "public case with empty userID is accessible",
			c:        &model.Case{IsPrivate: false},
			userID:   "",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := model.IsCaseAccessible(tt.c, tt.userID)
			if result != tt.expected {
				t.Errorf("IsCaseAccessible() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestRestrictCase(t *testing.T) {
	now := time.Now()
	original := &model.Case{
		ID:             42,
		Title:          "Sensitive Title",
		Description:    "Secret description",
		Status:         types.CaseStatusOpen,
		AssigneeIDs:    []string{"U001"},
		SlackChannelID: "C001",
		IsPrivate:      true,
		ChannelUserIDs: []string{"U001", "U002"},
		FieldValues:    map[string]model.FieldValue{"f1": {FieldID: "f1"}},
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	restricted := model.RestrictCase(original)

	// Preserved fields
	if restricted.ID != 42 {
		t.Errorf("ID = %d, want 42", restricted.ID)
	}
	if restricted.Status != types.CaseStatusOpen {
		t.Errorf("Status = %v, want %v", restricted.Status, types.CaseStatusOpen)
	}
	if !restricted.IsPrivate {
		t.Error("IsPrivate should be true")
	}
	if !restricted.AccessDenied {
		t.Error("AccessDenied should be true")
	}
	if !restricted.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", restricted.CreatedAt, now)
	}
	if !restricted.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", restricted.UpdatedAt, now)
	}

	// Cleared fields
	if restricted.Title != "" {
		t.Errorf("Title = %q, want empty", restricted.Title)
	}
	if restricted.Description != "" {
		t.Errorf("Description = %q, want empty", restricted.Description)
	}
	if len(restricted.AssigneeIDs) != 0 {
		t.Errorf("AssigneeIDs = %v, want empty", restricted.AssigneeIDs)
	}
	if restricted.SlackChannelID != "" {
		t.Errorf("SlackChannelID = %q, want empty", restricted.SlackChannelID)
	}
	if len(restricted.ChannelUserIDs) != 0 {
		t.Errorf("ChannelUserIDs = %v, want empty", restricted.ChannelUserIDs)
	}
	if len(restricted.FieldValues) != 0 {
		t.Errorf("FieldValues = %v, want empty", restricted.FieldValues)
	}

	// Original should be unchanged
	if original.Title != "Sensitive Title" {
		t.Error("Original case should not be modified")
	}
}
