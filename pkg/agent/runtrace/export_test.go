package runtrace

// TokenUsageForTest is the per-handler token total. Production code reaches it
// only through AddTokenUsage, so the type stays unexported; tests need the
// literal to assert the accumulation itself.
type TokenUsageForTest = tokenUsage

// HandlerTokenUsageForTest exposes Handler.tokenUsage so tests can observe the
// running total without persisting a JobRunLog first.
func HandlerTokenUsageForTest(h *Handler) tokenUsage { return h.tokenUsage() }
