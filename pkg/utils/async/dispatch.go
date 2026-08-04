package async

import (
	"context"
	"sync"

	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

var inflight sync.WaitGroup

// Dispatch executes a handler function asynchronously in a new goroutine.
// The background context inherits every value from the caller's ctx
// (logger, Sentry hub, trace IDs, etc.) but the cancellation signal is
// severed via context.WithoutCancel — the entry-point request will
// return before the background work completes, and we do not want that
// to cancel the tail. Handler errors route through errutil.Handle so
// they land in Sentry alongside the structured log. Pending dispatches
// are tracked by a package-level WaitGroup so tests can synchronise on
// completion via Wait().
func Dispatch(ctx context.Context, handler func(ctx context.Context) error) {
	bgCtx := context.WithoutCancel(ctx)

	inflight.Add(1)
	go func() {
		defer inflight.Done()
		defer func() {
			if r := recover(); r != nil {
				errutil.Handle(bgCtx, goerr.New("panic in async handler", goerr.V("panic", r)), "async handler panicked")
			}
		}()

		if err := handler(bgCtx); err != nil {
			errutil.Handle(bgCtx, err, "async handler failed")
		}
	}()
}

// DispatchCancelable runs handler in a goroutine that dies with ctx. It is
// Dispatch minus the context.WithoutCancel: the handler receives the caller's
// context verbatim, so cancelling it terminates the goroutine.
//
// Use it for a helper goroutine bound to the caller's lifetime — a heartbeat
// that must stop when the work it guards stops, a ticker owned by one
// processing flow. Use Dispatch instead for the async tail of a request that
// has already returned, where the request's cancellation must NOT abort the
// remaining work. Both are tracked by the same WaitGroup, so Wait() covers
// either; a handler that never returns therefore blocks Wait() forever, so
// every loop needs an exit (ctx, a stop channel, or a deadline).
func DispatchCancelable(ctx context.Context, handler func(ctx context.Context) error) {
	inflight.Add(1)
	go func() {
		defer inflight.Done()
		defer func() {
			if r := recover(); r != nil {
				errutil.Handle(ctx, goerr.New("panic in async handler", goerr.V("panic", r)), "async handler panicked")
			}
		}()

		if err := handler(ctx); err != nil {
			errutil.Handle(ctx, err, "async handler failed")
		}
	}()
}

// Wait blocks until all in-flight dispatches launched via Dispatch have
// returned. Intended for use in tests that need to assert on side effects of
// the async tail; production code must not call this.
func Wait() {
	inflight.Wait()
}
