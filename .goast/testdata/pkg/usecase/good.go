package usecase

import "context"

type Repo struct{}

// context.Context is the first parameter — allowed.
func (r *Repo) RunTask(ctx context.Context, name string) error { return nil }

// Unexported — allowed regardless of signature.
func helper(name string) {}

// New* constructor — exempt by prefix.
func NewRepo() *Repo { return &Repo{} }

// With* functional option — exempt by prefix.
func WithTimeout(seconds int) {}

// Is* boolean predicate — exempt by prefix.
func (r *Repo) IsReady() bool { return true }

// Parse* pure decoder — exempt by prefix.
func ParseValue(v string) string { return v }

// Validate is an exact-name exemption (pure value method).
func (r *Repo) Validate() error { return nil }
