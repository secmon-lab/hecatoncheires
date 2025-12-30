package http

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/slack-go/slack/slackevents"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const slackBodyKey contextKey = "slack_body"

// verifySlackSignature verifies the Slack request signature
// This is a pure function that can be used independently for testing
func verifySlackSignature(signingSecret, timestamp, signature string, body []byte) error {
	if timestamp == "" {
		return goerr.New("missing timestamp")
	}

	if signature == "" {
		return goerr.New("missing signature")
	}

	// Check timestamp to prevent replay attacks (within 5 minutes)
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return goerr.Wrap(err, "invalid timestamp")
	}

	now := time.Now().Unix()
	if now-ts > 60*5 {
		return goerr.New("timestamp too old", goerr.V("timestamp", timestamp), goerr.V("now", now))
	}

	// Compute expected signature
	baseString := fmt.Sprintf("v0:%s:%s", timestamp, body)
	mac := hmac.New(sha256.New, []byte(signingSecret))
	if _, err := mac.Write([]byte(baseString)); err != nil {
		return goerr.Wrap(err, "failed to compute HMAC")
	}
	expectedSignature := "v0=" + hex.EncodeToString(mac.Sum(nil))

	// Compare signatures
	if !hmac.Equal([]byte(expectedSignature), []byte(signature)) {
		return goerr.New("signature mismatch")
	}

	return nil
}

// SlackSignatureMiddleware creates a middleware that verifies Slack request signatures
func SlackSignatureMiddleware(signingSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Read body
			body, err := io.ReadAll(r.Body)
			if err != nil {
				errutil.HandleHTTP(ctx, w, goerr.Wrap(err, "failed to read request body"), http.StatusBadRequest)
				return
			}
			defer func() {
				if err := r.Body.Close(); err != nil {
					logger := logging.From(ctx)
					logger.Error("failed to close request body", "error", err)
				}
			}()

			// Get headers
			timestamp := r.Header.Get("X-Slack-Request-Timestamp")
			signature := r.Header.Get("X-Slack-Signature")

			// Verify signature
			if err := verifySlackSignature(signingSecret, timestamp, signature, body); err != nil {
				errutil.HandleHTTP(ctx, w, goerr.Wrap(err, "slack signature verification failed"), http.StatusUnauthorized)
				return
			}

			// Store body in context for later use and restore it to the request
			ctx = context.WithValue(ctx, slackBodyKey, body)
			r.Body = io.NopCloser(bytes.NewBuffer(body))

			// Call next handler
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SlackWebhookHandler handles Slack Events API webhook requests
type SlackWebhookHandler struct {
	slackUC *usecase.SlackUseCases
}

// NewSlackWebhookHandler creates a new Slack webhook handler
func NewSlackWebhookHandler(slackUC *usecase.SlackUseCases) *SlackWebhookHandler {
	return &SlackWebhookHandler{
		slackUC: slackUC,
	}
}

// ServeHTTP handles Slack webhook requests
func (h *SlackWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Read body (already verified by middleware)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		errutil.HandleHTTP(ctx, w, goerr.Wrap(err, "failed to read request body"), http.StatusBadRequest)
		return
	}

	// Parse event
	eventsAPIEvent, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
	if err != nil {
		errutil.HandleHTTP(ctx, w, goerr.Wrap(err, "failed to parse slack event"), http.StatusBadRequest)
		return
	}

	// Handle different event types
	switch eventsAPIEvent.Type {
	case slackevents.URLVerification:
		// URL Verification challenge
		var r *slackevents.ChallengeResponse
		if err := json.Unmarshal(body, &r); err != nil {
			errutil.HandleHTTP(ctx, w, goerr.Wrap(err, "failed to unmarshal challenge"), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(r.Challenge)); err != nil {
			logger := logging.From(ctx)
			logger.Error("failed to write challenge response", "error", err)
		}
		return

	case slackevents.CallbackEvent:
		// Return 200 immediately to satisfy Slack's 3-second timeout requirement
		w.WriteHeader(http.StatusOK)

		// Process event asynchronously
		async.Dispatch(ctx, func(ctx context.Context) error {
			logger := logging.From(ctx)
			logger.Info("processing slack callback event",
				"type", eventsAPIEvent.Type,
				"team_id", eventsAPIEvent.TeamID,
			)

			if err := h.slackUC.HandleSlackEvent(ctx, &eventsAPIEvent); err != nil {
				return goerr.Wrap(err, "failed to handle slack event")
			}

			return nil
		})

	default:
		// Unknown event type, log and return 200
		logger := logging.From(ctx)
		logger.Warn("unknown slack event type", "type", eventsAPIEvent.Type)
		w.WriteHeader(http.StatusOK)
	}
}
