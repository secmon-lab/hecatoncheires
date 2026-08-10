package casebound_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/casebound"
)

// A HostFuncs entry that was never supplied must ERROR rather than no-op:
// silently dropping the answer to a mention is indistinguishable, from the
// user's side, from the agent having had nothing to say.
func TestHostFuncsMissingEntriesAreErrors(t *testing.T) {
	ctx := context.Background()
	var h casebound.Host = casebound.HostFuncs{}

	gt.Error(t, h.Reply(ctx, "C1", "1700000000.000001", "answer"))
	gt.Error(t, h.ReportFailure(ctx, "C1", "1700000000.000001", "reason"))
}

func TestHostFuncsRoutesEachCall(t *testing.T) {
	ctx := context.Background()
	type call struct {
		kind, channelID, threadTS, text string
	}
	var got []call

	h := casebound.HostFuncs{
		ReplyFn: func(_ context.Context, channelID, threadTS, text string) error {
			got = append(got, call{"reply", channelID, threadTS, text})
			return nil
		},
		ReportFailureFn: func(_ context.Context, channelID, threadTS, reason string) error {
			got = append(got, call{"failure", channelID, threadTS, reason})
			return nil
		},
	}

	gt.NoError(t, h.Reply(ctx, "C1", "1700000000.000001", "the answer"))
	gt.NoError(t, h.ReportFailure(ctx, "C2", "1700000000.000002", "budget exhausted"))

	gt.Array(t, got).Length(2).Required()
	gt.Value(t, got[0]).Equal(call{"reply", "C1", "1700000000.000001", "the answer"})
	gt.Value(t, got[1]).Equal(call{"failure", "C2", "1700000000.000002", "budget exhausted"})
}
