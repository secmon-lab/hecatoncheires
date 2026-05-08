package types_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

func TestActionStatus_String(t *testing.T) {
	tests := []struct {
		name   string
		status types.ActionStatus
		want   string
	}{
		{name: "backlog", status: types.ActionStatusBacklog, want: "BACKLOG"},
		{name: "todo", status: types.ActionStatusTodo, want: "TODO"},
		{name: "in-progress", status: types.ActionStatusInProgress, want: "IN_PROGRESS"},
		{name: "blocked", status: types.ActionStatusBlocked, want: "BLOCKED"},
		{name: "completed", status: types.ActionStatusCompleted, want: "COMPLETED"},
		{name: "custom", status: types.ActionStatus("custom"), want: "custom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gt.Value(t, tt.status.String()).Equal(tt.want)
		})
	}
}
