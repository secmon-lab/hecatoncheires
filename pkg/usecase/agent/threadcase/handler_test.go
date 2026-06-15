package threadcase_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/threadcase"
)

// TestHandlerFuncs_TraceRouting verifies the struct-of-funcs adapter routes
// milestone (TraceAppend) and transient activity (TraceReplace) lines to their
// respective closures.
func TestHandlerFuncs_TraceRouting(t *testing.T) {
	ctx := context.Background()
	var appended, replaced []string
	h := threadcase.HandlerFuncs{
		TraceAppendFn:  func(_ context.Context, line string) { appended = append(appended, line) },
		TraceReplaceFn: func(_ context.Context, line string) { replaced = append(replaced, line) },
	}

	h.TraceAppend(ctx, "🔎 Investigating (2 task(s))")
	h.TraceReplace(ctx, "Searching Slack: from:@issei")
	h.TraceReplace(ctx, "Fetching Notion page abc")

	gt.Array(t, appended).Equal([]string{"🔎 Investigating (2 task(s))"})
	gt.Array(t, replaced).Equal([]string{
		"Searching Slack: from:@issei",
		"Fetching Notion page abc",
	})
}

// TestHandlerFuncs_UnsetIsNoOp verifies that unset trace closures are treated
// as no-ops rather than panicking, preserving backward compatibility for
// minimal hosts and tests that only wire Question/Create.
func TestHandlerFuncs_UnsetIsNoOp(t *testing.T) {
	ctx := context.Background()
	var h threadcase.HandlerFuncs
	h.TraceAppend(ctx, "x")
	h.TraceReplace(ctx, "y")
}
