package casebound_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/casebound"
)

// TestHandlerFunc_RoutesAppendDropsReplace verifies the single-func adapter:
// the supplied closure receives milestones via TraceAppend, while TraceReplace
// (transient per-tool activity) is silently dropped because a single func has
// nowhere to render an overwriting line.
func TestHandlerFunc_RoutesAppendDropsReplace(t *testing.T) {
	ctx := context.Background()
	var got []string
	var h casebound.Handler = casebound.HandlerFunc(func(_ context.Context, line string) {
		got = append(got, line)
	})

	h.TraceAppend(ctx, "milestone-1")
	h.TraceReplace(ctx, "activity-1")
	h.TraceAppend(ctx, "milestone-2")

	gt.Array(t, got).Equal([]string{"milestone-1", "milestone-2"})
}

// TestHandlerFunc_NilIsNoOp verifies a nil HandlerFunc does not panic on
// either method.
func TestHandlerFunc_NilIsNoOp(t *testing.T) {
	ctx := context.Background()
	var h casebound.HandlerFunc
	h.TraceAppend(ctx, "x")
	h.TraceReplace(ctx, "y")
}

// TestHandlerFuncs_RoutesEachKind verifies the struct-of-funcs adapter routes
// appends and replaces to their respective closures, and treats nil entries as
// no-ops.
func TestHandlerFuncs_RoutesEachKind(t *testing.T) {
	ctx := context.Background()
	var appended, replaced []string
	h := casebound.HandlerFuncs{
		TraceAppendFn:  func(_ context.Context, line string) { appended = append(appended, line) },
		TraceReplaceFn: func(_ context.Context, line string) { replaced = append(replaced, line) },
	}

	h.TraceAppend(ctx, "m1")
	h.TraceReplace(ctx, "a1")
	h.TraceReplace(ctx, "a2")

	gt.Array(t, appended).Equal([]string{"m1"})
	gt.Array(t, replaced).Equal([]string{"a1", "a2"})

	// Nil entries must not panic.
	empty := casebound.HandlerFuncs{}
	empty.TraceAppend(ctx, "x")
	empty.TraceReplace(ctx, "y")
}
