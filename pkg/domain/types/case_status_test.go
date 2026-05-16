package types_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

func TestCaseStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status types.CaseStatus
		want   bool
	}{
		{
			name:   "valid draft",
			status: types.CaseStatusDraft,
			want:   true,
		},
		{
			name:   "valid open",
			status: types.CaseStatusOpen,
			want:   true,
		},
		{
			name:   "valid closed",
			status: types.CaseStatusClosed,
			want:   true,
		},
		{
			name:   "invalid status",
			status: types.CaseStatus("invalid"),
			want:   false,
		},
		{
			name:   "empty status",
			status: types.CaseStatus(""),
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

func TestParseCaseStatus(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    types.CaseStatus
		wantErr bool
	}{
		{
			name:    "valid draft",
			input:   "DRAFT",
			want:    types.CaseStatusDraft,
			wantErr: false,
		},
		{
			name:    "valid open",
			input:   "OPEN",
			want:    types.CaseStatusOpen,
			wantErr: false,
		},
		{
			name:    "valid closed",
			input:   "CLOSED",
			want:    types.CaseStatusClosed,
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
			got, err := types.ParseCaseStatus(tt.input)
			if tt.wantErr {
				gt.Error(t, err)
			} else {
				gt.NoError(t, err)
				gt.V(t, got).Equal(tt.want)
			}
		})
	}
}

func TestAllCaseStatuses(t *testing.T) {
	statuses := types.AllCaseStatuses()
	gt.A(t, statuses).Length(3)

	for _, status := range statuses {
		gt.B(t, status.IsValid()).
			Describef("Status %s should be valid", status).
			True()
	}

	statusMap := make(map[types.CaseStatus]bool)
	for _, status := range statuses {
		statusMap[status] = true
	}

	gt.B(t, statusMap[types.CaseStatusDraft]).True()
	gt.B(t, statusMap[types.CaseStatusOpen]).True()
	gt.B(t, statusMap[types.CaseStatusClosed]).True()
}

func TestCaseStatus_Normalize(t *testing.T) {
	gt.V(t, types.CaseStatus("").Normalize()).Equal(types.CaseStatusOpen)
	gt.V(t, types.CaseStatusDraft.Normalize()).Equal(types.CaseStatusDraft)
	gt.V(t, types.CaseStatusOpen.Normalize()).Equal(types.CaseStatusOpen)
	gt.V(t, types.CaseStatusClosed.Normalize()).Equal(types.CaseStatusClosed)
}

func TestCaseStatus_IsDraft(t *testing.T) {
	gt.B(t, types.CaseStatusDraft.IsDraft()).True()
	gt.B(t, types.CaseStatusOpen.IsDraft()).False()
	gt.B(t, types.CaseStatusClosed.IsDraft()).False()
	gt.B(t, types.CaseStatus("").IsDraft()).False()
}

func TestCaseStatus_String(t *testing.T) {
	gt.S(t, types.CaseStatusDraft.String()).Equal("DRAFT")
	gt.S(t, types.CaseStatusOpen.String()).Equal("OPEN")
	gt.S(t, types.CaseStatusClosed.String()).Equal("CLOSED")
}
