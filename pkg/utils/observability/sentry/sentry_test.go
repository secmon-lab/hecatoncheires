package sentry_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	obssentry "github.com/secmon-lab/hecatoncheires/pkg/utils/observability/sentry"
)

// captureTransport is a sentrygo.Transport implementation that records
// every event it would have sent so tests can inspect them in-process.
type captureTransport struct {
	mu     sync.Mutex
	events []*sentrygo.Event
}

func (t *captureTransport) Configure(_ sentrygo.ClientOptions) {}

func (t *captureTransport) SendEvent(event *sentrygo.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, event)
}

func (t *captureTransport) Flush(_ time.Duration) bool              { return true }
func (t *captureTransport) FlushWithContext(_ context.Context) bool { return true }
func (t *captureTransport) Close()                                  {}
func (t *captureTransport) Snapshot() []*sentrygo.Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]*sentrygo.Event, len(t.events))
	copy(out, t.events)
	return out
}

// withSentry initializes the global Sentry hub with a capturing transport
// for the duration of the test, then resets it on cleanup.
func withSentry(t *testing.T) *captureTransport {
	t.Helper()
	tr := &captureTransport{}
	gt.NoError(t, sentrygo.Init(sentrygo.ClientOptions{
		Dsn:       "http://test@127.0.0.1/1",
		Transport: tr,
	})).Required()
	obssentry.SetEnabledForTest(true)
	t.Cleanup(func() {
		obssentry.SetEnabledForTest(false)
	})
	return tr
}

func TestCapture_DisabledIsNoop(t *testing.T) {
	gt.Bool(t, obssentry.Enabled()).False().Required()
	obssentry.Capture(context.Background(), goerr.New("ignored"))
	// Capture must not enable Sentry as a side-effect.
	gt.Bool(t, obssentry.Enabled()).False()
}

func TestCapture_NilErrorIsNoop(t *testing.T) {
	tr := withSentry(t)
	obssentry.Capture(context.Background(), nil)
	gt.Array(t, tr.Snapshot()).Length(0)
}

func TestCapture_AttachesGoerrValuesAsContext(t *testing.T) {
	tr := withSentry(t)

	err := goerr.New("boom",
		goerr.V("slack_error", "missing_scope"),
		goerr.V("query", "in:#general"),
	)
	obssentry.Capture(context.Background(), err)
	obssentry.Flush(time.Second)

	events := tr.Snapshot()
	gt.Array(t, events).Length(1).Required()

	gv, ok := events[0].Contexts["goerr_values"]
	gt.Bool(t, ok).True().Required()
	gt.Value(t, gv["slack_error"]).Equal("missing_scope")
	gt.Value(t, gv["query"]).Equal("in:#general")
}

func TestCapture_PlainError_NoContextAttached(t *testing.T) {
	tr := withSentry(t)
	obssentry.Capture(context.Background(), errString("plain"))
	obssentry.Flush(time.Second)

	events := tr.Snapshot()
	gt.Array(t, events).Length(1).Required()
	_, hasGoerr := events[0].Contexts["goerr_values"]
	gt.Bool(t, hasGoerr).False()
}

func TestHTTPMiddleware_DisabledReturnsHandlerUnchanged(t *testing.T) {
	gt.Bool(t, obssentry.Enabled()).False().Required()

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	wrapped := obssentry.HTTPMiddleware(next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	wrapped.ServeHTTP(rec, req)

	gt.Bool(t, called).True()
}

func TestFlush_DisabledIsNoop(t *testing.T) {
	gt.Bool(t, obssentry.Enabled()).False().Required()
	gt.Bool(t, obssentry.Flush(time.Millisecond)).True()
}

// errString is a minimal error type for tests that need a non-goerr value.
type errString string

func (e errString) Error() string { return string(e) }
