package model_test

import (
	"errors"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
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

func TestCase_IsDraft(t *testing.T) {
	gt.Bool(t, (&model.Case{Status: types.CaseStatusDraft}).IsDraft()).True()
	gt.Bool(t, (&model.Case{Status: types.CaseStatusOpen}).IsDraft()).False()
	gt.Bool(t, (&model.Case{Status: types.CaseStatusClosed}).IsDraft()).False()
	gt.Bool(t, (&model.Case{Status: types.CaseStatus("")}).IsDraft()).False()

	// Nil receiver is safe and reports false; this guards against accidental
	// nil dereference in higher layers that route Case lookups through
	// IsDraft as a guard.
	var nilCase *model.Case
	gt.Bool(t, nilCase.IsDraft()).False()
}

func TestCase_SubmitDraft(t *testing.T) {
	t.Run("draft transitions to open", func(t *testing.T) {
		c := &model.Case{Status: types.CaseStatusDraft}
		gt.NoError(t, c.SubmitDraft()).Required()
		gt.Value(t, c.Status).Equal(types.CaseStatusOpen)
	})

	t.Run("open case cannot be re-submitted", func(t *testing.T) {
		c := &model.Case{Status: types.CaseStatusOpen}
		err := c.SubmitDraft()
		gt.Error(t, err)
		gt.Bool(t, errors.Is(err, model.ErrCaseNotDraft)).True()
		// Status must stay unchanged.
		gt.Value(t, c.Status).Equal(types.CaseStatusOpen)
	})

	t.Run("closed case cannot be submitted", func(t *testing.T) {
		c := &model.Case{Status: types.CaseStatusClosed}
		err := c.SubmitDraft()
		gt.Error(t, err)
		gt.Bool(t, errors.Is(err, model.ErrCaseNotDraft)).True()
		gt.Value(t, c.Status).Equal(types.CaseStatusClosed)
	})

	t.Run("nil receiver returns error", func(t *testing.T) {
		var nilCase *model.Case
		err := nilCase.SubmitDraft()
		gt.Error(t, err)
		gt.Bool(t, errors.Is(err, model.ErrCaseNotDraft)).True()
	})
}
