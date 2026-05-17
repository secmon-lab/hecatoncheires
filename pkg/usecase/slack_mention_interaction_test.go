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
	uc := usecase.NewMentionProposalUseCase(repo, registry, slackMock, newDraftUC(t, repo, stubPlannerLLM(stubMaterializePlannerJSON("ws-1"))))

	d := model.NewCaseProposal(time.Now().UTC(), "U1")
	d.SelectedWorkspaceID = "ws-1"
	gt.NoError(t, repo.CaseProposal().Save(context.Background(), d)).Required()

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

	_, err := repo.CaseProposal().Get(context.Background(), d.ID)
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
	// stubPlannerLLM is keyed off the workspace_id baked into the JSON, so
	// the test fixture must materialize for ws-B (the destination of the
	// switch).
	uc := usecase.NewMentionProposalUseCase(repo, registry, slackMock, newDraftUC(t, repo, stubPlannerLLM(stubMaterializePlannerJSON("ws-B"))))

	const channelID = "C-WS"
	const threadTS = "1700000010.000000"
	d := model.NewCaseProposal(time.Now().UTC(), "U1")
	d.SelectedWorkspaceID = "ws-A"
	d.Materialization = &model.WorkspaceMaterialization{Title: "old", Description: "old desc"}
	d.Source = model.ProposalSource{ChannelID: channelID, ThreadTS: threadTS, MentionTS: threadTS}
	d.EphemeralChannelID = channelID
	d.EphemeralMessageTS = threadTS
	gt.NoError(t, repo.CaseProposal().Save(context.Background(), d)).Required()

	// Seed a Session for the thread; HandleSelectWorkspace looks it up to
	// pass into draft.UseCase.RunTurn.
	gt.NoError(t, repo.Session().Put(context.Background(), &model.Session{
		ID:            "ssn-test",
		ChannelID:     channelID,
		ThreadTS:      threadTS,
		CreatorUserID: "U1",
		ProposalID:    d.ID,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	})).Required()

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

	// One response_url POST for the lock-state UI.
	gt.Number(t, len(*captured)).GreaterOrEqual(1)
	// The post-planner preview is rendered via slackService.UpdateMessage
	// (chat.update against the ephemeral message TS).
	gt.Number(t, len(slackMock.updateBlockPosts)).GreaterOrEqual(1)

	got, err := repo.CaseProposal().Get(context.Background(), d.ID)
	gt.NoError(t, err).Required()
	gt.Value(t, got.SelectedWorkspaceID).Equal("ws-B")
	gt.Bool(t, got.InferenceInProgress).False()
	gt.Value(t, got.Materialization).NotNil().Required()
	gt.Value(t, got.Materialization.Title).Equal("AI suggested title")
}

// TestHandleEdit_OpensModalWithTriggerID locks the contract that the Edit
// button invokes views.open with the callback's trigger_id. The controller
// runs HandleEdit synchronously so the trigger_id is consumed inside Slack's
// ~3-second TTL window; dispatching this through async risks invalid_arguments
// once the goroutine is delayed by Firestore latency.
func TestHandleEdit_OpensModalWithTriggerID(t *testing.T) {
	repo := memory.New()
	schema := &config.FieldSchema{Fields: []config.FieldDefinition{
		{ID: "severity", Type: types.FieldTypeSelect,
			Options: []config.FieldOption{{ID: "low"}, {ID: "high"}}},
	}}
	registry := newRegistryWithSchema("ws-edit", "WS Edit", schema)
	slackMock := newCollectorOnlyMockSlack()
	uc := usecase.NewMentionProposalUseCase(repo, registry, slackMock, newDraftUC(t, repo, stubPlannerLLM(stubMaterializePlannerJSON("ws-edit"))))

	d := model.NewCaseProposal(time.Now().UTC(), "U1")
	d.SelectedWorkspaceID = "ws-edit"
	d.Materialization = &model.WorkspaceMaterialization{Title: "draft title", Description: "draft desc"}
	d.EphemeralChannelID = "C-EDIT"
	d.EphemeralMessageTS = "1700000020.000000"
	gt.NoError(t, repo.CaseProposal().Save(context.Background(), d)).Required()

	cb := &goslack.InteractionCallback{
		Type:      goslack.InteractionTypeBlockActions,
		TriggerID: "test-trigger-id-abc",
		User:      goslack.User{ID: "U1"},
		ActionCallback: goslack.ActionCallbacks{
			BlockActions: []*goslack.BlockAction{
				{ActionID: usecase.ActionIDDraftEdit, Value: string(d.ID)},
			},
		},
	}

	gt.NoError(t, uc.HandleEdit(context.Background(), cb, cb.ActionCallback.BlockActions[0])).Required()

	gt.Array(t, slackMock.openViewCalls).Length(1).Required()
	gt.String(t, slackMock.openViewCalls[0].triggerID).Equal("test-trigger-id-abc")
	gt.Value(t, slackMock.openViewCalls[0].view.Type).Equal(goslack.VTModal)
	gt.String(t, string(slackMock.openViewCalls[0].view.PrivateMetadata)).Contains(string(d.ID))
}

// TestHandleSubmit_RecordsReporter pins the reporter-recording contract
// for the preview "Submit" button on the Slack mention draft. Slack
// interactivity callbacks do not go through the Web auth middleware, so
// the handler MUST inject callback.User.ID as the auth-context Token
// before reaching CaseUseCase.CreateCase — otherwise the case lands
// with an empty ReporterID and the Cases page shows an empty Reporter
// column for every Slack-originated case. The original bug was
// exactly that: no test covered persisted ReporterID for this entry
// point.
func TestHandleSubmit_RecordsReporter(t *testing.T) {
	const reporterID = "U-MENTION-SUBMIT"
	repo := memory.New()
	registry := newRegistryWithSchema("ws-submit", "WS Submit", &config.FieldSchema{})
	slackMock := newCollectorOnlyMockSlack()
	mentionUC := usecase.NewMentionProposalUseCase(repo, registry, slackMock, newDraftUC(t, repo, stubPlannerLLM(stubMaterializePlannerJSON("ws-submit"))))
	caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")

	d := model.NewCaseProposal(time.Now().UTC(), reporterID)
	d.SelectedWorkspaceID = "ws-submit"
	d.Materialization = &model.WorkspaceMaterialization{
		Title:       "Submitted from mention",
		Description: "body",
	}
	d.EphemeralChannelID = "C-SUBMIT"
	d.EphemeralMessageTS = "1700000020.000000"
	gt.NoError(t, repo.CaseProposal().Save(context.Background(), d)).Required()

	respURL, _ := captureResponseURL(t)
	cb := &goslack.InteractionCallback{
		Type:        goslack.InteractionTypeBlockActions,
		ResponseURL: respURL,
		User:        goslack.User{ID: reporterID},
		Team:        goslack.Team{ID: "T-WS"},
		ActionCallback: goslack.ActionCallbacks{
			BlockActions: []*goslack.BlockAction{
				{ActionID: usecase.ActionIDDraftSubmit, Value: string(d.ID)},
			},
		},
	}

	gt.NoError(t, mentionUC.HandleSubmit(context.Background(), caseUC, cb, cb.ActionCallback.BlockActions[0])).Required()

	cases, err := repo.Case().List(context.Background(), "ws-submit")
	gt.NoError(t, err).Required()
	gt.Array(t, cases).Length(1).Required()
	gt.Value(t, cases[0].ReporterID).Equal(reporterID)
	gt.Value(t, cases[0].Title).Equal("Submitted from mention")
}

// TestHandleEditSubmit_RecordsReporter is the modal-submission twin of
// TestHandleSubmit_RecordsReporter. The mention-draft Edit modal also
// calls CaseUseCase.CreateCase from a Slack interactivity callback, so
// it shares the "no Web auth middleware → empty ReporterID" failure
// mode and needs the same explicit auth-context injection.
func TestHandleEditSubmit_RecordsReporter(t *testing.T) {
	const reporterID = "U-MENTION-EDIT-SUBMIT"
	repo := memory.New()
	registry := newRegistryWithSchema("ws-edit-submit", "WS Edit Submit", &config.FieldSchema{})
	slackMock := newCollectorOnlyMockSlack()
	mentionUC := usecase.NewMentionProposalUseCase(repo, registry, slackMock, newDraftUC(t, repo, stubPlannerLLM(stubMaterializePlannerJSON("ws-edit-submit"))))
	caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")

	d := model.NewCaseProposal(time.Now().UTC(), reporterID)
	d.SelectedWorkspaceID = "ws-edit-submit"
	d.Materialization = &model.WorkspaceMaterialization{Title: "Initial title", Description: "Initial body"}
	d.EphemeralChannelID = "C-EDIT-SUBMIT"
	d.EphemeralMessageTS = "1700000030.000000"
	gt.NoError(t, repo.CaseProposal().Save(context.Background(), d)).Required()

	meta, _ := json.Marshal(map[string]string{
		"workspace_id":         "ws-edit-submit",
		"proposal_id":          string(d.ID),
		"ephemeral_channel_id": d.EphemeralChannelID,
		"ephemeral_message_ts": d.EphemeralMessageTS,
	})

	cb := &goslack.InteractionCallback{
		Type: goslack.InteractionTypeViewSubmission,
		User: goslack.User{ID: reporterID},
		Team: goslack.Team{ID: "T-WS"},
		View: goslack.View{
			CallbackID:      usecase.SlackCallbackIDDraftEdit,
			PrivateMetadata: string(meta),
			State: &goslack.ViewState{
				Values: map[string]map[string]goslack.BlockAction{
					usecase.BlockIDDraftEditTitleForTest: {
						usecase.ActionIDDraftEditTitleForTest: {Value: "Edited title"},
					},
					usecase.BlockIDDraftEditDescriptionForTest: {
						usecase.ActionIDDraftEditDescriptionForTest: {Value: "Edited body"},
					},
				},
			},
		},
	}

	gt.NoError(t, mentionUC.HandleEditSubmit(context.Background(), caseUC, cb)).Required()

	cases, err := repo.Case().List(context.Background(), "ws-edit-submit")
	gt.NoError(t, err).Required()
	gt.Array(t, cases).Length(1).Required()
	gt.Value(t, cases[0].ReporterID).Equal(reporterID)
	gt.Value(t, cases[0].Title).Equal("Edited title")
}
