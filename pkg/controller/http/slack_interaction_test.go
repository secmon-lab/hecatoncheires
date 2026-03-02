package http_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	httpctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/http"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	goslack "github.com/slack-go/slack"
)

func TestSlackInteractionHandler(t *testing.T) {
	setup := func(t *testing.T) (*usecase.ActionUseCase, *httpctrl.SlackInteractionHandler, int64) {
		t.Helper()
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")

		ctx := auth.ContextWithToken(t.Context(), &auth.Token{Sub: "UTESTUSER"})
		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Desc", []string{}, nil, false)
		gt.NoError(t, err).Required()

		action, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Desc", []string{"U001"}, "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		handler := httpctrl.NewSlackInteractionHandler(actionUC, nil)
		return actionUC, handler, action.ID
	}

	t.Run("handles assign button click", func(t *testing.T) {
		actionUC, handler, actionID := setup(t)

		callback := goslack.InteractionCallback{
			Type: goslack.InteractionTypeBlockActions,
			User: goslack.User{ID: "UNEW"},
			ActionCallback: goslack.ActionCallbacks{
				BlockActions: []*goslack.BlockAction{
					{
						ActionID: usecase.SlackActionIDAssign,
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

		// Verify assignee was added
		action, err := actionUC.GetAction(t.Context(), testWorkspaceID, actionID)
		gt.NoError(t, err).Required()
		gt.Array(t, action.AssigneeIDs).Length(2)
		gt.Value(t, action.AssigneeIDs[1]).Equal("UNEW")
	})

	t.Run("handles in_progress button click", func(t *testing.T) {
		actionUC, handler, actionID := setup(t)

		callback := goslack.InteractionCallback{
			Type: goslack.InteractionTypeBlockActions,
			User: goslack.User{ID: "U001"},
			ActionCallback: goslack.ActionCallbacks{
				BlockActions: []*goslack.BlockAction{
					{
						ActionID: usecase.SlackActionIDInProgress,
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

		action, err := actionUC.GetAction(t.Context(), testWorkspaceID, actionID)
		gt.NoError(t, err).Required()
		gt.Value(t, action.Status).Equal(types.ActionStatusInProgress)
	})

	t.Run("handles complete button click", func(t *testing.T) {
		actionUC, handler, actionID := setup(t)

		callback := goslack.InteractionCallback{
			Type: goslack.InteractionTypeBlockActions,
			User: goslack.User{ID: "U001"},
			ActionCallback: goslack.ActionCallbacks{
				BlockActions: []*goslack.BlockAction{
					{
						ActionID: usecase.SlackActionIDComplete,
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

		action, err := actionUC.GetAction(t.Context(), testWorkspaceID, actionID)
		gt.NoError(t, err).Required()
		gt.Value(t, action.Status).Equal(types.ActionStatusCompleted)
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
		actionUC := usecase.NewActionUseCase(repo, nil, "")
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

func itoa(n int64) string {
	return fmt.Sprintf("%d", n)
}
