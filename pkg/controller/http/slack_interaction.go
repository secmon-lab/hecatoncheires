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

// SlackInteractionHandler handles Slack interactive component payloads (button clicks, modal submissions, etc.)
type SlackInteractionHandler struct {
	actionUC *usecase.ActionUseCase
	agentUC  *usecase.AgentUseCase
	slackUC  *usecase.SlackUseCases
	caseUC   *usecase.CaseUseCase
}

// NewSlackInteractionHandler creates a new Slack interaction handler
func NewSlackInteractionHandler(actionUC *usecase.ActionUseCase, agentUC *usecase.AgentUseCase) *SlackInteractionHandler {
	return &SlackInteractionHandler{
		actionUC: actionUC,
		agentUC:  agentUC,
	}
}

// WithSlackCommand configures the handler to process slash command modal submissions
func (h *SlackInteractionHandler) WithSlackCommand(slackUC *usecase.SlackUseCases, caseUC *usecase.CaseUseCase) {
	h.slackUC = slackUC
	h.caseUC = caseUC
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

	// Handle view_submission (modal form submissions)
	if callback.Type == slack.InteractionTypeViewSubmission {
		h.handleViewSubmission(w, r, &callback)
		return
	}

	// Only handle block_actions (button clicks) below
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

		case usecase.SlackAgentSessionActionsID:
			if h.agentUC == nil || action.SelectedOption.Value == "" {
				continue
			}
			actionType, data, err := usecase.ParseAgentActionValue(action.SelectedOption.Value)
			if err != nil {
				logger := logging.From(ctx)
				logger.Warn("failed to parse agent action value",
					"error", err,
					"value", action.SelectedOption.Value,
				)
				continue
			}
			switch actionType {
			case usecase.SlackAgentActionShowSessionInfo:
				if err := h.agentUC.HandleSessionInfoRequest(ctx, callback.TriggerID, data); err != nil {
					logger := logging.From(ctx)
					logger.Error("failed to handle session info request",
						"error", err,
						"session_id", data,
					)
				}
			}

		default:
			// Unknown action ID, skip
			continue
		}
	}

	w.WriteHeader(http.StatusOK)
}

// handleViewSubmission processes view_submission interaction callbacks
func (h *SlackInteractionHandler) handleViewSubmission(w http.ResponseWriter, r *http.Request, callback *slack.InteractionCallback) {
	ctx := r.Context()
	logger := logging.From(ctx)

	if h.slackUC == nil || h.caseUC == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch callback.View.CallbackID {
	case usecase.SlackCallbackIDSelectWorkspace:
		// Workspace selection → return updated view with case creation modal
		view, err := h.slackUC.HandleWorkspaceSelectSubmit(callback)
		if err != nil {
			logger.Error("failed to handle workspace selection",
				"error", err,
			)
			writeViewSubmissionError(w, "Failed to process workspace selection. Please try again.")
			return
		}

		// Respond with response_action: update to replace the modal
		resp := slack.ViewSubmissionResponse{
			ResponseAction: slack.RAUpdate,
			View:           view,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			logger.Error("failed to encode view submission response", "error", err)
		}

	case usecase.SlackCallbackIDCreateCase:
		// Case creation → create case and close modal
		if err := h.slackUC.HandleCaseCreationSubmit(ctx, h.caseUC, callback); err != nil {
			logger.Error("failed to handle case creation",
				"error", err,
			)
			writeViewSubmissionError(w, "Failed to create case. Please try again.")
			return
		}

		// Return empty 200 to close the modal
		w.WriteHeader(http.StatusOK)

	default:
		w.WriteHeader(http.StatusOK)
	}
}

// writeViewSubmissionError writes a view_submission error response that shows errors in the modal
func writeViewSubmissionError(w http.ResponseWriter, msg string) {
	resp := slack.ViewSubmissionResponse{
		ResponseAction: slack.RAErrors,
		Errors: map[string]string{
			"hc_case_title_block": msg,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
