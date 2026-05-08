package http_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	httpctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/http"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
	goslack "github.com/slack-go/slack"
)

func TestSlackInteractionHandler(t *testing.T) {
	setup := func(t *testing.T) (*usecase.ActionUseCase, *httpctrl.SlackInteractionHandler, int64) {
		t.Helper()
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")

		// Register the seed assignee in the SlackUser DB so UpdateAction's
		// bot-rejection guard accepts it.
		gt.NoError(t, repo.SlackUser().SaveMany(t.Context(), []*model.SlackUser{
			{ID: "U001", Name: "alice", RealName: "Alice"},
		})).Required()

		ctx := auth.ContextWithToken(t.Context(), &auth.Token{Sub: "UTESTUSER"})
		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Desc", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		action, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Desc", "U001", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		handler := httpctrl.NewSlackInteractionHandler(actionUC, nil)
		return actionUC, handler, action.ID
	}

	// setupWithUser is like setup but also registers extraUserID as a known
	// human in the SlackUser DB so the bot-rejection guard lets it through.
	setupWithUser := func(t *testing.T, extraUserID string) (*usecase.ActionUseCase, *httpctrl.SlackInteractionHandler, int64) {
		t.Helper()
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")

		gt.NoError(t, repo.SlackUser().SaveMany(t.Context(), []*model.SlackUser{
			{ID: "U001", Name: "alice", RealName: "Alice"},
			{ID: model.SlackUserID(extraUserID), Name: "extra", RealName: "Extra"},
		})).Required()

		ctx := auth.ContextWithToken(t.Context(), &auth.Token{Sub: "UTESTUSER"})
		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Desc", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		action, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Desc", "U001", "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		handler := httpctrl.NewSlackInteractionHandler(actionUC, nil)
		return actionUC, handler, action.ID
	}

	t.Run("handles status_select to IN_PROGRESS", func(t *testing.T) {
		actionUC, handler, actionID := setup(t)

		callback := goslack.InteractionCallback{
			Type: goslack.InteractionTypeBlockActions,
			User: goslack.User{ID: "U001"},
			ActionCallback: goslack.ActionCallbacks{
				BlockActions: []*goslack.BlockAction{
					{
						ActionID: usecase.SlackActionIDStatusSelect,
						BlockID:  "hc_action_status_block",
						SelectedOption: goslack.OptionBlockObject{
							Value: testWorkspaceID + ":" + itoa(actionID) + ":IN_PROGRESS",
						},
					},
				},
			},
		}
		payloadJSON, err := json.Marshal(callback)
		gt.NoError(t, err).Required()

		form := url.Values{"payload": {string(payloadJSON)}}
		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/interaction", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		gt.Value(t, rec.Code).Equal(http.StatusOK)
		async.Wait()

		action, err := actionUC.GetAction(t.Context(), testWorkspaceID, actionID)
		gt.NoError(t, err).Required()
		gt.Value(t, action.Status).Equal(types.ActionStatusInProgress)
	})

	t.Run("handles status_select to COMPLETED", func(t *testing.T) {
		actionUC, handler, actionID := setup(t)

		callback := goslack.InteractionCallback{
			Type: goslack.InteractionTypeBlockActions,
			User: goslack.User{ID: "U001"},
			ActionCallback: goslack.ActionCallbacks{
				BlockActions: []*goslack.BlockAction{
					{
						ActionID: usecase.SlackActionIDStatusSelect,
						BlockID:  "hc_action_status_block",
						SelectedOption: goslack.OptionBlockObject{
							Value: testWorkspaceID + ":" + itoa(actionID) + ":COMPLETED",
						},
					},
				},
			},
		}
		payloadJSON, err := json.Marshal(callback)
		gt.NoError(t, err).Required()

		form := url.Values{"payload": {string(payloadJSON)}}
		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/interaction", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		gt.Value(t, rec.Code).Equal(http.StatusOK)
		async.Wait()

		action, err := actionUC.GetAction(t.Context(), testWorkspaceID, actionID)
		gt.NoError(t, err).Required()
		gt.Value(t, action.Status).Equal(types.ActionStatusCompleted)
	})

	t.Run("handles users_select to set assignee", func(t *testing.T) {
		actionUC, handler, actionID := setupWithUser(t, "U999")

		blockID := usecase.SlackActionAssigneeBlockID(testWorkspaceID, actionID)
		callback := goslack.InteractionCallback{
			Type: goslack.InteractionTypeBlockActions,
			User: goslack.User{ID: "U001"},
			ActionCallback: goslack.ActionCallbacks{
				BlockActions: []*goslack.BlockAction{
					{
						ActionID:     usecase.SlackActionIDAssigneeSelect,
						BlockID:      blockID,
						SelectedUser: "U999",
					},
				},
			},
		}
		payloadJSON, err := json.Marshal(callback)
		gt.NoError(t, err).Required()

		form := url.Values{"payload": {string(payloadJSON)}}
		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/interaction", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		gt.Value(t, rec.Code).Equal(http.StatusOK)
		async.Wait()

		action, err := actionUC.GetAction(t.Context(), testWorkspaceID, actionID)
		gt.NoError(t, err).Required()
		gt.Value(t, action.AssigneeID).Equal("U999")
	})

	t.Run("handles users_select with empty user (clears assignee)", func(t *testing.T) {
		actionUC, handler, actionID := setup(t)

		blockID := usecase.SlackActionAssigneeBlockID(testWorkspaceID, actionID)
		callback := goslack.InteractionCallback{
			Type: goslack.InteractionTypeBlockActions,
			User: goslack.User{ID: "U001"},
			ActionCallback: goslack.ActionCallbacks{
				BlockActions: []*goslack.BlockAction{
					{
						ActionID:     usecase.SlackActionIDAssigneeSelect,
						BlockID:      blockID,
						SelectedUser: "",
					},
				},
			},
		}
		payloadJSON, err := json.Marshal(callback)
		gt.NoError(t, err).Required()

		form := url.Values{"payload": {string(payloadJSON)}}
		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/interaction", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		gt.Value(t, rec.Code).Equal(http.StatusOK)
		async.Wait()

		action, err := actionUC.GetAction(t.Context(), testWorkspaceID, actionID)
		gt.NoError(t, err).Required()
		gt.Value(t, action.AssigneeID).Equal("")
	})

	t.Run("rejects bot / unknown user assignee", func(t *testing.T) {
		actionUC, handler, actionID := setup(t)

		blockID := usecase.SlackActionAssigneeBlockID(testWorkspaceID, actionID)
		callback := goslack.InteractionCallback{
			Type: goslack.InteractionTypeBlockActions,
			User: goslack.User{ID: "U001"},
			ActionCallback: goslack.ActionCallbacks{
				BlockActions: []*goslack.BlockAction{
					{
						ActionID:     usecase.SlackActionIDAssigneeSelect,
						BlockID:      blockID,
						SelectedUser: "BBOT123", // not in SlackUser DB
					},
				},
			},
		}
		payloadJSON, err := json.Marshal(callback)
		gt.NoError(t, err).Required()

		form := url.Values{"payload": {string(payloadJSON)}}
		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/interaction", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		gt.Value(t, rec.Code).Equal(http.StatusOK)
		async.Wait()

		// Assignee must remain unchanged because the selected user is not
		// in the SlackUser DB (treated as a bot / unknown).
		action, err := actionUC.GetAction(t.Context(), testWorkspaceID, actionID)
		gt.NoError(t, err).Required()
		gt.Value(t, action.AssigneeID).Equal("U001")
	})

	t.Run("returns 400 for missing payload", func(t *testing.T) {
		_, handler, _ := setup(t)

		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/interaction", strings.NewReader(""))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		gt.Value(t, rec.Code).Equal(http.StatusBadRequest)
	})

	t.Run("returns 400 for invalid JSON payload", func(t *testing.T) {
		_, handler, _ := setup(t)

		form := url.Values{"payload": {"invalid json"}}
		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/interaction", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		gt.Value(t, rec.Code).Equal(http.StatusBadRequest)
	})

	t.Run("ignores non-block_actions interactions", func(t *testing.T) {
		_, handler, _ := setup(t)

		callback := goslack.InteractionCallback{
			Type: goslack.InteractionTypeViewSubmission,
		}
		payloadJSON, err := json.Marshal(callback)
		gt.NoError(t, err).Required()

		form := url.Values{"payload": {string(payloadJSON)}}
		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/interaction", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		gt.Value(t, rec.Code).Equal(http.StatusOK)
	})

	t.Run("ignores unknown action IDs", func(t *testing.T) {
		actionUC, handler, actionID := setup(t)

		callback := goslack.InteractionCallback{
			Type: goslack.InteractionTypeBlockActions,
			User: goslack.User{ID: "U001"},
			ActionCallback: goslack.ActionCallbacks{
				BlockActions: []*goslack.BlockAction{
					{
						ActionID: "unknown_action_id",
						Value:    "test-ws:" + itoa(actionID),
					},
				},
			},
		}
		payloadJSON, err := json.Marshal(callback)
		gt.NoError(t, err).Required()

		form := url.Values{"payload": {string(payloadJSON)}}
		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/interaction", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		gt.Value(t, rec.Code).Equal(http.StatusOK)

		// Action should be unchanged
		action, err := actionUC.GetAction(t.Context(), testWorkspaceID, actionID)
		gt.NoError(t, err).Required()
		gt.Value(t, action.Status).Equal(types.ActionStatusTodo)
	})
}

func TestSlackInteractionHandler_AgentSessionActions(t *testing.T) {
	t.Run("handles show_session_info overflow option", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		// agentUC is nil here; the handler should handle this gracefully
		handler := httpctrl.NewSlackInteractionHandler(actionUC, nil)

		callback := goslack.InteractionCallback{
			Type: goslack.InteractionTypeBlockActions,
			User: goslack.User{ID: "U001"},
			ActionCallback: goslack.ActionCallbacks{
				BlockActions: []*goslack.BlockAction{
					{
						ActionID: usecase.SlackAgentSessionActionsID,
						SelectedOption: goslack.OptionBlockObject{
							Value: usecase.SlackAgentActionShowSessionInfo + ":test-session-uuid",
						},
					},
				},
			},
			TriggerID: "trigger-abc",
		}
		payloadJSON, err := json.Marshal(callback)
		gt.NoError(t, err).Required()

		form := url.Values{"payload": {string(payloadJSON)}}
		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/interaction", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		// Should return 200 even though agentUC is nil (logged error, not HTTP error)
		gt.Value(t, rec.Code).Equal(http.StatusOK)
	})
}

func TestSlackInteractionHandler_ViewSubmission(t *testing.T) {
	t.Run("handles workspace select submission with response_action update", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, &mockSlackServiceForCommand{})
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")

		handler := httpctrl.NewSlackInteractionHandler(actionUC, nil)
		handler.WithSlackCommand(slackUC, caseUC)

		meta, _ := json.Marshal(map[string]string{"channel_id": "C001"})
		callback := goslack.InteractionCallback{
			Type: goslack.InteractionTypeViewSubmission,
			View: goslack.View{
				CallbackID:      usecase.SlackCallbackIDSelectWorkspace,
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						"hc_ws_select_block": {
							"hc_ws_radio": {
								SelectedOption: goslack.OptionBlockObject{Value: "risk"},
							},
						},
					},
				},
			},
		}
		payloadJSON, err := json.Marshal(callback)
		gt.NoError(t, err).Required()

		form := url.Values{"payload": {string(payloadJSON)}}
		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/interaction", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		gt.Value(t, rec.Code).Equal(http.StatusOK)

		// Response should be JSON with response_action: update
		var resp struct {
			ResponseAction string `json:"response_action"`
			View           struct {
				CallbackID string `json:"callback_id"`
			} `json:"view"`
		}
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		gt.NoError(t, err).Required()
		gt.Value(t, resp.ResponseAction).Equal("update")
		gt.Value(t, resp.View.CallbackID).Equal(usecase.SlackCallbackIDCreateCase)
	})

	t.Run("handles case creation submission", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, &mockSlackServiceForCommand{})
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")

		handler := httpctrl.NewSlackInteractionHandler(actionUC, nil)
		handler.WithSlackCommand(slackUC, caseUC)

		meta, _ := json.Marshal(map[string]string{
			"workspace_id": "risk",
			"channel_id":   "C001",
		})
		callback := goslack.InteractionCallback{
			Type: goslack.InteractionTypeViewSubmission,
			User: goslack.User{ID: "U001"},
			View: goslack.View{
				CallbackID:      usecase.SlackCallbackIDCreateCase,
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						"hc_case_title_block": {
							"hc_case_title": {Value: "New Case from Slash Command"},
						},
						"hc_case_desc_block": {
							"hc_case_desc": {Value: "Created via slash command"},
						},
					},
				},
			},
		}
		payloadJSON, err := json.Marshal(callback)
		gt.NoError(t, err).Required()

		form := url.Values{"payload": {string(payloadJSON)}}
		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/interaction", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		gt.Value(t, rec.Code).Equal(http.StatusOK)

		// Case creation is async (via async.Dispatch), wait for it to complete
		var cases []*model.Case
		for range 50 {
			time.Sleep(10 * time.Millisecond)
			cases, err = repo.Case().List(t.Context(), "risk")
			if err == nil && len(cases) > 0 {
				break
			}
		}
		gt.NoError(t, err).Required()
		gt.Array(t, cases).Length(1)
		gt.Value(t, cases[0].Title).Equal("New Case from Slash Command")
	})

	t.Run("handles case edit submission", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, &mockSlackServiceForCommand{})
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")

		// Create an existing case
		ctx := auth.ContextWithToken(t.Context(), &auth.Token{Sub: "UTESTUSER"})
		created, err := caseUC.CreateCase(ctx, "risk", "Original Title", "Original desc", []string{}, nil, false, "", "")
		gt.NoError(t, err).Required()

		handler := httpctrl.NewSlackInteractionHandler(actionUC, nil)
		handler.WithSlackCommand(slackUC, caseUC)

		meta, _ := json.Marshal(map[string]any{
			"workspace_id": "risk",
			"channel_id":   "C-EDIT",
			"case_id":      created.ID,
		})
		callback := goslack.InteractionCallback{
			Type: goslack.InteractionTypeViewSubmission,
			User: goslack.User{ID: "U001"},
			View: goslack.View{
				CallbackID:      usecase.SlackCallbackIDEditCase,
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						"hc_case_title_block": {
							"hc_case_title": {Value: "Updated Title via Interaction"},
						},
						"hc_case_desc_block": {
							"hc_case_desc": {Value: "Updated desc"},
						},
					},
				},
			},
		}
		payloadJSON, err := json.Marshal(callback)
		gt.NoError(t, err).Required()

		form := url.Values{"payload": {string(payloadJSON)}}
		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/interaction", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		gt.Value(t, rec.Code).Equal(http.StatusOK)

		// Case edit is async (via async.Dispatch), wait for it to complete
		var updated *model.Case
		for range 50 {
			time.Sleep(10 * time.Millisecond)
			updated, err = repo.Case().Get(t.Context(), "risk", created.ID)
			if err == nil && updated.Title == "Updated Title via Interaction" {
				break
			}
		}
		gt.NoError(t, err).Required()
		gt.Value(t, updated.Title).Equal("Updated Title via Interaction")
		gt.Value(t, updated.Description).Equal("Updated desc")
	})

	t.Run("handles command choice submission with response_action update", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})
		actionUC := usecase.NewActionUseCase(repo, registry, nil, "")
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, &mockSlackServiceForCommand{})
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")

		// Seed a case so HandleCommandChoiceSubmit can resolve it.
		created, err := repo.Case().Create(t.Context(), "risk", &model.Case{Title: "Existing"})
		gt.NoError(t, err).Required()

		handler := httpctrl.NewSlackInteractionHandler(actionUC, nil)
		handler.WithSlackCommand(slackUC, caseUC)

		meta, _ := json.Marshal(map[string]any{
			"workspace_id": "risk",
			"channel_id":   "C-CASE",
			"case_id":      created.ID,
		})
		callback := goslack.InteractionCallback{
			Type: goslack.InteractionTypeViewSubmission,
			User: goslack.User{ID: "U001"},
			View: goslack.View{
				CallbackID:      usecase.SlackCallbackIDCommandChoice,
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						usecase.SlackBlockIDCommandChoice: {
							usecase.SlackActionIDCommandChoice: {
								SelectedOption: goslack.OptionBlockObject{Value: "create_action"},
							},
						},
					},
				},
			},
		}
		payloadJSON, err := json.Marshal(callback)
		gt.NoError(t, err).Required()

		form := url.Values{"payload": {string(payloadJSON)}}
		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/interaction", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		gt.Value(t, rec.Code).Equal(http.StatusOK)

		var resp struct {
			ResponseAction string `json:"response_action"`
			View           struct {
				CallbackID string `json:"callback_id"`
			} `json:"view"`
		}
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		gt.NoError(t, err).Required()
		gt.Value(t, resp.ResponseAction).Equal("update")
		gt.Value(t, resp.View.CallbackID).Equal(usecase.SlackCallbackIDCreateAction)
	})

	t.Run("handles action creation submission asynchronously", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "risk", Name: "Risk Management"},
		})
		mockSvc := &mockSlackServiceForCommand{}
		actionUC := usecase.NewActionUseCase(repo, registry, mockSvc, "")
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, mockSvc)
		caseUC := usecase.NewCaseUseCase(repo, registry, nil, nil, "")

		created, err := repo.Case().Create(t.Context(), "risk", &model.Case{Title: "Parent"})
		gt.NoError(t, err).Required()

		handler := httpctrl.NewSlackInteractionHandler(actionUC, nil)
		handler.WithSlackCommand(slackUC, caseUC)

		meta, _ := json.Marshal(map[string]any{
			"workspace_id": "risk",
			"channel_id":   "C-CASE",
			"case_id":      created.ID,
		})
		callback := goslack.InteractionCallback{
			Type: goslack.InteractionTypeViewSubmission,
			User: goslack.User{ID: "U001"},
			View: goslack.View{
				CallbackID:      usecase.SlackCallbackIDCreateAction,
				PrivateMetadata: string(meta),
				State: &goslack.ViewState{
					Values: map[string]map[string]goslack.BlockAction{
						usecase.SlackBlockIDActionTitle: {
							usecase.SlackActionIDActionTitle: {Value: "Async Action"},
						},
						usecase.SlackBlockIDActionStatusInput: {
							usecase.SlackActionIDActionStatusIn: {
								Type:           "static_select",
								SelectedOption: goslack.OptionBlockObject{Value: "TODO"},
							},
						},
					},
				},
			},
		}
		payloadJSON, err := json.Marshal(callback)
		gt.NoError(t, err).Required()

		form := url.Values{"payload": {string(payloadJSON)}}
		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/interaction", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		gt.Value(t, rec.Code).Equal(http.StatusOK)

		// Async tail completes via async.Dispatch.
		async.Wait()

		actions, err := repo.Action().GetByCase(t.Context(), "risk", created.ID, interfaces.ActionListOptions{})
		gt.NoError(t, err).Required()
		gt.Array(t, actions).Length(1).Required()
		gt.Value(t, actions[0].Title).Equal("Async Action")
	})

	t.Run("view_submission with no slackUC configured returns 200", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo, nil, nil, "")
		handler := httpctrl.NewSlackInteractionHandler(actionUC, nil)
		// slackUC not configured

		callback := goslack.InteractionCallback{
			Type: goslack.InteractionTypeViewSubmission,
			View: goslack.View{
				CallbackID: usecase.SlackCallbackIDCreateCase,
			},
		}
		payloadJSON, err := json.Marshal(callback)
		gt.NoError(t, err).Required()

		form := url.Values{"payload": {string(payloadJSON)}}
		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/interaction", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		gt.Value(t, rec.Code).Equal(http.StatusOK)
	})
}

func itoa(n int64) string {
	return fmt.Sprintf("%d", n)
}
