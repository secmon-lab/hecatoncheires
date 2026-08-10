package sample

import "context"

type fakeKernel struct{}

func (fakeKernel) Serve(context.Context, ...any) error { return nil }

type holder struct {
	kernel fakeKernel
}

// Two workers started by calling Kernel.Serve directly, which skips
// NoDuplicateSideEffects — so a claim that died mid-transition would be re-run and
// its tool's side effect repeated.
func bad(ctx context.Context) {
	var agentKernel fakeKernel
	_ = agentKernel.Serve(ctx)

	var uc holder
	_ = uc.kernel.Serve(ctx)
}
