package http_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	httpctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/http"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// Export the private function for testing
var VerifySlackSignature = httpctrl.VerifySlackSignature

// computeSlackSignature computes the Slack signature for testing
func computeSlackSignature(signingSecret, timestamp, body string) string {
	baseString := fmt.Sprintf("v0:%s:%s", timestamp, body)
	h := hmac.New(sha256.New, []byte(signingSecret))
	h.Write([]byte(baseString))
	return "v0=" + hex.EncodeToString(h.Sum(nil))
}

// Test core signature verification function
func TestVerifySlackSignature(t *testing.T) {
	signingSecret := "test-signing-secret"
	body := []byte(`{"type":"url_verification","challenge":"test"}`)

	t.Run("valid signature", func(t *testing.T) {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		signature := computeSlackSignature(signingSecret, timestamp, string(body))

		err := VerifySlackSignature(signingSecret, timestamp, signature, body)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("invalid signature", func(t *testing.T) {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		invalidSignature := "v0=invalid_signature"

		err := VerifySlackSignature(signingSecret, timestamp, invalidSignature, body)
		if err == nil {
			t.Error("expected error for invalid signature, got nil")
		}
	})

	t.Run("missing timestamp", func(t *testing.T) {
		signature := computeSlackSignature(signingSecret, "123456", string(body))

		err := VerifySlackSignature(signingSecret, "", signature, body)
		if err == nil {
			t.Error("expected error for missing timestamp, got nil")
		}
	})

	t.Run("missing signature", func(t *testing.T) {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)

		err := VerifySlackSignature(signingSecret, timestamp, "", body)
		if err == nil {
			t.Error("expected error for missing signature, got nil")
		}
	})

	t.Run("timestamp too old", func(t *testing.T) {
		// Timestamp 10 minutes ago (should be rejected, limit is 5 minutes)
		oldTimestamp := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
		signature := computeSlackSignature(signingSecret, oldTimestamp, string(body))

		err := VerifySlackSignature(signingSecret, oldTimestamp, signature, body)
		if err == nil {
			t.Error("expected error for old timestamp, got nil")
		}
	})

	t.Run("invalid timestamp format", func(t *testing.T) {
		invalidTimestamp := "not-a-number"
		signature := computeSlackSignature(signingSecret, invalidTimestamp, string(body))

		err := VerifySlackSignature(signingSecret, invalidTimestamp, signature, body)
		if err == nil {
			t.Error("expected error for invalid timestamp format, got nil")
		}
	})

	t.Run("different secret produces different signature", func(t *testing.T) {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		signature := computeSlackSignature("wrong-secret", timestamp, string(body))

		err := VerifySlackSignature(signingSecret, timestamp, signature, body)
		if err == nil {
			t.Error("expected error when using wrong secret, got nil")
		}
	})

	t.Run("different body produces different signature", func(t *testing.T) {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		signature := computeSlackSignature(signingSecret, timestamp, "different body")

		err := VerifySlackSignature(signingSecret, timestamp, signature, body)
		if err == nil {
			t.Error("expected error when body doesn't match signature, got nil")
		}
	})
}

// Test middleware
func TestSlackSignatureMiddleware(t *testing.T) {
	signingSecret := "test-signing-secret"
	body := []byte(`{"type":"url_verification","challenge":"test"}`)

	t.Run("calls next handler when signature is valid", func(t *testing.T) {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		signature := computeSlackSignature(signingSecret, timestamp, string(body))

		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/event", bytes.NewReader(body))
		req.Header.Set("X-Slack-Request-Timestamp", timestamp)
		req.Header.Set("X-Slack-Signature", signature)

		rec := httptest.NewRecorder()

		nextCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusOK)
		})

		middleware := httpctrl.SlackSignatureMiddleware(signingSecret)
		middleware(nextHandler).ServeHTTP(rec, req)

		if !nextCalled {
			t.Error("expected next handler to be called, but it wasn't")
		}

		if rec.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
		}
	})

	t.Run("does not call next handler when signature is invalid", func(t *testing.T) {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		invalidSignature := "v0=invalid"

		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/event", bytes.NewReader(body))
		req.Header.Set("X-Slack-Request-Timestamp", timestamp)
		req.Header.Set("X-Slack-Signature", invalidSignature)

		rec := httptest.NewRecorder()

		nextCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusOK)
		})

		middleware := httpctrl.SlackSignatureMiddleware(signingSecret)
		middleware(nextHandler).ServeHTTP(rec, req)

		if nextCalled {
			t.Error("expected next handler NOT to be called, but it was")
		}

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
		}
	})

	t.Run("restores request body for next handler", func(t *testing.T) {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		signature := computeSlackSignature(signingSecret, timestamp, string(body))

		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/event", bytes.NewReader(body))
		req.Header.Set("X-Slack-Request-Timestamp", timestamp)
		req.Header.Set("X-Slack-Signature", signature)

		rec := httptest.NewRecorder()

		var receivedBody []byte
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var err error
			receivedBody, err = io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("failed to read body in next handler: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		})

		middleware := httpctrl.SlackSignatureMiddleware(signingSecret)
		middleware(nextHandler).ServeHTTP(rec, req)

		if string(receivedBody) != string(body) {
			t.Errorf("expected body %s, got %s", string(body), string(receivedBody))
		}
	})

	t.Run("handles different signing secrets", func(t *testing.T) {
		correctSecret := "correct-secret"
		wrongSecret := "wrong-secret"

		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		signature := computeSlackSignature(correctSecret, timestamp, string(body))

		req := httptest.NewRequest(http.MethodPost, "/hooks/slack/event", bytes.NewReader(body))
		req.Header.Set("X-Slack-Request-Timestamp", timestamp)
		req.Header.Set("X-Slack-Signature", signature)

		rec := httptest.NewRecorder()

		nextCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusOK)
		})

		// Use wrong secret - should fail
		middleware := httpctrl.SlackSignatureMiddleware(wrongSecret)
		middleware(nextHandler).ServeHTTP(rec, req)

		if nextCalled {
			t.Error("expected next handler NOT to be called with wrong secret, but it was")
		}

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
		}
	})
}

// Test webhook handler
func TestSlackWebhookHandler_URLVerification(t *testing.T) {
	signingSecret := "test-signing-secret"
	repo := memory.New()
	uc := usecase.New(repo)
	handler := httpctrl.NewSlackWebhookHandler(uc.Slack)

	challenge := "test-challenge-token"
	reqBody := map[string]interface{}{
		"type":      "url_verification",
		"challenge": challenge,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/hooks/slack/event", bytes.NewReader(body))
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature := computeSlackSignature(signingSecret, timestamp, string(body))

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Slack-Request-Timestamp", timestamp)
	req.Header.Set("X-Slack-Signature", signature)

	rec := httptest.NewRecorder()

	// Apply middleware and handler
	middlewareHandler := httpctrl.SlackSignatureMiddleware(signingSecret)(http.HandlerFunc(handler.ServeHTTP))
	middlewareHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	// Response should be the challenge as plain text
	respBody := rec.Body.String()
	if respBody != challenge {
		t.Errorf("expected challenge %s, got %s", challenge, respBody)
	}
}

func TestSlackWebhookHandler_MessageEvent(t *testing.T) {
	signingSecret := "test-signing-secret"
	repo := memory.New()
	uc := usecase.New(repo)
	handler := httpctrl.NewSlackWebhookHandler(uc.Slack)

	// Use raw JSON matching Slack's actual format
	reqBody := map[string]interface{}{
		"token":      "test-token",
		"team_id":    "T123",
		"api_app_id": "A123",
		"type":       "event_callback",
		"event": map[string]interface{}{
			"type":         "message",
			"user":         "U123",
			"text":         "Hello from test",
			"ts":           "1234567890.123456",
			"channel":      "C123",
			"event_ts":     "1234567890.123456",
			"channel_type": "channel",
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/hooks/slack/event", bytes.NewReader(body))
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature := computeSlackSignature(signingSecret, timestamp, string(body))

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Slack-Request-Timestamp", timestamp)
	req.Header.Set("X-Slack-Signature", signature)

	rec := httptest.NewRecorder()

	// Apply middleware and handler
	middlewareHandler := httpctrl.SlackSignatureMiddleware(signingSecret)(http.HandlerFunc(handler.ServeHTTP))
	middlewareHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	// Allow async processing to complete
	time.Sleep(100 * time.Millisecond)

	// Verify message was stored
	messages, _, err := repo.Slack().ListMessages(
		req.Context(),
		"C123",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(1*time.Hour),
		10,
		"",
	)
	if err != nil {
		t.Fatalf("failed to list messages: %v", err)
	}

	if len(messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(messages))
	}
}

func TestSlackWebhookHandler_InvalidSignature(t *testing.T) {
	signingSecret := "test-signing-secret"
	repo := memory.New()
	uc := usecase.New(repo)
	handler := httpctrl.NewSlackWebhookHandler(uc.Slack)

	reqBody := map[string]string{
		"type": "url_verification",
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/hooks/slack/event", bytes.NewReader(body))
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Slack-Request-Timestamp", timestamp)
	req.Header.Set("X-Slack-Signature", "v0=invalid_signature")

	rec := httptest.NewRecorder()

	// Apply middleware only
	middlewareHandler := httpctrl.SlackSignatureMiddleware(signingSecret)(http.HandlerFunc(handler.ServeHTTP))
	middlewareHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d for invalid signature, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestSlackWebhookHandler_TimestampTooOld(t *testing.T) {
	signingSecret := "test-signing-secret"
	repo := memory.New()
	uc := usecase.New(repo)
	handler := httpctrl.NewSlackWebhookHandler(uc.Slack)

	reqBody := map[string]string{
		"type": "url_verification",
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/hooks/slack/event", bytes.NewReader(body))
	// Timestamp 10 minutes ago (should be rejected)
	oldTimestamp := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	signature := computeSlackSignature(signingSecret, oldTimestamp, string(body))

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Slack-Request-Timestamp", oldTimestamp)
	req.Header.Set("X-Slack-Signature", signature)

	rec := httptest.NewRecorder()

	// Apply middleware only
	middlewareHandler := httpctrl.SlackSignatureMiddleware(signingSecret)(http.HandlerFunc(handler.ServeHTTP))
	middlewareHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d for old timestamp, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestSlackWebhookHandler_MissingHeaders(t *testing.T) {
	signingSecret := "test-signing-secret"
	repo := memory.New()
	uc := usecase.New(repo)
	handler := httpctrl.NewSlackWebhookHandler(uc.Slack)

	reqBody := map[string]string{
		"type": "url_verification",
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}

	tests := []struct {
		name         string
		setTimestamp bool
		setSignature bool
		expectedCode int
	}{
		{
			name:         "missing timestamp header",
			setTimestamp: false,
			setSignature: true,
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:         "missing signature header",
			setTimestamp: true,
			setSignature: false,
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:         "missing both headers",
			setTimestamp: false,
			setSignature: false,
			expectedCode: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/hooks/slack/event", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			if tt.setTimestamp {
				timestamp := strconv.FormatInt(time.Now().Unix(), 10)
				req.Header.Set("X-Slack-Request-Timestamp", timestamp)
			}

			if tt.setSignature {
				req.Header.Set("X-Slack-Signature", "v0=somesignature")
			}

			rec := httptest.NewRecorder()

			// Apply middleware only
			middlewareHandler := httpctrl.SlackSignatureMiddleware(signingSecret)(http.HandlerFunc(handler.ServeHTTP))
			middlewareHandler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedCode {
				t.Errorf("expected status %d, got %d", tt.expectedCode, rec.Code)
			}
		})
	}
}
