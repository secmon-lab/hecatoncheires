package usecase_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	goslack "github.com/slack-go/slack"
)

// captureResponseURL spins up a local HTTP server that records POSTs from
// the response_url helper, returning its URL and a request log.
func captureResponseURL(t *testing.T) (string, *[]map[string]any) {
	t.Helper()
	var captured []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var parsed map[string]any
		_ = json.Unmarshal(body, &parsed)
		captured = append(captured, parsed)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &captured
}

func TestHandleCancel_DeletesDraftAndEphemeral(t *testing.T) {
	repo := memory.New()
	registry := newRegistryWithSchema("ws-1", "ws", &config.FieldSchema{})
	slackMock := newCollectorOnlyMockSlack()
	uc := usecase.NewMentionDraftUseCase(repo, registry, slackMock, usecase.NewDraftMaterializer(stubMaterializerLLM()))

	d := model.NewCaseDraft(time.Now().UTC(), "U1")
	d.SelectedWorkspaceID = "ws-1"
	gt.NoError(t, repo.CaseDraft().Save(context.Background(), d)).Required()

	respURL, captured := captureResponseURL(t)
	cb := &goslack.InteractionCallback{
		Type:        goslack.InteractionTypeBlockActions,
		ResponseURL: respURL,
		ActionCallback: goslack.ActionCallbacks{
			BlockActions: []*goslack.BlockAction{
				{ActionID: usecase.ActionIDDraftCancel, Value: string(d.ID)},
			},
		},
	}

	gt.NoError(t, uc.HandleCancel(context.Background(), cb, cb.ActionCallback.BlockActions[0])).Required()

	_, err := repo.CaseDraft().Get(context.Background(), d.ID)
	gt.Value(t, err).NotNil().Required()

	gt.Number(t, len(*captured)).GreaterOrEqual(1)
	// Cancel now replaces the original message in place with a "Canceled"
	// tail rather than deleting it; verify replace_original=true and that
	// the canceled marker is present in the rendered text/blocks.
	gt.Bool(t, (*captured)[0]["replace_original"] == true).True()
	textVal, _ := (*captured)[0]["text"].(string)
	gt.String(t, textVal).Contains("Case draft canceled")
}

func TestHandleSelectWorkspace_LocksFirstThenUpdates(t *testing.T) {
	repo := memory.New()
	schema := &config.FieldSchema{Fields: []config.FieldDefinition{
		{ID: "severity", Type: types.FieldTypeSelect,
			Options: []config.FieldOption{{ID: "low"}, {ID: "high"}}},
	}}
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws-A", Name: "WS-A"}, FieldSchema: schema,
	})
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws-B", Name: "WS-B"}, FieldSchema: schema,
	})

	slackMock := newCollectorOnlyMockSlack()
	uc := usecase.NewMentionDraftUseCase(repo, registry, slackMock, usecase.NewDraftMaterializer(stubMaterializerLLM()))

	d := model.NewCaseDraft(time.Now().UTC(), "U1")
	d.SelectedWorkspaceID = "ws-A"
	d.Materialization = &model.WorkspaceMaterialization{Title: "old", Description: "old desc"}
	gt.NoError(t, repo.CaseDraft().Save(context.Background(), d)).Required()

	respURL, captured := captureResponseURL(t)
	cb := &goslack.InteractionCallback{
		Type:        goslack.InteractionTypeBlockActions,
		ResponseURL: respURL,
		User:        goslack.User{ID: "U1"},
		ActionCallback: goslack.ActionCallbacks{
			BlockActions: []*goslack.BlockAction{
				{
					ActionID:       usecase.ActionIDDraftSelectWS,
					BlockID:        usecase.BlockIDDraftWSSelect + ":" + string(d.ID),
					Value:          string(d.ID),
					SelectedOption: goslack.OptionBlockObject{Value: "ws-B"},
				},
			},
		},
	}

	gt.NoError(t, uc.HandleSelectWorkspace(context.Background(), cb, cb.ActionCallback.BlockActions[0])).Required()

	// At least 2 POSTs to response_url: lock, then preview update.
	gt.Number(t, len(*captured)).GreaterOrEqual(2)

	got, err := repo.CaseDraft().Get(context.Background(), d.ID)
	gt.NoError(t, err).Required()
	gt.Value(t, got.SelectedWorkspaceID).Equal("ws-B")
	gt.Bool(t, got.InferenceInProgress).False()
	gt.Value(t, got.Materialization).NotNil().Required()
	gt.Value(t, got.Materialization.Title).Equal("AI suggested title")
}
