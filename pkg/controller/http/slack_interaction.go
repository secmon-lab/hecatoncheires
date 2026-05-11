package http

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/slack-go/slack"
)

// SlackInteractionHandler handles Slack interactive component payloads (button clicks, modal submissions, etc.)
//
// All usecase dependencies are required at construction time; this handler is
// only wired when Slack webhooks are enabled, and at that point every Slack
// surface (action buttons, slash command modals, draft buttons) is in play.
// Optional wiring previously left fields nil and forced every entry point to
// guard against zero-value handlers — a fragile pattern that masked real wiring
// regressions behind silent skips.
type SlackInteractionHandler struct {
	actionUC       *usecase.ActionUseCase
	agentUC        *usecase.AgentUseCase
	slackUC        *usecase.SlackUseCases
	caseUC         *usecase.CaseUseCase
	mentionDraftUC *usecase.MentionDraftUseCase
}

// NewSlackInteractionHandler creates a new Slack interaction handler.
// Every dependency is mandatory.
func NewSlackInteractionHandler(
	actionUC *usecase.ActionUseCase,
	agentUC *usecase.AgentUseCase,
	slackUC *usecase.SlackUseCases,
	caseUC *usecase.CaseUseCase,
	mentionDraftUC *usecase.MentionDraftUseCase,
) *SlackInteractionHandler {
	return &SlackInteractionHandler{
		actionUC:       actionUC,
		agentUC:        agentUC,
		slackUC:        slackUC,
		caseUC:         caseUC,
		mentionDraftUC: mentionDraftUC,
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
				errutil.Handle(ctx, goerr.Wrap(err, "failed to parse status_select value", goerr.V("value", a.SelectedOption.Value)), "failed to parse status_select value")
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
			workspaceID, actionID, err := usecase.ParseSlackAssigneeBlockID(a.BlockID)
			if err != nil {
				errutil.Handle(ctx, goerr.Wrap(err, "failed to parse assignee block_id", goerr.V("block_id", a.BlockID)), "failed to parse assignee block_id")
				continue
			}
			input := usecase.UpdateActionInput{
				ID:                     actionID,
				SlackSync:              usecase.SlackSyncFull,
				Actor:                  usecase.ActorRef{Kind: usecase.ActorKindSlackUser, ID: cb.User.ID},
				RejectNonHumanAssignee: true,
			}
			if a.SelectedUser == "" {
				input.ClearAssignee = true
			} else {
				selected := a.SelectedUser
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
			if action.SelectedOption.Value == "" {
				continue
			}
			actionType, data, err := usecase.ParseAgentActionValue(action.SelectedOption.Value)
			if err != nil {
				errutil.Handle(ctx, goerr.Wrap(err, "failed to parse agent action value", goerr.V("value", action.SelectedOption.Value)), "failed to parse agent action value")
				continue
			}
			switch actionType {
			case usecase.SlackAgentActionShowSessionInfo:
				if err := h.agentUC.HandleSessionInfoRequest(ctx, callback.TriggerID, data); err != nil {
					errutil.Handle(ctx, goerr.Wrap(err, "failed to handle session info request", goerr.V("session_id", data)), "failed to handle session info request")
				}
			}

		case usecase.ActionIDDraftEdit:
			// trigger_id has a ~3 second TTL on Slack's side. Dispatching
			// views.open through async.Dispatch risks racing the TTL when
			// the goroutine is delayed by Firestore or other I/O, which
			// surfaces as invalid_arguments from views.open. Run the
			// trigger_id consuming path synchronously inside the 3-second
			// interactivity ack window.
			if err := h.mentionDraftUC.HandleEdit(ctx, &cb, a); err != nil {
				errutil.Handle(ctx, err, "failed to handle draft edit interaction")
			}

		case usecase.ActionIDDraftSelectWS,
			usecase.ActionIDDraftSubmit,
			usecase.ActionIDDraftCancel,
			usecase.ActionIDDraftQuestionSubmit:
			async.Dispatch(ctx, func(ctx context.Context) error {
				switch a.ActionID {
				case usecase.ActionIDDraftSelectWS:
					return h.mentionDraftUC.HandleSelectWorkspace(ctx, &cb, a)
				case usecase.ActionIDDraftSubmit:
					return h.mentionDraftUC.HandleSubmit(ctx, h.caseUC, &cb, a)
				case usecase.ActionIDDraftCancel:
					return h.mentionDraftUC.HandleCancel(ctx, &cb, a)
				case usecase.ActionIDDraftQuestionSubmit:
					return h.mentionDraftUC.HandleQuestionSubmit(ctx, &cb, a)
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

	switch callback.View.CallbackID {
	case usecase.SlackCallbackIDSelectWorkspace:
		// Workspace selection → return updated view with case creation modal
		view, err := h.slackUC.HandleWorkspaceSelectSubmit(r.Context(), callback)
		if err != nil {
			errutil.Handle(ctx, goerr.Wrap(err, "failed to handle workspace selection"), "failed to handle workspace selection")
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
			errutil.Handle(ctx, goerr.Wrap(err, "failed to encode view submission response"), "failed to encode view submission response")
		}

	case usecase.SlackCallbackIDCommandChoice:
		// Command choice (edit case vs create action) → swap to chosen modal
		view, err := h.slackUC.HandleCommandChoiceSubmit(r.Context(), callback)
		if err != nil {
			errutil.Handle(ctx, goerr.Wrap(err, "failed to handle command choice"), "failed to handle command choice")
			writeViewSubmissionError(ctx, w, usecase.SlackBlockIDCommandChoice, "Failed to process selection. Please try again.")
			return
		}
		resp := slack.ViewSubmissionResponse{
			ResponseAction: slack.RAUpdate,
			View:           view,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			errutil.Handle(ctx, goerr.Wrap(err, "failed to encode view submission response"), "failed to encode view submission response")
		}

	case usecase.SlackCallbackIDCreateAction:
		// Action creation modal submission → close modal, run async.
		w.WriteHeader(http.StatusOK)

		async.Dispatch(ctx, func(ctx context.Context) error {
			if err := h.slackUC.HandleActionCreationSubmit(ctx, h.actionUC, callback); err != nil {
				return goerr.Wrap(err, "failed to handle action creation submit")
			}
			return nil
		})

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
		errutil.Handle(ctx, goerr.Wrap(err, "failed to encode view submission error response"), "failed to encode view submission error response")
	}
}
