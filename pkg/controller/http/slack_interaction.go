package http

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/slack-go/slack"
)

// SlackInteractionHandler handles Slack interactive component payloads (button clicks, modal submissions, etc.)
type SlackInteractionHandler struct {
	actionUC       *usecase.ActionUseCase
	agentUC        *usecase.AgentUseCase
	slackUC        *usecase.SlackUseCases
	caseUC         *usecase.CaseUseCase
	mentionDraftUC *usecase.MentionDraftUseCase
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

// WithMentionDraft wires the MentionDraftUseCase so this handler dispatches
// the draft preview's button/select interactions and Edit modal submission.
func (h *SlackInteractionHandler) WithMentionDraft(uc *usecase.MentionDraftUseCase) {
	h.mentionDraftUC = uc
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
		a := action
		cb := callback
		// Only process our action IDs
		switch a.ActionID {
		case usecase.SlackActionIDStatusSelect:
			workspaceID, actionID, status, err := usecase.ParseSlackStatusSelectValue(a.SelectedOption.Value)
			if err != nil {
				logging.From(ctx).Warn("failed to parse status_select value",
					"error", err,
					"value", a.SelectedOption.Value,
				)
				continue
			}
			input := usecase.UpdateActionInput{
				ID:        actionID,
				Status:    &status,
				SlackSync: usecase.SlackSyncFull,
				Actor:     usecase.ActorRef{Kind: usecase.ActorKindSlackUser, ID: cb.User.ID},
			}
			async.Dispatch(ctx, func(ctx context.Context) error {
				if _, err := h.actionUC.UpdateAction(ctx, workspaceID, input); err != nil {
					return goerr.Wrap(err, "failed to update action status from Slack",
						goerr.V("workspace_id", workspaceID),
						goerr.V("action_id", actionID))
				}
				return nil
			})

		case usecase.SlackActionIDAssigneeSelect:
			workspaceID, actionID, selectedUserID, err := usecase.ParseSlackAssigneeSelectValue(a.SelectedOption.Value)
			if err != nil {
				logging.From(ctx).Warn("failed to parse assignee_select value",
					"error", err,
					"value", a.SelectedOption.Value,
				)
				continue
			}
			input := usecase.UpdateActionInput{
				ID:        actionID,
				SlackSync: usecase.SlackSyncFull,
				Actor:     usecase.ActorRef{Kind: usecase.ActorKindSlackUser, ID: cb.User.ID},
			}
			if selectedUserID == "" {
				input.ClearAssignee = true
			} else {
				selected := selectedUserID
				input.AssigneeID = &selected
			}
			async.Dispatch(ctx, func(ctx context.Context) error {
				if _, err := h.actionUC.UpdateAction(ctx, workspaceID, input); err != nil {
					return goerr.Wrap(err, "failed to update action assignee from Slack",
						goerr.V("workspace_id", workspaceID),
						goerr.V("action_id", actionID))
				}
				return nil
			})

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

		case usecase.ActionIDDraftSelectWS,
			usecase.ActionIDDraftSubmit,
			usecase.ActionIDDraftEdit,
			usecase.ActionIDDraftCancel:
			if h.mentionDraftUC == nil {
				continue
			}
			a := action
			cb := callback
			async.Dispatch(ctx, func(ctx context.Context) error {
				switch a.ActionID {
				case usecase.ActionIDDraftSelectWS:
					return h.mentionDraftUC.HandleSelectWorkspace(ctx, &cb, a)
				case usecase.ActionIDDraftSubmit:
					return h.mentionDraftUC.HandleSubmit(ctx, h.caseUC, &cb, a)
				case usecase.ActionIDDraftEdit:
					return h.mentionDraftUC.HandleEdit(ctx, &cb, a)
				case usecase.ActionIDDraftCancel:
					return h.mentionDraftUC.HandleCancel(ctx, &cb, a)
				}
				return nil
			})

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
		view, err := h.slackUC.HandleWorkspaceSelectSubmit(r.Context(), callback)
		if err != nil {
			logger.Error("failed to handle workspace selection",
				"error", err,
			)
			writeViewSubmissionError(ctx, w, usecase.SlackBlockIDWorkspaceSelect, "Failed to process workspace selection. Please try again.")
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
		// Return 200 immediately to close the modal, then process asynchronously
		w.WriteHeader(http.StatusOK)

		async.Dispatch(ctx, func(ctx context.Context) error {
			if err := h.slackUC.HandleCaseCreationSubmit(ctx, h.caseUC, callback); err != nil {
				return goerr.Wrap(err, "failed to handle case creation")
			}
			return nil
		})

	case usecase.SlackCallbackIDDraftEdit:
		// Draft Edit modal submission → close modal and create case asynchronously.
		w.WriteHeader(http.StatusOK)
		if h.mentionDraftUC == nil {
			return
		}
		async.Dispatch(ctx, func(ctx context.Context) error {
			if err := h.mentionDraftUC.HandleEditSubmit(ctx, h.caseUC, callback); err != nil {
				return goerr.Wrap(err, "failed to handle draft edit submit")
			}
			return nil
		})
		return

	case usecase.SlackCallbackIDEditCase:
		// Return 200 immediately to close the modal, then process asynchronously
		w.WriteHeader(http.StatusOK)

		async.Dispatch(ctx, func(ctx context.Context) error {
			if err := h.slackUC.HandleCaseEditSubmit(ctx, h.caseUC, callback); err != nil {
				return goerr.Wrap(err, "failed to handle case edit")
			}
			return nil
		})

	default:
		w.WriteHeader(http.StatusOK)
	}
}

// writeViewSubmissionError writes a view_submission error response that shows errors in the modal
func writeViewSubmissionError(ctx context.Context, w http.ResponseWriter, blockID string, msg string) {
	resp := slack.ViewSubmissionResponse{
		ResponseAction: slack.RAErrors,
		Errors: map[string]string{
			blockID: msg,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logging.From(ctx).Error("failed to encode view submission error response", "error", err)
	}
}
