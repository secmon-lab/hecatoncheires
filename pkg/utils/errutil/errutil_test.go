package errutil_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// captureTransport records every event the SDK would send so tests can
// inspect both the fact of capture and the attached scope.
type captureTransport struct {
	mu     sync.Mutex
	events []*sentrygo.Event
}

func (t *captureTransport) Configure(_ sentrygo.ClientOptions)      {}
func (t *captureTransport) Flush(_ time.Duration) bool              { return true }
func (t *captureTransport) FlushWithContext(_ context.Context) bool { return true }
func (t *captureTransport) Close()                                  {}
func (t *captureTransport) SendEvent(event *sentrygo.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, event)
}
func (t *captureTransport) Snapshot() []*sentrygo.Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]*sentrygo.Event, len(t.events))
	copy(out, t.events)
	return out
}

// withSentry installs a capturing transport on the global hub for the
// duration of the test and tears it down on cleanup. Sentry state is
// global, so every test that touches it must use this helper.
func withSentry(t *testing.T) *captureTransport {
	t.Helper()
	tr := &captureTransport{}
	gt.NoError(t, sentrygo.Init(sentrygo.ClientOptions{
		Dsn:       "http://test@127.0.0.1/1",
		Transport: tr,
	})).Required()
	t.Cleanup(func() { sentrygo.CurrentHub().BindClient(nil) })
	return tr
}

// withoutSentry is the inverse: ensure the global hub has no client so
// Handle's capture path becomes a no-op for this test.
func withoutSentry(t *testing.T) {
	t.Helper()
	sentrygo.CurrentHub().BindClient(nil)
}

func capturingCtx(t *testing.T) (context.Context, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logging.With(context.Background(), logger), &buf
}

// --- Handle: nil/plain/goerr/wrapped — every path must log AND capture
// the right thing, or stay silent when the input is nil.

func TestHandle_NilErrorIsNoop(t *testing.T) {
	tr := withSentry(t)
	ctx, buf := capturingCtx(t)

	errutil.Handle(ctx, nil, "should be ignored")

	gt.Number(t, buf.Len()).Equal(0)
	gt.Array(t, tr.Snapshot()).Length(0)
}

func TestHandle_PlainErrorLogsAndCaptures(t *testing.T) {
	tr := withSentry(t)
	ctx, buf := capturingCtx(t)

	errutil.Handle(ctx, errString("plain"), "non-goerr")

	logged := buf.String()
	gt.String(t, logged).Contains(`"error":"plain"`)
	gt.Bool(t, strings.Contains(logged, `"values"`)).False()

	events := tr.Snapshot()
	gt.Array(t, events).Length(1).Required()
	_, hasGoerr := events[0].Contexts["goerr_values"]
	gt.Bool(t, hasGoerr).False()
}

func TestHandle_GoerrAttachesValuesToBothLogAndSentry(t *testing.T) {
	tr := withSentry(t)
	ctx, buf := capturingCtx(t)

	err := goerr.New("boom",
		goerr.V("slack_error", "missing_scope"),
		goerr.V("query", "in:#general"),
	)
	errutil.Handle(ctx, err, "slack search failed")

	logged := buf.String()
	gt.String(t, logged).Contains(`"slack_error":"missing_scope"`)
	gt.String(t, logged).Contains(`"query":"in:#general"`)

	events := tr.Snapshot()
	gt.Array(t, events).Length(1).Required()
	gv, ok := events[0].Contexts["goerr_values"]
	gt.Bool(t, ok).True().Required()
	gt.Value(t, gv["slack_error"]).Equal("missing_scope")
	gt.Value(t, gv["query"]).Equal("in:#general")
}

func TestHandle_GoerrWrappedByFmtErrorfStillExtractsValues(t *testing.T) {
	// Real call sites mix goerr.Wrap with fmt.Errorf("%w", ...). Handle
	// must walk the chain via errors.As, not assert on the outermost
	// concrete type.
	tr := withSentry(t)
	ctx, _ := capturingCtx(t)

	inner := goerr.New("inner", goerr.V("user_id", "U123"))
	outer := fmt.Errorf("outer: %w", inner)
	errutil.Handle(ctx, outer, "wrapped")

	events := tr.Snapshot()
	gt.Array(t, events).Length(1).Required()
	gv, ok := events[0].Contexts["goerr_values"]
	gt.Bool(t, ok).True().Required()
	gt.Value(t, gv["user_id"]).Equal("U123")
}

func TestHandle_GoerrWrappedByGoerrPropagatesValues(t *testing.T) {
	// goerr.Wrap merges values up the chain. Verify the merged set
	// reaches Sentry, not just the outermost wrap's own values.
	tr := withSentry(t)
	ctx, _ := capturingCtx(t)

	inner := goerr.New("inner", goerr.V("inner_key", "v1"))
	outer := goerr.Wrap(inner, "outer", goerr.V("outer_key", "v2"))
	errutil.Handle(ctx, outer, "wrapped goerr")

	events := tr.Snapshot()
	gt.Array(t, events).Length(1).Required()
	gv, ok := events[0].Contexts["goerr_values"]
	gt.Bool(t, ok).True().Required()
	// Outer wrap's values are observable; inner ones are too.
	gt.Value(t, gv["outer_key"]).Equal("v2")
	gt.Value(t, gv["inner_key"]).Equal("v1")
}

func TestHandle_SentryDisabledStillLogs(t *testing.T) {
	// Losing Sentry must not lose the log; this guards the order of the
	// "log first, capture second" pair inside Handle.
	withoutSentry(t)
	ctx, buf := capturingCtx(t)

	errutil.Handle(ctx, goerr.New("x", goerr.V("k", "v")), "no sentry")

	logged := buf.String()
	gt.String(t, logged).Contains(`"k":"v"`)
}

func TestHandle_UsesContextLoggerNotDefault(t *testing.T) {
	// Two contexts with two different loggers; only the ctx-bound logger
	// receives the event. Guards against a regression to logging.Default().
	withoutSentry(t)
	ctxA, bufA := capturingCtx(t)
	_, bufB := capturingCtx(t)

	errutil.Handle(ctxA, goerr.New("x"), "to A only")

	gt.Number(t, bufA.Len()).GreaterOrEqual(1)
	gt.Number(t, bufB.Len()).Equal(0)
}

func TestHandle_RequestScopedHubIsPreferred(t *testing.T) {
	// When the HTTP middleware has attached a per-request hub, Handle
	// must report through it (so request URL/method etc. appear on the
	// event) rather than the global hub.
	tr := withSentry(t)
	ctx, _ := capturingCtx(t)

	reqHub := sentrygo.CurrentHub().Clone()
	reqHub.ConfigureScope(func(scope *sentrygo.Scope) {
		scope.SetTag("req_id", "abc-123")
	})
	ctx = sentrygo.SetHubOnContext(ctx, reqHub)

	errutil.Handle(ctx, goerr.New("scoped"), "with req hub")

	events := tr.Snapshot()
	gt.Array(t, events).Length(1).Required()
	gt.Value(t, events[0].Tags["req_id"]).Equal("abc-123")
}

// --- HandleHTTP: status response is the only behavioural delta over
// Handle, but it must still capture exactly once and stay silent on nil.

func TestHandleHTTP_WritesStatusAndCaptures(t *testing.T) {
	tr := withSentry(t)
	ctx, buf := capturingCtx(t)
	rec := httptest.NewRecorder()

	err := goerr.New("nope", goerr.V("user_id", "U123"))
	errutil.HandleHTTP(ctx, rec, err, 502)

	gt.Number(t, rec.Code).Equal(502)
	gt.String(t, rec.Body.String()).Contains("nope")
	gt.String(t, buf.String()).Contains(`"user_id":"U123"`)

	events := tr.Snapshot()
	gt.Array(t, events).Length(1).Required()
	gv, ok := events[0].Contexts["goerr_values"]
	gt.Bool(t, ok).True().Required()
	gt.Value(t, gv["user_id"]).Equal("U123")
}

func TestHandleHTTP_NilErrorIsNoop(t *testing.T) {
	tr := withSentry(t)
	ctx, buf := capturingCtx(t)
	rec := httptest.NewRecorder()

	errutil.HandleHTTP(ctx, rec, nil, 500)

	gt.Number(t, buf.Len()).Equal(0)
	// httptest defaults to 200 when WriteHeader is never called.
	gt.Number(t, rec.Code).Equal(200)
	gt.Array(t, tr.Snapshot()).Length(0)
}

type errString string

func (e errString) Error() string { return string(e) }
