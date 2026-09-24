package usecase_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestSlackUseCases_NotifyWorkspaceAccessDenied(t *testing.T) {
	i18n.Init(i18n.LangEN)
	ctx := context.Background()

	t.Run("tells the denied user alone, in the permission message", func(t *testing.T) {
		slackMock := &mockSlackService{}
		uc := usecase.NewSlackUseCases(memory.New(), model.NewWorkspaceRegistry(), nil, nil, slackMock)

		uc.NotifyWorkspaceAccessDenied(ctx, "C-CASE", "U-DENIED", model.ErrWorkspaceAccessDenied)

		gt.Value(t, slackMock.ephemeralChannelID).Equal("C-CASE")
		gt.Value(t, slackMock.ephemeralUserID).Equal("U-DENIED")
		gt.String(t, slackMock.ephemeralText).Contains(i18n.T(ctx, i18n.MsgUIErrWorkspaceAccessDeniedWhat))
	})

	t.Run("posts nothing without a channel to post in", func(t *testing.T) {
		slackMock := &mockSlackService{}
		uc := usecase.NewSlackUseCases(memory.New(), model.NewWorkspaceRegistry(), nil, nil, slackMock)

		uc.NotifyWorkspaceAccessDenied(ctx, "", "U-DENIED", model.ErrWorkspaceAccessDenied)

		gt.Value(t, slackMock.ephemeralUserID).Equal("")
		gt.Value(t, slackMock.ephemeralText).Equal("")
	})

	t.Run("does nothing when Slack is not wired", func(t *testing.T) {
		uc := usecase.NewSlackUseCases(memory.New(), model.NewWorkspaceRegistry(), nil, nil, nil)

		// Must not panic on the nil poster.
		uc.NotifyWorkspaceAccessDenied(ctx, "C-CASE", "U-DENIED", model.ErrWorkspaceAccessDenied)
	})
}
