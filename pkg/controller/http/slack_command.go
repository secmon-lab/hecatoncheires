package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
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
	text := r.FormValue("text")
	workspaceID := chi.URLParam(r, "ws_id")

	if triggerID == "" {
		http.Error(w, "missing trigger_id", http.StatusBadRequest)
		return
	}

	if err := h.slackUC.HandleSlashCommand(ctx, triggerID, userID, channelID, workspaceID, sourceTeamID, text); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to handle slash command",
			goerr.V("user_id", userID),
			goerr.V("channel_id", channelID),
			goerr.V("workspace_id", workspaceID),
			goerr.V("source_team_id", sourceTeamID),
		), "failed to handle slash command")
		// Return error text as ephemeral message to the user
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Failed to open case creation dialog. Please try again."))
		return
	}

	w.WriteHeader(http.StatusOK)
}
