package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
)

func TestLoadAppConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid configuration",
			content: `
[[category]]
id = "data-breach"
name = "データ漏洩"
description = "個人情報や機密情報の漏洩リスク"

[[likelihood]]
id = "very-low"
name = "極めて低い"
description = "発生する可能性が極めて低い"
score = 1

[[impact]]
id = "critical"
name = "致命的"
description = "事業継続に深刻な影響"
score = 5

[[team]]
id = "security-team"
name = "セキュリティチーム"
`,
			wantErr: false,
		},
		{
			name: "duplicate category ID",
			content: `
[[category]]
id = "data-breach"
name = "データ漏洩"
description = "個人情報や機密情報の漏洩リスク"

[[category]]
id = "data-breach"
name = "重複"
description = "重複ID"
`,
			wantErr: true,
			errMsg:  "duplicate category ID",
		},
		{
			name: "invalid category ID format",
			content: `
[[category]]
id = "Data-Breach"
name = "データ漏洩"
description = "大文字を含む"
`,
			wantErr: true,
			errMsg:  "invalid category ID",
		},
		{
			name: "missing category name",
			content: `
[[category]]
id = "data-breach"
description = "名前なし"
`,
			wantErr: true,
			errMsg:  "category name is required",
		},
		{
			name: "likelihood score out of range",
			content: `
[[likelihood]]
id = "very-low"
name = "極めて低い"
description = "発生する可能性が極めて低い"
score = 6
`,
			wantErr: true,
			errMsg:  "likelihood score must be between 1 and 5",
		},
		{
			name: "impact score out of range",
			content: `
[[impact]]
id = "critical"
name = "致命的"
description = "事業継続に深刻な影響"
score = 0
`,
			wantErr: true,
			errMsg:  "impact score must be between 1 and 5",
		},
		{
			name: "duplicate likelihood ID",
			content: `
[[likelihood]]
id = "low"
name = "低い"
score = 1

[[likelihood]]
id = "low"
name = "重複"
score = 2
`,
			wantErr: true,
			errMsg:  "duplicate likelihood ID",
		},
		{
			name: "duplicate impact ID",
			content: `
[[impact]]
id = "high"
name = "高い"
score = 4

[[impact]]
id = "high"
name = "重複"
score = 5
`,
			wantErr: true,
			errMsg:  "duplicate impact ID",
		},
		{
			name: "duplicate team ID",
			content: `
[[team]]
id = "security-team"
name = "セキュリティチーム"

[[team]]
id = "security-team"
name = "重複チーム"
`,
			wantErr: true,
			errMsg:  "duplicate team ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.toml")
			if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to create temp config file: %v", err)
			}

			cfg, err := config.LoadAppConfiguration(configPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadAppConfiguration() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil && tt.errMsg != "" {
				if errStr := err.Error(); !strings.Contains(errStr, tt.errMsg) {
					t.Errorf("LoadAppConfiguration() error message = %v, want to contain %v", errStr, tt.errMsg)
				}
			}

			if !tt.wantErr && cfg == nil {
				t.Error("LoadAppConfiguration() returned nil config without error")
			}
		})
	}
}

