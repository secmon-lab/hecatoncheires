package runtrace

// RunTotalsForTest is the per-handler tally of tokens and call counts.
// Production code reaches it only through AddRunTotals, so the type stays
// unexported; tests need the literal to assert the accumulation itself.
type RunTotalsForTest = runTotals

// HandlerRunTotalsForTest exposes Handler.runTotals so tests can observe the
// running tally without persisting a JobRunLog first.
func HandlerRunTotalsForTest(h *Handler) runTotals { return h.runTotals() }
