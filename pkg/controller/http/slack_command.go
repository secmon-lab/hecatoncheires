package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// SlackCommandHandler handles Slack slash command requests
type SlackCommandHandler struct {
	slackUC *usecase.SlackUseCases
}

// NewSlackCommandHandler creates a new Slack command handler
func NewSlackCommandHandler(slackUC *usecase.SlackUseCases) *SlackCommandHandler {
	return &SlackCommandHandler{
		slackUC: slackUC,
	}
}

// ServeHTTP handles Slack slash command webhook requests.
// It supports both /hooks/slack/command and /hooks/slack/command/{ws_id} paths.
func (h *SlackCommandHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	triggerID := r.FormValue("trigger_id")
	userID := r.FormValue("user_id")
	channelID := r.FormValue("channel_id")
	sourceTeamID := r.FormValue("team_id")
	workspaceID := chi.URLParam(r, "ws_id")

	if triggerID == "" {
		http.Error(w, "missing trigger_id", http.StatusBadRequest)
		return
	}

	if err := h.slackUC.HandleSlashCommand(ctx, triggerID, userID, channelID, workspaceID, sourceTeamID); err != nil {
		logger := logging.From(ctx)
		logger.Error("failed to handle slash command",
			"error", err,
			"user_id", userID,
			"channel_id", channelID,
			"workspace_id", workspaceID,
			"source_team_id", sourceTeamID,
		)
		// Return error text as ephemeral message to the user
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Failed to open case creation dialog. Please try again."))
		return
	}

	w.WriteHeader(http.StatusOK)
}
