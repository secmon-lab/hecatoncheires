package kernel_test

import (
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
)

// TestScopeRoundTrip pins that every field survives the trip through
// Process.Metadata. A field that silently fails to round-trip reappears as an
// agent with the wrong tools or no access actor, which is exactly the class of
// bug the typed Scope exists to prevent.
func TestScopeRoundTrip(t *testing.T) {
	want := kernel.Scope{
		WorkspaceID: "ws-1",
		CaseID:      42,
		ChannelID:   "C123",
		ThreadTS:    "1700000000.000100",
		SessionID:   "ssn-1",
		ActorUserID: "U999",
		Lang:        "ja",
		ToolSets:    []string{"slack_ro", "notion"},
		PrivateCase: true,
		JobID:       "job-1",
		JobRunID:    "run-1",
		EventType:   "mention",
	}

	got := kernel.ScopeFrom(want.Metadata())
	gt.Value(t, got).Equal(want)
}

func TestScopeMetadataOmitsEmptyValues(t *testing.T) {
	sc := kernel.Scope{ToolSets: []string{kernel.ToolSetsAll}}
	m := sc.Metadata()

	gt.Map(t, m).HasKey("toolsets")
	for _, key := range []string{
		"workspace_id", "case_id", "channel_id", "thread_ts",
		"session_id", "actor_user_id", "lang", "private_case",
		"job_id", "job_run_id", "event_type",
	} {
		_, ok := m[key]
		gt.Bool(t, ok).False()
	}
}

// TestScopeFromMalformedValues pins that a hand-edited or older record still
// yields a usable Scope. Refusing to parse would strand the Process with no way
// to finish.
func TestScopeFromMalformedValues(t *testing.T) {
	got := kernel.ScopeFrom(map[string]string{
		"case_id":      "not-a-number",
		"private_case": "yes",
		"toolsets":     ",slack_ro,,notion,",
	})

	gt.Value(t, got.CaseID).Equal(int64(0))
	gt.Bool(t, got.PrivateCase).False() // only "1" means private
	gt.Array(t, got.ToolSets).Equal([]string{"slack_ro", "notion"})
}

func TestScopeFromEmptyMetadata(t *testing.T) {
	got := kernel.ScopeFrom(nil)
	gt.Value(t, got).Equal(kernel.Scope{})
}

func TestScopeValidate(t *testing.T) {
	valid := kernel.Scope{
		WorkspaceID: "ws-1",
		CaseID:      1,
		ChannelID:   "C1",
		ThreadTS:    "1.1",
		ToolSets:    []string{kernel.ToolSetsAll},
	}

	testCases := map[string]struct {
		mutate  func(kernel.Scope) kernel.Scope
		wantErr bool
	}{
		"valid": {
			mutate: func(s kernel.Scope) kernel.Scope { return s },
		},
		"thread without channel": {
			mutate:  func(s kernel.Scope) kernel.Scope { s.ChannelID = ""; return s },
			wantErr: true,
		},
		"channel without thread": {
			mutate:  func(s kernel.Scope) kernel.Scope { s.ThreadTS = ""; return s },
			wantErr: true,
		},
		"neither channel nor thread is fine": {
			mutate: func(s kernel.Scope) kernel.Scope { s.ChannelID = ""; s.ThreadTS = ""; return s },
		},
		"case without workspace": {
			mutate:  func(s kernel.Scope) kernel.Scope { s.WorkspaceID = ""; return s },
			wantErr: true,
		},
		"negative case id": {
			mutate:  func(s kernel.Scope) kernel.Scope { s.CaseID = -1; return s },
			wantErr: true,
		},
		"no toolsets": {
			mutate:  func(s kernel.Scope) kernel.Scope { s.ToolSets = nil; return s },
			wantErr: true,
		},
		"empty toolset id": {
			mutate:  func(s kernel.Scope) kernel.Scope { s.ToolSets = []string{"slack_ro", ""}; return s },
			wantErr: true,
		},
		"job run without job": {
			mutate:  func(s kernel.Scope) kernel.Scope { s.JobRunID = "run-1"; return s },
			wantErr: true,
		},
		"job run with job": {
			mutate: func(s kernel.Scope) kernel.Scope { s.JobID = "job-1"; s.JobRunID = "run-1"; return s },
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			err := tc.mutate(valid).Validate()
			if tc.wantErr {
				gt.Value(t, err).NotNil()
				return
			}
			gt.NoError(t, err)
		})
	}
}

// TestWithToolSets pins the property a sub-agent spawn depends on: agentkit's
// SpawnChild REPLACES the parent's metadata map, so the child's map must carry
// the parent's scope forward and differ only in the toolset list.
func TestWithToolSets(t *testing.T) {
	parent := kernel.Scope{
		WorkspaceID: "ws-1",
		CaseID:      7,
		ChannelID:   "C1",
		ThreadTS:    "1.1",
		ActorUserID: "U1",
		ToolSets:    []string{kernel.ToolSetsAll},
	}.Metadata()

	child := kernel.WithToolSets(parent, []string{"slack_ro", "webfetch"})
	childScope := kernel.ScopeFrom(child)

	gt.String(t, childScope.WorkspaceID).Equal("ws-1")
	gt.Value(t, childScope.CaseID).Equal(int64(7))
	gt.String(t, childScope.ActorUserID).Equal("U1")
	gt.Array(t, childScope.ToolSets).Equal([]string{"slack_ro", "webfetch"})

	// The parent's own map must not have been rewritten.
	gt.Array(t, kernel.ScopeFrom(parent).ToolSets).Equal([]string{kernel.ToolSetsAll})
}

func TestWithToolSetsEmptyList(t *testing.T) {
	parent := kernel.Scope{WorkspaceID: "ws-1", ToolSets: []string{"slack_ro"}}.Metadata()
	child := kernel.WithToolSets(parent, nil)

	gt.String(t, kernel.ScopeFrom(child).WorkspaceID).Equal("ws-1")
	gt.Array(t, kernel.ScopeFrom(child).ToolSets).Length(0)
}

func TestWithToolSetsNilParent(t *testing.T) {
	child := kernel.WithToolSets(nil, []string{"slack_ro"})
	gt.Array(t, kernel.ScopeFrom(child).ToolSets).Equal([]string{"slack_ro"})
}
