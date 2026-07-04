// Sample input for usecase_context_first.rego — both funcs must be flagged.
// Nested under a pkg/usecase path because the rule is scoped to that path.
package usecase

import "context"

// Exported use-case function with no context.Context first — must be flagged.
func DoWork(id string) error { return nil }

// Exported with context.Context present but not first — must be flagged.
func Handle(id string, ctx context.Context) error { return nil }
