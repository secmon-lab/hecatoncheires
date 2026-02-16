package http

import (
	"encoding/json"
	"net/http"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/slack-go/slack"
)

// SlackInteractionHandler handles Slack interactive component payloads (button clicks, etc.)
type SlackInteractionHandler struct {
	actionUC *usecase.ActionUseCase
}

// NewSlackInteractionHandler creates a new Slack interaction handler
func NewSlackInteractionHandler(actionUC *usecase.ActionUseCase) *SlackInteractionHandler {
	return &SlackInteractionHandler{
		actionUC: actionUC,
	}
}

// ServeHTTP handles Slack interaction webhook requests
func (h *SlackInteractionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Slack sends interaction payloads as application/x-www-form-urlencoded
	// with a "payload" field containing JSON
	payload := r.FormValue("payload")
	if payload == "" {
		errutil.HandleHTTP(ctx, w, goerr.New("missing payload field in interaction request"), http.StatusBadRequest)
		return
	}

	var callback slack.InteractionCallback
	if err := json.Unmarshal([]byte(payload), &callback); err != nil {
		errutil.HandleHTTP(ctx, w, goerr.Wrap(err, "failed to parse interaction payload"), http.StatusBadRequest)
		return
	}

	// Only handle block_actions (button clicks)
	if callback.Type != slack.InteractionTypeBlockActions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Process each action in the callback
	for _, action := range callback.ActionCallback.BlockActions {
		// Only process our action IDs
		switch action.ActionID {
		case usecase.SlackActionIDAssign, usecase.SlackActionIDInProgress, usecase.SlackActionIDComplete:
			// Parse the button value to get workspaceID and actionID
			workspaceID, actionID, err := usecase.ParseSlackActionValue(action.Value)
			if err != nil {
				logger := logging.From(ctx)
				logger.Warn("failed to parse Slack action value",
					"error", err,
					"value", action.Value,
				)
				continue
			}

			userID := callback.User.ID
			if err := h.actionUC.HandleSlackInteraction(ctx, workspaceID, actionID, userID, action.ActionID); err != nil {
				logger := logging.From(ctx)
				logger.Error("failed to handle Slack interaction",
					"error", err,
					"action_id", action.ActionID,
					"workspace_id", workspaceID,
					"action_id_num", actionID,
					"user_id", userID,
				)
			}

		default:
			// Unknown action ID, skip
			continue
		}
	}

	w.WriteHeader(http.StatusOK)
}
