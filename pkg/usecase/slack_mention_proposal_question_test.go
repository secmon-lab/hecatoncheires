package usecase_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/proposal"
)

// TestBuildProposalQuestionBlocks_Fallback locks the notification fallback
// of the mention-draft question form to the i18n layer: the English text is
// the historical hardcoded string, and a Japanese-locale context must yield
// the Japanese translation (pulled from the same i18n source the production
// code reads, not hardcoded here).
func TestBuildProposalQuestionBlocks_Fallback(t *testing.T) {
	q := proposal.QuestionPayload{
		Reason: "need severity",
		Items: []proposal.QuestionItem{
			{ID: "severity", Text: "How severe is it?", Type: proposal.QuestionItemSelect, Options: []string{"high", "low"}},
		},
	}

	t.Run("default context yields English fallback", func(t *testing.T) {
		blocks, fallback := usecase.BuildProposalQuestionBlocksForTest(context.Background(), q, "draft-1", "U123")
		gt.Number(t, len(blocks)).GreaterOrEqual(1)
		gt.Value(t, fallback).Equal("We need a bit more info to draft this case.")
	})

	t.Run("Japanese context yields localized fallback", func(t *testing.T) {
		jaCtx := i18n.ContextWithLang(context.Background(), i18n.LangJA)
		blocks, fallback := usecase.BuildProposalQuestionBlocksForTest(jaCtx, q, "draft-1", "U123")
		gt.Number(t, len(blocks)).GreaterOrEqual(1)
		gt.Value(t, fallback).Equal(i18n.T(jaCtx, i18n.MsgMentionQuestionFallback))
		gt.Value(t, fallback).NotEqual("We need a bit more info to draft this case.")
	})
}
