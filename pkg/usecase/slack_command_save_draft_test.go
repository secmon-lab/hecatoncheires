package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	goslack "github.com/slack-go/slack"
)

// updateViewCall records arguments to mockSlackService.UpdateView so the
// Save-as-Draft tests can assert exact view content.
type updateViewCall struct {
	view       goslack.ModalViewRequest
	externalID string
	hash       string
	viewID     string
}

// saveDraftMockSlack extends commandTestSlackService with UpdateView capture.
// Only the splash-update path of the Save-as-Draft flow needs this hook;
// existing slash-command tests can keep using the unembellished mock.
type saveDraftMockSlack struct {
	commandTestSlackService
	updateViewCalls []updateViewCall
}

func (m *saveDraftMockSlack) UpdateView(_ context.Context, view goslack.ModalViewRequest, externalID, hash, viewID string) error {
	m.updateViewCalls = append(m.updateViewCalls, updateViewCall{
		view:       view,
		externalID: externalID,
		hash:       hash,
		viewID:     viewID,
	})
	return nil
}

// newSaveDraftCallback builds a block_actions InteractionCallback whose
// view carries the form state and private_metadata expected by
// HandleSaveAsDraftClick. The callback's User is set to userID so the
// CreateDraft → reporter mapping flows through as it would in production.
func newSaveDraftCallback(t *testing.T, userID, workspaceID, channelID, title, description string, isPrivate bool) *goslack.InteractionCallback {
	t.Helper()
	meta := map[string]string{
		"workspace_id": workspaceID,
		"channel_id":   channelID,
	}
	metaJSON, err := json.Marshal(meta)
	gt.NoError(t, err).Required()

	values := map[string]map[string]goslack.BlockAction{
		usecase.SlackBlockIDCaseTitle: {
			usecase.SlackActionIDCaseTitle: {
				Value: title,
			},
		},
		usecase.SlackBlockIDCaseDescription: {
			usecase.SlackActionIDCaseDescription: {
				Value: description,
			},
		},
	}
	if isPrivate {
		values[usecase.SlackBlockIDCasePrivate] = map[string]goslack.BlockAction{
			usecase.SlackActionIDCasePrivate: {
				SelectedOptions: []goslack.OptionBlockObject{{Value: "private"}},
			},
		}
	}

	return &goslack.InteractionCallback{
		Type: goslack.InteractionTypeBlockActions,
		User: goslack.User{ID: userID},
		View: goslack.View{
			ID:              "V-VIEW-1",
			PrivateMetadata: string(metaJSON),
			State: &goslack.ViewState{
				Values: values,
			},
		},
	}
}

func TestSlackUseCases_HandleSaveAsDraftClick(t *testing.T) {
	i18n.Init(i18n.LangEN)

	t.Run("persists draft and updates modal + posts ephemeral", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-1", Name: "WS 1"},
		})

		slackMock := &saveDraftMockSlack{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		seedSlackUsers(t, repo, "UREPORTER")

		cb := newSaveDraftCallback(t, "UREPORTER", "ws-1", "C-ORIGIN", "Working title", "draft body", true)

		err := slackUC.HandleSaveAsDraftClick(context.Background(), caseUC, cb)
		gt.NoError(t, err).Required()

		// Exactly one draft is persisted, owned by the reporter.
		drafts, err := repo.Case().ListDrafts(context.Background(), "ws-1")
		gt.NoError(t, err).Required()
		gt.Number(t, len(drafts)).Equal(1)
		gt.Value(t, drafts[0].Status).Equal(types.CaseStatusDraft)
		gt.Value(t, drafts[0].Title).Equal("Working title")
		gt.Value(t, drafts[0].Description).Equal("draft body")
		gt.Value(t, drafts[0].IsPrivate).Equal(true)
		gt.Value(t, drafts[0].ReporterID).Equal("UREPORTER")
		// Save-as-Draft auto-assigns the clicker, mirroring the regular
		// Submit path's behaviour, so the eventual case has the user as
		// an assignee (and therefore a channel member after activation).
		gt.Number(t, len(drafts[0].AssigneeIDs)).Equal(1)
		gt.Value(t, drafts[0].AssigneeIDs[0]).Equal("UREPORTER")

		// Modal was replaced with the splash view.
		gt.Number(t, len(slackMock.updateViewCalls)).Equal(1)
		gt.Value(t, slackMock.updateViewCalls[0].viewID).Equal("V-VIEW-1")

		// Ephemeral confirmation was posted to the originating channel.
		gt.Value(t, slackMock.ephemeralChannelID).Equal("C-ORIGIN")
		gt.Value(t, slackMock.ephemeralUserID).Equal("UREPORTER")
		gt.String(t, slackMock.ephemeralText).Contains("draft")
	})

	t.Run("blank title is still saved (drafts allow empty title)", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-1", Name: "WS 1"},
		})

		slackMock := &saveDraftMockSlack{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		seedSlackUsers(t, repo, "UREPORTER")

		cb := newSaveDraftCallback(t, "UREPORTER", "ws-1", "C-ORIGIN", "", "just a body", false)

		err := slackUC.HandleSaveAsDraftClick(context.Background(), caseUC, cb)
		gt.NoError(t, err).Required()

		drafts, err := repo.Case().ListDrafts(context.Background(), "ws-1")
		gt.NoError(t, err).Required()
		gt.Number(t, len(drafts)).Equal(1)
		gt.Value(t, drafts[0].Title).Equal("")
	})

	t.Run("nil caseUC produces a configuration error", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		slackMock := &saveDraftMockSlack{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		cb := newSaveDraftCallback(t, "UREPORTER", "ws-1", "C-ORIGIN", "T", "D", false)
		err := slackUC.HandleSaveAsDraftClick(context.Background(), nil, cb)
		gt.Error(t, err)

		drafts, listErr := repo.Case().ListDrafts(context.Background(), "ws-1")
		gt.NoError(t, listErr).Required()
		gt.Number(t, len(drafts)).Equal(0)
	})

	t.Run("invalid private_metadata propagates error", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		slackMock := &saveDraftMockSlack{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")

		// Build a callback with deliberately malformed private_metadata.
		cb := &goslack.InteractionCallback{
			Type: goslack.InteractionTypeBlockActions,
			User: goslack.User{ID: "UREPORTER"},
			View: goslack.View{
				ID:              "V-VIEW-2",
				PrivateMetadata: "not-json",
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{},
				},
			},
		}

		err := slackUC.HandleSaveAsDraftClick(context.Background(), caseUC, cb)
		gt.Error(t, err)

		// Nothing should have been persisted nor posted.
		gt.Number(t, len(slackMock.updateViewCalls)).Equal(0)
		gt.String(t, slackMock.ephemeralChannelID).Equal("")
	})

	t.Run("ephemeral failure does not propagate to caller", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-1", Name: "WS 1"},
		})

		slackMock := &saveDraftMockSlack{}
		slackMock.postEphemeralFn = func(_ context.Context, _ string, _ string, _ string) error {
			return errors.New("slack ephemeral down")
		}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		seedSlackUsers(t, repo, "UREPORTER")

		cb := newSaveDraftCallback(t, "UREPORTER", "ws-1", "C-ORIGIN", "Working title", "", false)
		err := slackUC.HandleSaveAsDraftClick(context.Background(), caseUC, cb)
		// Draft must be saved regardless of the ephemeral hiccup.
		gt.NoError(t, err).Required()

		drafts, err := repo.Case().ListDrafts(context.Background(), "ws-1")
		gt.NoError(t, err).Required()
		gt.Number(t, len(drafts)).Equal(1)
	})

	t.Run("no channel skips ephemeral but still saves", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-1", Name: "WS 1"},
		})

		slackMock := &saveDraftMockSlack{}
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
		seedSlackUsers(t, repo, "UREPORTER")

		cb := newSaveDraftCallback(t, "UREPORTER", "ws-1", "", "T", "D", false)
		err := slackUC.HandleSaveAsDraftClick(context.Background(), caseUC, cb)
		gt.NoError(t, err).Required()

		gt.String(t, slackMock.ephemeralChannelID).Equal("")
		gt.Number(t, len(slackMock.updateViewCalls)).Equal(1)
	})
}
