package model_test

import (
	"errors"
	"testing"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestParseNotionID(t *testing.T) {
	t.Parallel()

	// UUID format of a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6
	const wantUUID = "a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6"

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "raw 32-char hex ID",
			input: "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			want:  wantUUID,
		},
		{
			name:  "UUID format with dashes",
			input: "a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6",
			want:  wantUUID,
		},
		{
			name:  "uppercase hex ID",
			input: "A1B2C3D4E5F6A7B8C9D0E1F2A3B4C5D6",
			want:  wantUUID,
		},
		{
			name:  "Notion URL with workspace and title",
			input: "https://www.notion.so/myworkspace/My-Database-a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6?v=abc",
			want:  wantUUID,
		},
		{
			name:  "Notion URL without title prefix",
			input: "https://www.notion.so/myworkspace/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			want:  wantUUID,
		},
		{
			name:  "Notion URL without workspace",
			input: "https://www.notion.so/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			want:  wantUUID,
		},
		{
			name:  "Notion URL with notion.so (no www)",
			input: "https://notion.so/workspace/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6?v=xyz",
			want:  wantUUID,
		},
		{
			name:  "Notion URL with trailing slash",
			input: "https://www.notion.so/workspace/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6/",
			want:  wantUUID,
		},
		{
			name:  "input with leading/trailing spaces",
			input: "  a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6  ",
			want:  wantUUID,
		},
		{
			name:  "Notion URL with UUID dashes in path",
			input: "https://www.notion.so/workspace/a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6",
			want:  wantUUID,
		},
		{
			name:  "real Notion URL with source query param",
			input: "https://www.notion.so/mztn/2e6e628816658068b14bf84b39ad0762?v=2e6e6288166580199635000c717d60e7&source=copy_link",
			want:  "2e6e6288-1665-8068-b14b-f84b39ad0762",
		},
		// Error cases
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: true,
		},
		{
			name:    "too short hex",
			input:   "a1b2c3d4",
			wantErr: true,
		},
		{
			name:    "non-hex characters",
			input:   "g1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			wantErr: true,
		},
		{
			name:    "URL with wrong host",
			input:   "https://example.com/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			wantErr: true,
		},
		{
			name:    "Notion URL with no valid ID in path",
			input:   "https://www.notion.so/workspace/some-page",
			wantErr: true,
		},
		{
			name:    "random string",
			input:   "not-a-valid-id-at-all",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := model.ParseNotionID(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseNotionID(%q) expected error, got %q", tt.input, got)
				}
				if !errors.Is(err, model.ErrInvalidNotionID) {
					t.Errorf("ParseNotionID(%q) expected ErrInvalidNotionID, got %v", tt.input, err)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseNotionID(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("ParseNotionID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
