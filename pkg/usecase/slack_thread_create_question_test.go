package usecase_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestCaseThreadValueCodec(t *testing.T) {
	t.Run("round-trips channel and thread ts", func(t *testing.T) {
		v := usecase.EncodeCaseThreadValueForTest("C-MONITOR", "1700000000.000100")
		ch, ts, ok := usecase.ParseCaseThreadValueForTest(v)
		gt.Bool(t, ok).True()
		gt.String(t, ch).Equal("C-MONITOR")
		gt.String(t, ts).Equal("1700000000.000100")
	})

	t.Run("a bare thread ts (no channel) is not parseable", func(t *testing.T) {
		// No colon → the submit handler rejects it as malformed rather than
		// splitting on the ts's dot.
		_, _, ok := usecase.ParseCaseThreadValueForTest("1700000000.000100")
		gt.Bool(t, ok).False()
	})

	t.Run("empty and separator-only values are rejected", func(t *testing.T) {
		for _, v := range []string{"", ":", "C-ONLY:", ":1700000000.0001"} {
			_, _, ok := usecase.ParseCaseThreadValueForTest(v)
			gt.Bool(t, ok).False()
		}
	})
}
