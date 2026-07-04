package controller

// Outside pkg/usecase, so usecase_context_first.rego does not apply even though
// this exported function takes no context.Context — must NOT be flagged.
func DoWork(name string) error { return nil }
