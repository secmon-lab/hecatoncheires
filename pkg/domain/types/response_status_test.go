package types_test

import (
	"testing"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

func TestResponseStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status types.ResponseStatus
		want   bool
	}{
		{
			name:   "valid backlog",
			status: types.ResponseStatusBacklog,
			want:   true,
		},
		{
			name:   "valid todo",
			status: types.ResponseStatusTodo,
			want:   true,
		},
		{
			name:   "valid in-progress",
			status: types.ResponseStatusInProgress,
			want:   true,
		},
		{
			name:   "valid blocked",
			status: types.ResponseStatusBlocked,
			want:   true,
		},
		{
			name:   "valid completed",
			status: types.ResponseStatusCompleted,
			want:   true,
		},
		{
			name:   "valid abandoned",
			status: types.ResponseStatusAbandoned,
			want:   true,
		},
		{
			name:   "invalid status",
			status: types.ResponseStatus("invalid"),
			want:   false,
		},
		{
			name:   "empty status",
			status: types.ResponseStatus(""),
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.want {
				t.Errorf("ResponseStatus.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseResponseStatus(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    types.ResponseStatus
		wantErr bool
	}{
		{
			name:    "valid backlog",
			input:   "backlog",
			want:    types.ResponseStatusBacklog,
			wantErr: false,
		},
		{
			name:    "valid todo",
			input:   "todo",
			want:    types.ResponseStatusTodo,
			wantErr: false,
		},
		{
			name:    "valid in-progress",
			input:   "in-progress",
			want:    types.ResponseStatusInProgress,
			wantErr: false,
		},
		{
			name:    "valid blocked",
			input:   "blocked",
			want:    types.ResponseStatusBlocked,
			wantErr: false,
		},
		{
			name:    "valid completed",
			input:   "completed",
			want:    types.ResponseStatusCompleted,
			wantErr: false,
		},
		{
			name:    "valid abandoned",
			input:   "abandoned",
			want:    types.ResponseStatusAbandoned,
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
			got, err := types.ParseResponseStatus(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseResponseStatus() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseResponseStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAllResponseStatuses(t *testing.T) {
	statuses := types.AllResponseStatuses()
	expectedCount := 6

	if len(statuses) != expectedCount {
		t.Errorf("AllResponseStatuses() returned %d statuses, want %d", len(statuses), expectedCount)
	}

	// Verify all returned statuses are valid
	for _, status := range statuses {
		if !status.IsValid() {
			t.Errorf("AllResponseStatuses() returned invalid status: %v", status)
		}
	}

	// Verify all expected statuses are present
	expectedStatuses := []types.ResponseStatus{
		types.ResponseStatusBacklog,
		types.ResponseStatusTodo,
		types.ResponseStatusInProgress,
		types.ResponseStatusBlocked,
		types.ResponseStatusCompleted,
		types.ResponseStatusAbandoned,
	}

	statusMap := make(map[types.ResponseStatus]bool)
	for _, status := range statuses {
		statusMap[status] = true
	}

	for _, expected := range expectedStatuses {
		if !statusMap[expected] {
			t.Errorf("AllResponseStatuses() missing expected status: %v", expected)
		}
	}
}

func TestResponseStatus_String(t *testing.T) {
	tests := []struct {
		name   string
		status types.ResponseStatus
		want   string
	}{
		{
			name:   "backlog",
			status: types.ResponseStatusBacklog,
			want:   "backlog",
		},
		{
			name:   "todo",
			status: types.ResponseStatusTodo,
			want:   "todo",
		},
		{
			name:   "in-progress",
			status: types.ResponseStatusInProgress,
			want:   "in-progress",
		},
		{
			name:   "blocked",
			status: types.ResponseStatusBlocked,
			want:   "blocked",
		},
		{
			name:   "completed",
			status: types.ResponseStatusCompleted,
			want:   "completed",
		},
		{
			name:   "abandoned",
			status: types.ResponseStatusAbandoned,
			want:   "abandoned",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("ResponseStatus.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
