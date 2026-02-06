package types_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

func TestActionStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status types.ActionStatus
		want   bool
	}{
		{
			name:   "valid backlog",
			status: types.ActionStatusBacklog,
			want:   true,
		},
		{
			name:   "valid todo",
			status: types.ActionStatusTodo,
			want:   true,
		},
		{
			name:   "valid in-progress",
			status: types.ActionStatusInProgress,
			want:   true,
		},
		{
			name:   "valid blocked",
			status: types.ActionStatusBlocked,
			want:   true,
		},
		{
			name:   "valid completed",
			status: types.ActionStatusCompleted,
			want:   true,
		},
		{
			name:   "valid abandoned",
			status: types.ActionStatusAbandoned,
			want:   true,
		},
		{
			name:   "invalid status",
			status: types.ActionStatus("invalid"),
			want:   false,
		},
		{
			name:   "empty status",
			status: types.ActionStatus(""),
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.want {
				gt.B(t, tt.status.IsValid()).True()
			} else {
				gt.B(t, tt.status.IsValid()).False()
			}
		})
	}
}

func TestParseActionStatus(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    types.ActionStatus
		wantErr bool
	}{
		{
			name:    "valid backlog",
			input:   "BACKLOG",
			want:    types.ActionStatusBacklog,
			wantErr: false,
		},
		{
			name:    "valid todo",
			input:   "TODO",
			want:    types.ActionStatusTodo,
			wantErr: false,
		},
		{
			name:    "valid in-progress",
			input:   "IN_PROGRESS",
			want:    types.ActionStatusInProgress,
			wantErr: false,
		},
		{
			name:    "valid blocked",
			input:   "BLOCKED",
			want:    types.ActionStatusBlocked,
			wantErr: false,
		},
		{
			name:    "valid completed",
			input:   "COMPLETED",
			want:    types.ActionStatusCompleted,
			wantErr: false,
		},
		{
			name:    "valid abandoned",
			input:   "ABANDONED",
			want:    types.ActionStatusAbandoned,
			wantErr: false,
		},
		{
			name:    "invalid status",
			input:   "invalid",
			want:    "",
			wantErr: true,
		},
		{
			name:    "empty status",
			input:   "",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := types.ParseActionStatus(tt.input)
			if tt.wantErr {
				gt.Error(t, err)
			} else {
				gt.NoError(t, err)
				gt.V(t, got).Equal(tt.want)
			}
		})
	}
}

func TestAllActionStatuses(t *testing.T) {
	statuses := types.AllActionStatuses()
	expectedCount := 6

	gt.A(t, statuses).Length(expectedCount)

	// Verify all returned statuses are valid
	for _, status := range statuses {
		gt.B(t, status.IsValid()).
			Describef("Status %s should be valid", status).
			True()
	}

	// Verify all expected statuses are present
	expectedStatuses := []types.ActionStatus{
		types.ActionStatusBacklog,
		types.ActionStatusTodo,
		types.ActionStatusInProgress,
		types.ActionStatusBlocked,
		types.ActionStatusCompleted,
		types.ActionStatusAbandoned,
	}

	statusMap := make(map[types.ActionStatus]bool)
	for _, status := range statuses {
		statusMap[status] = true
	}

	for _, expected := range expectedStatuses {
		gt.B(t, statusMap[expected]).
			Describef("Expected status %s should be present", expected).
			True()
	}
}

func TestActionStatus_String(t *testing.T) {
	tests := []struct {
		name   string
		status types.ActionStatus
		want   string
	}{
		{
			name:   "backlog",
			status: types.ActionStatusBacklog,
			want:   "BACKLOG",
		},
		{
			name:   "todo",
			status: types.ActionStatusTodo,
			want:   "TODO",
		},
		{
			name:   "in-progress",
			status: types.ActionStatusInProgress,
			want:   "IN_PROGRESS",
		},
		{
			name:   "blocked",
			status: types.ActionStatusBlocked,
			want:   "BLOCKED",
		},
		{
			name:   "completed",
			status: types.ActionStatusCompleted,
			want:   "COMPLETED",
		},
		{
			name:   "abandoned",
			status: types.ActionStatusAbandoned,
			want:   "ABANDONED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gt.S(t, tt.status.String()).Equal(tt.want)
		})
	}
}
