package slack_test

import (
	"testing"

	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

func TestNormalizeChannelName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "basic pattern",
			input: "Test Risk",
			want:  "test-risk",
		},
		{
			name:  "uppercase conversion",
			input: "UPPERCASE",
			want:  "uppercase",
		},
		{
			name:  "multiple spaces",
			input: "multiple   spaces",
			want:  "multiple---spaces",
		},
		{
			name:  "Japanese preserved",
			input: "焼きそばパン売り切れ",
			want:  "焼きそばパン売り切れ",
		},
		{
			name:  "Japanese mixed with English",
			input: "テストTest123",
			want:  "テストtest123",
		},
		{
			name:  "symbols removed",
			input: "test!@#$%risk",
			want:  "testrisk",
		},
		{
			name:  "allowed characters preserved",
			input: "test-risk_123",
			want:  "test-risk_123",
		},
		{
			name:  "Japanese punctuation removed",
			input: "焼きそばパン、売り切れ。",
			want:  "焼きそばパン売り切れ",
		},
		{
			name:  "slash removed",
			input: "リスク管理/2024",
			want:  "リスク管理2024",
		},
		{
			name:  "complex pattern",
			input: "リスク#123 Test!",
			want:  "リスク123-test",
		},
		{
			name:  "underscores preserved",
			input: "リスク_管理#123",
			want:  "リスク_管理123",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slack.NormalizeChannelName(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeChannelName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGenerateRiskChannelName(t *testing.T) {
	tests := []struct {
		name     string
		riskID   int64
		riskName string
		prefix   string
		want     string
	}{
		{
			name:     "basic pattern with risk prefix",
			riskID:   1,
			riskName: "Test Risk",
			prefix:   "risk",
			want:     "risk-1-test-risk",
		},
		{
			name:     "Japanese preserved",
			riskID:   3,
			riskName: "焼きそばパン売り切れ",
			prefix:   "risk",
			want:     "risk-3-焼きそばパン売り切れ",
		},
		{
			name:     "Japanese mixed with English",
			riskID:   2,
			riskName: "リスク管理_Test",
			prefix:   "risk",
			want:     "risk-2-リスク管理_test",
		},
		{
			name:     "long name truncated to 80 characters",
			riskID:   1,
			riskName: "This is a very long risk name that will definitely exceed the 80 character limit for Slack channel names",
			prefix:   "risk",
			want:     "risk-1-this-is-a-very-long-risk-name-that-will-definitely-exceed-the-80-characte",
		},
		{
			name:     "trailing hyphen removed after truncation",
			riskID:   1,
			riskName: "This is a very long risk name that will definitely exceed the 80 character limit for Slack channel",
			prefix:   "risk",
			want:     "risk-1-this-is-a-very-long-risk-name-that-will-definitely-exceed-the-80-characte",
		},
		{
			name:     "empty name",
			riskID:   1,
			riskName: "",
			prefix:   "risk",
			want:     "risk-1",
		},
		{
			name:     "custom prefix",
			riskID:   1,
			riskName: "Test Risk",
			prefix:   "incident",
			want:     "incident-1-test-risk",
		},
		{
			name:     "custom Japanese prefix",
			riskID:   5,
			riskName: "脆弱性",
			prefix:   "セキュリティ",
			want:     "セキュリティ-5-脆弱性",
		},
		{
			name:     "custom prefix with special chars (normalized)",
			riskID:   2,
			riskName: "Security Issue",
			prefix:   "SEC Alert!",
			want:     "sec-alert-2-security-issue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slack.GenerateRiskChannelName(tt.riskID, tt.riskName, tt.prefix)
			if got != tt.want {
				t.Errorf("GenerateRiskChannelName(%d, %q, %q) = %q (len=%d), want %q (len=%d)",
					tt.riskID, tt.riskName, tt.prefix, got, len(got), tt.want, len(tt.want))
			}
			// Verify length constraint
			if len(got) > 80 {
				t.Errorf("GenerateRiskChannelName(%d, %q, %q) returned a name longer than 80 characters: %q (len=%d)",
					tt.riskID, tt.riskName, tt.prefix, got, len(got))
			}
		})
	}
}
