package http

import (
	"context"
	"net/http"

	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// ScheduledScanner is the narrow surface the HTTP layer needs to fire a
// scheduled-Job sweep. The runtime implementation lives in
// pkg/usecase/job.ScheduledScanner; this interface keeps the HTTP layer
// off the usecase import.
type ScheduledScanner interface {
	Scan(ctx context.Context) error
}

// ScheduledHookHandler exposes POST /hooks/scheduled. The endpoint is
// intentionally unauthenticated: the assumption is that the deployment
// fronts it with IAP / internal-network policy. Body is ignored.
//
// The handler returns 200 immediately and dispatches the sweep in the
// background, mirroring the Slack hook ack-fast pattern: external
// schedulers (Cloud Scheduler) treat a delayed response as failure
// (and may retry, doubling the work) while the LLM round-trips inside
// the sweep can comfortably exceed any reasonable HTTP timeout.
type ScheduledHookHandler struct {
	scanner ScheduledScanner
}

// NewScheduledHookHandler builds the handler.
func NewScheduledHookHandler(scanner ScheduledScanner) *ScheduledHookHandler {
	return &ScheduledHookHandler{scanner: scanner}
}

// ServeHTTP implements http.Handler.
func (h *ScheduledHookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.scanner == nil {
		http.Error(w, "scheduled scanner not configured", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	async.Dispatch(r.Context(), func(bgCtx context.Context) error {
		if err := h.scanner.Scan(bgCtx); err != nil {
			errutil.Handle(bgCtx, goerr.Wrap(err, "scheduled scan"), "scheduled scan")
			return err
		}
		return nil
	})
}
