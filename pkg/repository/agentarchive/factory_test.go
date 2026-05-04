package agentarchive_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
)

func TestHistoryObjectPath(t *testing.T) {
	t.Run("without prefix", func(t *testing.T) {
		got := agentarchive.HistoryObjectPathForTest("", "session-A")
		gt.String(t, got).Equal("v1/sessions/session-A/history.json")
	})

	t.Run("with prefix", func(t *testing.T) {
		got := agentarchive.HistoryObjectPathForTest("envs/dev", "019df0c9-0311-7bda-a763-116754f0310b")
		gt.String(t, got).Equal("envs/dev/v1/sessions/019df0c9-0311-7bda-a763-116754f0310b/history.json")
	})

	t.Run("trims surrounding slashes from prefix", func(t *testing.T) {
		got := agentarchive.HistoryObjectPathForTest("/staging/", "S")
		gt.String(t, got).Equal("staging/v1/sessions/S/history.json")
	})
}

func TestTraceObjectPath(t *testing.T) {
	t.Run("without prefix", func(t *testing.T) {
		got := agentarchive.TraceObjectPathForTest("", "session-A", "trace-1")
		gt.String(t, got).Equal("v1/traces/session-A/trace-1.json")
	})

	t.Run("with prefix", func(t *testing.T) {
		got := agentarchive.TraceObjectPathForTest("envs/dev", "S", "T")
		gt.String(t, got).Equal("envs/dev/v1/traces/S/T.json")
	})

	t.Run("trims surrounding slashes from prefix", func(t *testing.T) {
		got := agentarchive.TraceObjectPathForTest("/staging/", "S", "T")
		gt.String(t, got).Equal("staging/v1/traces/S/T.json")
	})
}
