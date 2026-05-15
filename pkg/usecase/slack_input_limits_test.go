package usecase_test

import (
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestClampPlainText_EmptyString(t *testing.T) {
	gt.String(t, usecase.ClampPlainTextForTest("", false)).Equal("")
	gt.String(t, usecase.ClampPlainTextForTest("", true)).Equal("")
}

func TestClampPlainText_ShortStringPassesThrough(t *testing.T) {
	short := "hello world"
	gt.String(t, usecase.ClampPlainTextForTest(short, false)).Equal(short)
	gt.String(t, usecase.ClampPlainTextForTest(short, true)).Equal(short)
}

func TestClampPlainText_BoundaryAtMax(t *testing.T) {
	// Exactly the ceiling passes through unchanged.
	exactlyMax := strings.Repeat("a", usecase.SlackPlainTextMaxRunesForTest)
	got := usecase.ClampPlainTextForTest(exactlyMax, true)
	gt.Number(t, len([]rune(got))).Equal(usecase.SlackPlainTextMaxRunesForTest)
	gt.String(t, got).Equal(exactlyMax)
}

func TestClampPlainText_OneOverMaxIsClamped(t *testing.T) {
	overByOne := strings.Repeat("a", usecase.SlackPlainTextMaxRunesForTest+1)

	gotMultiline := usecase.ClampPlainTextForTest(overByOne, true)
	gt.Bool(t, strings.HasSuffix(gotMultiline, usecase.ClampSuffixMultiLineForTest)).True()
	gt.Number(t, len([]rune(gotMultiline))).Equal(
		usecase.SlackPlainTextMaxRunesForTest + len([]rune(usecase.ClampSuffixMultiLineForTest)),
	)

	gotSingle := usecase.ClampPlainTextForTest(overByOne, false)
	gt.Bool(t, strings.HasSuffix(gotSingle, usecase.ClampSuffixSingleLineForTest)).True()
	gt.Number(t, len([]rune(gotSingle))).Equal(
		usecase.SlackPlainTextMaxRunesForTest + len([]rune(usecase.ClampSuffixSingleLineForTest)),
	)
}

func TestClampPlainText_RespectsRuneBoundaries(t *testing.T) {
	// Build a string of multibyte (Japanese) characters that exceeds the
	// ceiling. After clamping, the rune count must equal ceiling + suffix,
	// and the byte slice must still decode cleanly (no severed multibyte
	// sequence). Detecting a severed rune surfaces as a U+FFFD replacement
	// when re-decoded, so we assert the suffix is intact and the prefix
	// rune-count is exactly the ceiling.
	const jaRune = "あ"
	long := strings.Repeat(jaRune, usecase.SlackPlainTextMaxRunesForTest+50)
	got := usecase.ClampPlainTextForTest(long, true)

	gt.Bool(t, strings.HasSuffix(got, usecase.ClampSuffixMultiLineForTest)).True()
	prefix := strings.TrimSuffix(got, usecase.ClampSuffixMultiLineForTest)
	gt.Number(t, len([]rune(prefix))).Equal(usecase.SlackPlainTextMaxRunesForTest)
	// Every prefix rune is the expected multibyte char — no severed sequences.
	gt.String(t, prefix).Equal(strings.Repeat(jaRune, usecase.SlackPlainTextMaxRunesForTest))
}

func TestClampPlainText_FivekDescriptionFitsUnderSlackCeiling(t *testing.T) {
	// 5000 ASCII chars is the scenario from the Sentry incident. After
	// clamping, the rune length must be well below Slack's 3000 ceiling.
	long := strings.Repeat("x", 5000)
	got := usecase.ClampPlainTextForTest(long, true)
	gt.Number(t, len([]rune(got))).LessOrEqual(2600)
	gt.Bool(t, strings.HasSuffix(got, usecase.ClampSuffixMultiLineForTest)).True()
}

func TestIsLikelySlackUserID(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"normal user ID", "U01ABC234", true},
		{"enterprise grid user", "W01XYZ", true},
		{"too short", "U", false},
		{"single-char W", "W", false},
		{"empty", "", false},
		{"lowercase rejected", "u01abc234", false},
		{"mixed case rejected", "U01abc234", false},
		{"email-shaped value", "alice@example.com", false},
		{"plain name", "alice", false},
		{"bot ID rejected", "B01ABC234", false},
		{"channel ID rejected", "C01ABC234", false},
		{"two char (boundary)", "U12", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := usecase.IsLikelySlackUserIDForTest(tc.in)
			if tc.want {
				gt.Bool(t, got).True()
			} else {
				gt.Bool(t, got).False()
			}
		})
	}
}

func TestFilterSlackUserIDs(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		gt.Value(t, usecase.FilterSlackUserIDsForTest(nil)).Nil()
	})

	t.Run("all valid passes through", func(t *testing.T) {
		in := []string{"U01ABC", "W02DEF"}
		got := usecase.FilterSlackUserIDsForTest(in)
		gt.Array(t, got).Length(2).Required()
		gt.String(t, got[0]).Equal("U01ABC")
		gt.String(t, got[1]).Equal("W02DEF")
	})

	t.Run("invalid entries are dropped, order preserved", func(t *testing.T) {
		in := []string{"U01ABC", "alice@example.com", "W02DEF", "", "B01BOT"}
		got := usecase.FilterSlackUserIDsForTest(in)
		gt.Array(t, got).Length(2).Required()
		gt.String(t, got[0]).Equal("U01ABC")
		gt.String(t, got[1]).Equal("W02DEF")
	})

	t.Run("all invalid yields empty slice", func(t *testing.T) {
		in := []string{"alice", "bob@example.com"}
		got := usecase.FilterSlackUserIDsForTest(in)
		gt.Array(t, got).Length(0)
	})
}
