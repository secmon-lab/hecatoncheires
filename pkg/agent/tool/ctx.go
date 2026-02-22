package tool

import "context"

// UpdateFunc is a function that posts a progress message during tool execution.
// Tools call this to report what they are doing to the user in real time.
type UpdateFunc func(ctx context.Context, message string)

type contextKey struct{}

// WithUpdate returns a new context that carries the given UpdateFunc.
func WithUpdate(ctx context.Context, fn UpdateFunc) context.Context {
	return context.WithValue(ctx, contextKey{}, fn)
}

// Update calls the UpdateFunc stored in ctx with the given message.
// If no UpdateFunc is present in ctx, the call is a no-op.
func Update(ctx context.Context, message string) {
	if fn, ok := ctx.Value(contextKey{}).(UpdateFunc); ok {
		fn(ctx, message)
	}
}
