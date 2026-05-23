package http_test

import (
	"context"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/m-mizutani/gt"

	httpctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/http"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

type stubScanner struct {
	calls atomic.Int32
}

func (s *stubScanner) Scan(_ context.Context) error {
	s.calls.Add(1)
	return nil
}

func TestScheduledHookHandler_RespondsImmediatelyAndTriggersScan(t *testing.T) {
	scanner := &stubScanner{}
	h := httpctrl.NewScheduledHookHandler(scanner)

	req := httptest.NewRequest("POST", "/hooks/scheduled", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	gt.Number(t, rec.Code).Equal(200)

	// Wait for the async dispatch to complete.
	async.Wait()
	gt.Number(t, scanner.calls.Load()).Equal(int32(1))
}

func TestScheduledHookHandler_FailsWhenScannerNil(t *testing.T) {
	h := httpctrl.NewScheduledHookHandler(nil)
	req := httptest.NewRequest("POST", "/hooks/scheduled", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	gt.Number(t, rec.Code).Equal(503)
}
