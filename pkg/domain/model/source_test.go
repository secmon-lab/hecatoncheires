package model_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestParseGitHubRepo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "owner/repo format",
			input:     "secmon-lab/hecatoncheires",
			wantOwner: "secmon-lab",
			wantRepo:  "hecatoncheires",
		},
		{
			name:      "GitHub HTTPS URL",
			input:     "https://github.com/secmon-lab/hecatoncheires",
			wantOwner: "secmon-lab",
			wantRepo:  "hecatoncheires",
		},
		{
			name:      "GitHub URL with trailing slash",
			input:     "https://github.com/secmon-lab/hecatoncheires/",
			wantOwner: "secmon-lab",
			wantRepo:  "hecatoncheires",
		},
		{
			name:      "GitHub URL with .git suffix",
			input:     "https://github.com/secmon-lab/hecatoncheires.git",
			wantOwner: "secmon-lab",
			wantRepo:  "hecatoncheires",
		},
		{
			name:      "owner/repo with dots and underscores",
			input:     "my_org/my.repo",
			wantOwner: "my_org",
			wantRepo:  "my.repo",
		},
		{
			name:      "input with leading/trailing spaces",
			input:     "  secmon-lab/hecatoncheires  ",
			wantOwner: "secmon-lab",
			wantRepo:  "hecatoncheires",
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
			name:    "just a name without slash",
			input:   "hecatoncheires",
			wantErr: true,
		},
		{
			name:    "URL with wrong host",
			input:   "https://gitlab.com/secmon-lab/hecatoncheires",
			wantErr: true,
		},
		{
			name:    "URL with extra path segments",
			input:   "https://github.com/secmon-lab/hecatoncheires/tree/main",
			wantErr: true,
		},
		{
			name:    "three-part path",
			input:   "org/team/repo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			owner, repo, err := model.ParseGitHubRepo(tt.input)
			if tt.wantErr {
				gt.Error(t, err).Is(model.ErrInvalidGitHubRepo)
				return
			}
			gt.NoError(t, err).Required()
			gt.Value(t, owner).Equal(tt.wantOwner)
			gt.Value(t, repo).Equal(tt.wantRepo)
		})
	}
}

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
		{
			name:  "app.notion.com URL with /p path and title prefix",
			input: "https://app.notion.com/p/myworkspace/My-Database-a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			want:  wantUUID,
		},
		{
			name:  "URL with uppercase host and scheme",
			input: "HTTPS://WWW.NOTION.SO/workspace/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			want:  wantUUID,
		},
		{
			name:  "URL with uppercase hex ID in path",
			input: "https://www.notion.so/workspace/A1B2C3D4E5F6A7B8C9D0E1F2A3B4C5D6",
			want:  wantUUID,
		},
		// Error cases
		// Host-spoofing inputs the exact-match allow-list must reject. These
		// pin the security contract: a future switch to suffix/substring
		// matching would regress here.
		{
			name:    "subdomain of notion host",
			input:   "https://evil.app.notion.com/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			wantErr: true,
		},
		{
			name:    "notion host as prefix of attacker domain",
			input:   "https://app.notion.com.evil.example/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			wantErr: true,
		},
		{
			name:    "notion host in userinfo before attacker host",
			input:   "https://app.notion.com@evil.example/a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			wantErr: true,
		},
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
				gt.Error(t, err).Is(model.ErrInvalidNotionID)
				return
			}
			gt.NoError(t, err).Required()
			gt.Value(t, got).Equal(tt.want)
		})
	}
}

func TestSource_Validate(t *testing.T) {
	t.Run("valid source passes", func(t *testing.T) {
		s := &model.Source{ID: model.NewSourceID(), Name: "My DB", SourceType: model.SourceTypeNotionDB}
		gt.NoError(t, s.Validate())
	})

	t.Run("nil source is rejected", func(t *testing.T) {
		var s *model.Source
		gt.Error(t, s.Validate()).Is(model.ErrSourceValidation)
	})

	t.Run("missing Name is rejected", func(t *testing.T) {
		s := &model.Source{SourceType: model.SourceTypeSlack}
		gt.Error(t, s.Validate()).Is(model.ErrSourceValidation)
	})

	t.Run("missing SourceType is rejected", func(t *testing.T) {
		s := &model.Source{Name: "typeless"}
		gt.Error(t, s.Validate()).Is(model.ErrSourceValidation)
	})
}
