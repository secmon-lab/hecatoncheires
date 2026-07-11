package usecase_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// TestBuildThreadCreateQuestionBlocks_Fallback locks the notification
// fallback of the thread-mode question form to the i18n layer: English is
// the historical hardcoded string, and a Japanese-locale context must yield
// the Japanese translation (compared against the same i18n source the
// production code reads, not hardcoded here).
func TestBuildThreadCreateQuestionBlocks_Fallback(t *testing.T) {
	items := []model.PendingQuestionItem{
		{ID: "severity", Text: "How severe is it?", Type: "select", Options: []string{"high", "low"}},
	}

	t.Run("default context yields English fallback", func(t *testing.T) {
		blocks, fallback := usecase.BuildThreadCreateQuestionBlocksForTest(context.Background(), "need severity", items, "C-CASE:1700000000.000100", "U123")
		gt.Number(t, len(blocks)).GreaterOrEqual(1)
		gt.Value(t, fallback).Equal("We need a bit more info to create this case.")
	})

	t.Run("Japanese context yields localized fallback", func(t *testing.T) {
		jaCtx := i18n.ContextWithLang(context.Background(), i18n.LangJA)
		blocks, fallback := usecase.BuildThreadCreateQuestionBlocksForTest(jaCtx, "need severity", items, "C-CASE:1700000000.000100", "U123")
		gt.Number(t, len(blocks)).GreaterOrEqual(1)
		gt.Value(t, fallback).Equal(i18n.T(jaCtx, i18n.MsgThreadCaseQuestionFallback))
		gt.Value(t, fallback).NotEqual("We need a bit more info to create this case.")
	})
}

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
