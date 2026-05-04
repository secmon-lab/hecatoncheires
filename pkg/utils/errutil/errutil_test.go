package errutil_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// newCapturingCtx returns a context whose logger writes to buf as JSON,
// so tests can assert on structured fields without touching the global
// default logger.
func newCapturingCtx(t *testing.T) (context.Context, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logging.With(context.Background(), logger), &buf
}

func TestHandle_NilErrorIsNoop(t *testing.T) {
	ctx, buf := newCapturingCtx(t)
	errutil.Handle(ctx, nil, "should be ignored")
	gt.Number(t, buf.Len()).Equal(0)
}

func TestHandle_GoerrLogsValuesAndStack(t *testing.T) {
	ctx, buf := newCapturingCtx(t)
	err := goerr.New("boom",
		goerr.V("slack_error", "missing_scope"),
		goerr.V("query", "in:#general"),
	)

	errutil.Handle(ctx, err, "slack search failed")

	logged := buf.String()
	gt.String(t, logged).Contains(`"msg":"slack search failed"`)
	gt.String(t, logged).Contains(`"slack_error":"missing_scope"`)
	gt.String(t, logged).Contains(`"query":"in:#general"`)
}

func TestHandle_PlainErrorLogsMessageOnly(t *testing.T) {
	ctx, buf := newCapturingCtx(t)
	errutil.Handle(ctx, errString("plain"), "non-goerr")
	logged := buf.String()
	gt.String(t, logged).Contains(`"error":"plain"`)
	gt.Bool(t, strings.Contains(logged, `"values"`)).False()
}

func TestHandle_UsesContextLoggerNotDefault(t *testing.T) {
	// Two contexts with two different loggers; only the ctx-bound logger
	// should receive the error event. This guards against a regression to
	// logging.Default().
	ctxA, bufA := newCapturingCtx(t)
	_, bufB := newCapturingCtx(t)

	errutil.Handle(ctxA, goerr.New("x"), "to A only")

	gt.Number(t, bufA.Len()).GreaterOrEqual(1)
	gt.Number(t, bufB.Len()).Equal(0)
}

func TestHandleHTTP_WritesStatusAndLogs(t *testing.T) {
	ctx, buf := newCapturingCtx(t)
	rec := httptest.NewRecorder()

	err := goerr.New("nope", goerr.V("user_id", "U123"))
	errutil.HandleHTTP(ctx, rec, err, 502)

	gt.Number(t, rec.Code).Equal(502)
	gt.String(t, rec.Body.String()).Contains("nope")
	logged := buf.String()
	gt.String(t, logged).Contains(`"status":502`)
	gt.String(t, logged).Contains(`"user_id":"U123"`)
}

func TestHandleHTTP_NilErrorIsNoop(t *testing.T) {
	ctx, buf := newCapturingCtx(t)
	rec := httptest.NewRecorder()
	errutil.HandleHTTP(ctx, rec, nil, 500)
	gt.Number(t, buf.Len()).Equal(0)
	// httptest.ResponseRecorder defaults to 200 when WriteHeader is never
	// called. We assert no status was written.
	gt.Number(t, rec.Code).Equal(200)
}

type errString string

func (e errString) Error() string { return string(e) }
