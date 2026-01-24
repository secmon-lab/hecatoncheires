package slack_test

import (
	"testing"
	"unicode/utf8"

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
		{
			name:     "Japanese only exceeding 80 bytes truncated safely",
			riskID:   1,
			riskName: "あいうえおかきくけこさしすせそたちつてとなにぬねのはひふへほ", // 30 Japanese chars = 90 bytes
			prefix:   "risk",
			want:     "risk-1-あいうえおかきくけこさしすせそたちつてとなにぬね", // 7 + 72 = 79 bytes (24 Japanese chars)
		},
		{
			name:     "mixed Japanese and English exceeding 80 bytes",
			riskID:   1,
			riskName: "セキュリティインシデント-Critical-Alert-重要な警告メッセージ",
			prefix:   "incident",
			want:     "incident-1-セキュリティインシデント-critical-alert-重要な警告", // Truncated at character boundary
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slack.GenerateRiskChannelName(tt.riskID, tt.riskName, tt.prefix)
			if got != tt.want {
				t.Errorf("GenerateRiskChannelName(%d, %q, %q) = %q (len=%d), want %q (len=%d)",
					tt.riskID, tt.riskName, tt.prefix, got, len(got), tt.want, len(tt.want))
			}
			// Verify byte length constraint (80 bytes max for Slack)
			if len(got) > 80 {
				t.Errorf("GenerateRiskChannelName(%d, %q, %q) returned a name longer than 80 bytes: %q (len=%d bytes)",
					tt.riskID, tt.riskName, tt.prefix, got, len(got))
			}
			// Verify UTF-8 validity (truncation should not corrupt multi-byte characters)
			if !utf8.ValidString(got) {
				t.Errorf("GenerateRiskChannelName(%d, %q, %q) returned invalid UTF-8: %q",
					tt.riskID, tt.riskName, tt.prefix, got)
			}
		})
	}
}

func TestTruncateToMaxBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxBytes int
		want     string
	}{
		{
			name:     "ASCII string within limit",
			input:    "hello",
			maxBytes: 10,
			want:     "hello",
		},
		{
			name:     "ASCII string at exact limit",
			input:    "hello",
			maxBytes: 5,
			want:     "hello",
		},
		{
			name:     "ASCII string exceeds limit",
			input:    "hello world",
			maxBytes: 5,
			want:     "hello",
		},
		{
			name:     "Japanese string within limit",
			input:    "あいう", // 9 bytes
			maxBytes: 10,
			want:     "あいう",
		},
		{
			name:     "Japanese string at character boundary",
			input:    "あいう", // 9 bytes (3 bytes each)
			maxBytes: 9,
			want:     "あいう",
		},
		{
			name:     "Japanese string truncated at boundary",
			input:    "あいうえお", // 15 bytes
			maxBytes: 10,
			want:     "あいう", // 9 bytes (cannot fit 4th char as it would be 12 bytes)
		},
		{
			name:     "Japanese string truncated mid-character",
			input:    "あいうえお", // 15 bytes
			maxBytes: 8,       // 8 bytes cannot fit 3 complete Japanese chars
			want:     "あい",    // 6 bytes
		},
		{
			name:     "mixed content truncated safely",
			input:    "abcあいう", // 3 + 9 = 12 bytes
			maxBytes: 10,
			want:     "abcあい", // 3 + 6 = 9 bytes
		},
		{
			name:     "empty string",
			input:    "",
			maxBytes: 10,
			want:     "",
		},
		{
			name:     "zero max bytes",
			input:    "hello",
			maxBytes: 0,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slack.TruncateToMaxBytes(tt.input, tt.maxBytes)
			if got != tt.want {
				t.Errorf("TruncateToMaxBytes(%q, %d) = %q (len=%d), want %q (len=%d)",
					tt.input, tt.maxBytes, got, len(got), tt.want, len(tt.want))
			}
			// Always verify the result is valid UTF-8
			if !utf8.ValidString(got) {
				t.Errorf("TruncateToMaxBytes(%q, %d) returned invalid UTF-8: %q",
					tt.input, tt.maxBytes, got)
			}
			// Verify byte length constraint
			if len(got) > tt.maxBytes {
				t.Errorf("TruncateToMaxBytes(%q, %d) returned string with %d bytes, exceeds limit",
					tt.input, tt.maxBytes, len(got))
			}
		})
	}
}
