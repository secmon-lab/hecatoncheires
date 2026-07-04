// Sample input for usecase_context_first.rego — nothing below must be flagged.
package usecase

import "context"

// context.Context first — allowed.
func RunTask(ctx context.Context, id string) error { return nil }

// unexported — out of scope.
func helper(id string) {}

// constructor (New* prefix) — exempt.
func NewThing() *Thing { return nil }

// functional option (With* prefix) — exempt.
func WithTimeout(d int) Option { return nil }

// pure value method (listed in exempt_name) — exempt.
func (t Thing) Validate() error { return nil }

type Thing struct{}

type Option func()
