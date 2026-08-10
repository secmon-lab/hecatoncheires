package kernel

import (
	"context"
	"log/slog"

	"github.com/gollem-dev/agentkit"
	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// SlotGate admits a bounded number of runs to execute concurrently across the
// whole deployment. It is the port behind the Job concurrency limit; the kernel
// holds no opinion about what a slot means beyond "capacity to run".
//
// The gate is asked once per CLAIM, not once per run. That is what makes it work
// without any durable hold: a claim is the only scope that brackets a worker's
// whole stretch of work on a Process, so the token and its heartbeat can live in
// the claim's memory and context and still be released correctly when the claim
// ends — including when the instance dies, since an unrenewed slot expires.
type SlotGate interface {
	// Acquire takes a slot for the run identified by ref, and returns
	// (nil, nil) when every slot is occupied.
	//
	// "None free" is an ordinary answer, not an error: the caller waits and is
	// asked again. An error means the gate could not tell how many runs are in
	// flight, which the caller must treat as "do not proceed".
	Acquire(ctx context.Context, ref SlotRef) (SlotHold, error)
}

// SlotRef identifies the run a slot is held for. It exists so the gate can
// record who holds what without the kernel handing over its Scope.
type SlotRef struct {
	WorkspaceID string
	CaseID      int64
	JobID       string
}

// SlotHold is an acquired slot. Release frees it; it must be safe to call more
// than once.
type SlotHold interface {
	Release(ctx context.Context)
}

// slotGuard wraps a claim in the concurrency gate.
//
// A claim that cannot get a slot returns WITHOUT calling next, which agentkit
// defines as refusing the claim: the Process goes back to pending with a retry
// backoff and is claimed again later, and the step-attempt counter is not
// charged because no Step ran. So a full gate delays work rather than failing
// it, and a run cannot be stranded by waiting.
//
// The gate is also asked on every later claim of the same run, not only the
// first. A run that suspends to wait for its children genuinely occupies no
// execution capacity while it waits, so holding a slot across the wait would
// under-admit; conversely a run must not resume without capacity just because it
// had some earlier.
func slotGuard(gate SlotGate) agentkit.ClaimMiddleware {
	return func(next agentkit.ClaimHandler) agentkit.ClaimHandler {
		return func(ctx context.Context, req *agentkit.ClaimRequest) (agentkit.ClaimOutcome, error) {
			proc := req.Process
			sc := ScopeFrom(proc.Metadata)
			if gate == nil || !sc.SlotGated {
				return next(ctx, req)
			}

			hold, err := gate.Acquire(ctx, SlotRef{
				WorkspaceID: sc.WorkspaceID, CaseID: sc.CaseID, JobID: sc.JobID,
			})
			if err != nil {
				// Fail closed. With the gate's state unreadable there is no way to
				// know how many runs are already in flight, and proceeding anyway
				// invites the provider rate-limit blowout the gate exists to
				// prevent. Refusing retries later, which costs latency only.
				errutil.Handle(ctx, goerr.Wrap(err, "acquire an execution slot",
					goerr.V("process", proc.ID)), "acquire an execution slot")
				return agentkit.ClaimRefused, nil
			}
			if hold == nil {
				logging.From(ctx).Debug("no execution slot free; the run waits",
					slog.String("process_id", string(proc.ID)))
				return agentkit.ClaimRefused, nil
			}
			// WithoutCancel so the release still reaches the backend when the claim
			// ended because its context was cancelled — otherwise a shutdown would
			// leave slots occupied until their TTL expires.
			defer hold.Release(context.WithoutCancel(ctx))

			return next(ctx, req)
		}
	}
}
