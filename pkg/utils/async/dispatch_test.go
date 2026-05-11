package async_test

import (
	"context"
	"sync"
	"testing"
	"time"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

// captureTransport records every event the SDK would send so tests can
// assert that Dispatch's error path reaches Sentry via errutil.Handle.
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
func (t *captureTransport) snapshot() []*sentrygo.Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]*sentrygo.Event, len(t.events))
	copy(out, t.events)
	return out
}

// withSentry installs a capturing transport on the global hub for the
// duration of the test. Sentry state is global, so every test that
// touches it must use this helper.
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

func TestDispatch_HandlerErrorReachesSentry(t *testing.T) {
	tr := withSentry(t)

	bug := goerr.New("synthetic handler failure", goerr.V("step", "test"))
	async.Dispatch(context.Background(), func(_ context.Context) error {
		return bug
	})
	async.Wait()

	events := tr.snapshot()
	gt.Array(t, events).Length(1).Required()
	gt.Array(t, events[0].Exception).Length(1).Required()
	gt.String(t, events[0].Exception[0].Value).Contains("synthetic handler failure")
	gv, ok := events[0].Contexts["goerr_values"]
	gt.Bool(t, ok).True().Required()
	gt.Value(t, gv["step"]).Equal("test")
}

func TestDispatch_HandlerNilErrorDoesNotReachSentry(t *testing.T) {
	tr := withSentry(t)

	async.Dispatch(context.Background(), func(_ context.Context) error {
		return nil
	})
	async.Wait()

	gt.Array(t, tr.snapshot()).Length(0)
}

func TestDispatch_HandlerPanicReachesSentry(t *testing.T) {
	tr := withSentry(t)

	async.Dispatch(context.Background(), func(_ context.Context) error {
		panic("boom")
	})
	async.Wait()

	events := tr.snapshot()
	gt.Array(t, events).Length(1).Required()
	gt.Array(t, events[0].Exception).Length(1).Required()
	gt.String(t, events[0].Exception[0].Value).Contains("panic in async handler")
	gv, ok := events[0].Contexts["goerr_values"]
	gt.Bool(t, ok).True().Required()
	gt.Value(t, gv["panic"]).Equal("boom")
}
