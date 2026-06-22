package logging_test

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

func TestIsTerminal(t *testing.T) {
	t.Run("bytes.Buffer is not a TTY", func(t *testing.T) {
		gt.Bool(t, logging.IsTerminalForTest(&bytes.Buffer{})).False()
	})

	t.Run("io.Discard is not a TTY", func(t *testing.T) {
		gt.Bool(t, logging.IsTerminalForTest(io.Discard)).False()
	})
}

func TestNew_ConsoleGoerrWithoutStacktrace(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, slog.LevelDebug, logging.FormatConsole, false)

	err := goerr.New("boom", goerr.V("op", "ping"))
	logger.Info("something failed", slog.Any("err", err))

	out := buf.String()
	// goerr Values() entries are expanded under the "err" group.
	gt.String(t, out).Contains("err.op=")
	gt.String(t, out).Contains("ping")
	// Without stacktrace, hooks.GoErr() injects the error text under "message".
	gt.String(t, out).Contains("err.message=")
	gt.String(t, out).Contains("boom")
	// No deferred stacktrace block when stacktrace is disabled.
	gt.Bool(t, strings.Contains(out, "Error:")).False()
	// Output written to a non-TTY writer must not carry ANSI escapes.
	gt.Bool(t, strings.Contains(out, "\x1b[")).False()
}

func TestNew_ConsoleGoerrWithStacktrace(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, slog.LevelDebug, logging.FormatConsole, true)

	err := goerr.New("boom", goerr.V("op", "ping"))
	logger.Info("something failed", slog.Any("err", err))

	out := buf.String()
	// Values() entries still expand under the group.
	gt.String(t, out).Contains("err.op=")
	gt.String(t, out).Contains("ping")
	// With stacktrace, the message comes via the deferred "Error: ..." block.
	gt.String(t, out).Contains("Error:")
	gt.String(t, out).Contains("boom")
	// Stacktrace mode must not also leak the "message" key.
	gt.Bool(t, strings.Contains(out, "err.message=")).False()
}

func TestNew_ConsolePlainAttrUnaffected(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, slog.LevelDebug, logging.FormatConsole, false)

	logger.Info("hello", slog.String("user", "alice"))

	out := buf.String()
	gt.String(t, out).Contains("user=")
	gt.String(t, out).Contains("alice")
	gt.String(t, out).Contains("hello")
}

func TestNew_JSONFormatRedactsAuthorization(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, slog.LevelDebug, logging.FormatJSON, false)

	type payload struct {
		Authorization string
		User          string
	}
	logger.Info("req", slog.Any("payload", payload{Authorization: "secret-token", User: "alice"}))

	out := buf.String()
	// masq must redact the Authorization field.
	gt.Bool(t, strings.Contains(out, "secret-token")).False()
	gt.String(t, out).Contains("alice")
}

// TestNew_JSONFormatRedactsSecretTagAndHeaderMapKey covers the two redaction
// paths the MCP authorization input relies on: a `masq:"secret"` tagged field
// (the env allow-list) and an "Authorization" key inside a header map (which
// cannot carry a struct tag, so it is redacted via masq.WithFieldName).
func TestNew_JSONFormatRedactsSecretTagAndHeaderMapKey(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&buf, slog.LevelDebug, logging.FormatJSON, false)

	type input struct {
		Env    map[string]string   `masq:"secret"`
		Header map[string][]string `json:"header"`
		Tool   string              `json:"tool"`
	}
	logger.Info("mcp", slog.Any("input", input{
		Env:    map[string]string{"MCP_TOKEN": "env-secret-value"},
		Header: map[string][]string{"Authorization": {"Bearer header-secret-value"}},
		Tool:   "hecaton_list_cases",
	}))

	out := buf.String()
	// The secret-tagged env value must not appear in the log output.
	gt.Bool(t, strings.Contains(out, "env-secret-value")).False()
	// The Authorization header value (a map key, not a struct field) must be redacted.
	gt.Bool(t, strings.Contains(out, "header-secret-value")).False()
	// Non-sensitive fields are still logged.
	gt.String(t, out).Contains("hecaton_list_cases")
}
