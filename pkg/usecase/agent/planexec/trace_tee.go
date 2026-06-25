package planexec

import (
	"github.com/gollem-dev/gollem/trace"
)

// combineTrace returns a single gollem trace.Handler that forwards to
// every non-nil argument. gollem's WithTrace keeps only one handler, so
// when a run needs to feed both its internal archive recorder and a
// host-supplied per-event handler (e.g. the Job timeline handler), they
// are combined here and wired as one.
//
// It collapses the trivial cases so the common single-backend path keeps
// zero wrapper overhead and unchanged behaviour:
//   - no handler        → nil (gollem's WithTrace skips a nil handler)
//   - exactly one       → that handler, unwrapped
//   - two or more       → trace.Multi, which gives each backend its own
//     isolated context so two handlers never collide on a context key.
func combineTrace(handlers ...trace.Handler) trace.Handler {
	nonNil := make([]trace.Handler, 0, len(handlers))
	for _, h := range handlers {
		if h != nil {
			nonNil = append(nonNil, h)
		}
	}
	switch len(nonNil) {
	case 0:
		return nil
	case 1:
		return nonNil[0]
	default:
		return trace.Multi(nonNil...)
	}
}
