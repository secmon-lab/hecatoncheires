package types_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

func TestActionEventKind_IsValid(t *testing.T) {
	tests := []struct {
		name string
		kind types.ActionEventKind
		want bool
	}{
		{name: "created", kind: types.ActionEventCreated, want: true},
		{name: "title-changed", kind: types.ActionEventTitleChanged, want: true},
		{name: "status-changed", kind: types.ActionEventStatusChanged, want: true},
		{name: "assignee-changed", kind: types.ActionEventAssigneeChanged, want: true},
		{name: "archived", kind: types.ActionEventArchived, want: true},
		{name: "unarchived", kind: types.ActionEventUnarchived, want: true},
		{name: "unknown", kind: types.ActionEventKind("UNKNOWN"), want: false},
		{name: "empty", kind: types.ActionEventKind(""), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gt.Value(t, tt.kind.IsValid()).Equal(tt.want)
		})
	}
}

func TestActionEventKind_String(t *testing.T) {
	gt.Value(t, types.ActionEventArchived.String()).Equal("ARCHIVED")
	gt.Value(t, types.ActionEventUnarchived.String()).Equal("UNARCHIVED")
}
