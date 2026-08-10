// Package kernel assembles the agentkit Kernel this application runs its
// agents on: the tool factory, the observability middleware, and the typed view
// of the per-Process scope that both of them read.
package kernel

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/m-mizutani/goerr/v2"
)

// Metadata keys. They are unexported because Scope is the only supported way to
// read or write Process.Metadata: a caller reaching for a raw string key is one
// typo away from a silently empty scope, and the failure would surface as an
// agent with no tools rather than as a compile error.
const (
	metaWorkspaceID = "workspace_id"
	metaCaseID      = "case_id"
	metaChannelID   = "channel_id"
	metaThreadTS    = "thread_ts"
	metaSessionID   = "session_id"
	metaActorUserID = "actor_user_id"
	metaLang        = "lang"
	metaToolSets    = "toolsets"
	metaPrivateCase = "private_case"
	metaJobID       = "job_id"
	metaJobRunID    = "job_run_id"
	metaEventType   = "event_type"
	metaSlotGated   = "slot_gated"
)

// ToolSetsAll is the toolsets value meaning "everything this agent kind is
// entitled to". A root Process uses it; a sub-agent Process instead carries the
// explicit subset its task was planned with.
const ToolSetsAll = "*"

// toolSetSeparator joins the toolsets list into one metadata value. The value
// carries one meaning — an ordered list of toolset ids — so the separator is not
// packing several semantics into a single string.
const toolSetSeparator = ","

// Scope is the typed view of Process.Metadata: the infrastructure-facing
// identifiers a claim needs to rebuild an agent's tools, its language, its
// access actor and its run records.
//
// It is data, not a credential. Every field is derived server-side before Spawn
// from an already-validated request; nothing here is re-verified at claim time,
// and nothing here may be treated as proof of anything (agentkit ADR-0011).
type Scope struct {
	// WorkspaceID identifies the workspace whose configuration and tools this
	// Process runs under. Empty only for the workspace-agnostic draft flow.
	WorkspaceID string
	// CaseID is the case this Process is pinned to, or 0 when there is none
	// (a draft turn, or a create turn before the case exists).
	CaseID int64
	// ChannelID / ThreadTS locate the Slack thread the run reports into.
	ChannelID string
	ThreadTS  string
	// SessionID is the model.Session this thread belongs to. It doubles as the
	// turn-lock subject id.
	SessionID string
	// ActorUserID is the Slack user whose access this run acts under. The claim
	// middleware turns it into the request-scoped auth token, so a run without
	// one acts with no user scope at all.
	ActorUserID string
	// Lang is the i18n language tag for user-facing copy this run produces.
	Lang string
	// ToolSets is the list of toolset ids this Process may use, or the single
	// element ToolSetsAll.
	ToolSets []string
	// PrivateCase withholds the workspace-wide knowledge write tools: a private
	// case's contents must not reach shared knowledge through an agent write.
	PrivateCase bool
	// JobID / JobRunID / EventType tie the run to its JobRunLog record. Empty
	// for runs that keep no such record.
	JobID     string
	JobRunID  string
	EventType string
	// SlotGated subjects the run to the deployment-wide concurrency gate: a
	// claim on it waits for a free execution slot before any transition runs.
	//
	// The host decides it at spawn rather than the runtime inferring it, so the
	// kernel needs no opinion about which kinds of run are rate-limited. Today
	// only scheduled Job runs set it — an interactive turn is a person waiting
	// for an answer, and a lifecycle or manual run is a single deliberate
	// action, so making either queue behind a batch would be the wrong trade.
	SlotGated bool
}

// Validate enforces the invariants the claim path depends on, so a wiring
// mistake fails at Spawn rather than as an agent that silently has no tools.
func (s Scope) Validate() error {
	if (s.ChannelID == "") != (s.ThreadTS == "") {
		return goerr.New("channel id and thread ts must be set together",
			goerr.V("channel_id", s.ChannelID), goerr.V("thread_ts", s.ThreadTS))
	}
	if s.CaseID != 0 && s.WorkspaceID == "" {
		return goerr.New("workspace id is required when a case id is set",
			goerr.V("case_id", s.CaseID))
	}
	if s.CaseID < 0 {
		return goerr.New("case id must not be negative", goerr.V("case_id", s.CaseID))
	}
	if len(s.ToolSets) == 0 {
		return goerr.New("at least one toolset is required")
	}
	if slices.Contains(s.ToolSets, "") {
		return goerr.New("toolset ids must not be empty")
	}
	if s.JobRunID != "" && s.JobID == "" {
		return goerr.New("job id is required when a job run id is set",
			goerr.V("job_run_id", s.JobRunID))
	}
	return nil
}

// Metadata renders the scope for Spawn. Empty values are omitted so a reader
// can tell "not set" from "set to empty", and so the stored map stays small.
func (s Scope) Metadata() map[string]string {
	m := map[string]string{}
	put := func(k, v string) {
		if v != "" {
			m[k] = v
		}
	}
	put(metaWorkspaceID, s.WorkspaceID)
	if s.CaseID != 0 {
		m[metaCaseID] = strconv.FormatInt(s.CaseID, 10)
	}
	put(metaChannelID, s.ChannelID)
	put(metaThreadTS, s.ThreadTS)
	put(metaSessionID, s.SessionID)
	put(metaActorUserID, s.ActorUserID)
	put(metaLang, s.Lang)
	put(metaToolSets, joinToolSets(s.ToolSets))
	if s.PrivateCase {
		m[metaPrivateCase] = "1"
	}
	put(metaJobID, s.JobID)
	put(metaJobRunID, s.JobRunID)
	put(metaEventType, s.EventType)
	if s.SlotGated {
		m[metaSlotGated] = "1"
	}
	return m
}

// ScopeFrom reads a scope back out of Process.Metadata.
//
// A malformed numeric value falls back to the zero value rather than failing:
// the map was written by Metadata on this same code path, so a bad value means
// the record was hand-edited or written by an older build, and refusing to run
// the Process would strand it with no way forward.
func ScopeFrom(m map[string]string) Scope {
	caseID, _ := strconv.ParseInt(m[metaCaseID], 10, 64)
	return Scope{
		WorkspaceID: m[metaWorkspaceID],
		CaseID:      caseID,
		ChannelID:   m[metaChannelID],
		ThreadTS:    m[metaThreadTS],
		SessionID:   m[metaSessionID],
		ActorUserID: m[metaActorUserID],
		Lang:        m[metaLang],
		ToolSets:    splitToolSets(m[metaToolSets]),
		PrivateCase: m[metaPrivateCase] == "1",
		JobID:       m[metaJobID],
		JobRunID:    m[metaJobRunID],
		EventType:   m[metaEventType],
		SlotGated:   m[metaSlotGated] == "1",
	}
}

// WithToolSets returns a copy of the metadata map carrying a different toolset
// list. It is what a strategy uses when spawning a child, because
// SpawnChild's WithMetadata REPLACES the parent's map rather than merging into
// it: rebuilding the map from scratch there would drop the workspace and case
// the child needs to have any tools at all.
func WithToolSets(parent map[string]string, toolSets []string) map[string]string {
	next := maps.Clone(parent)
	if next == nil {
		next = map[string]string{}
	}
	if joined := joinToolSets(toolSets); joined != "" {
		next[metaToolSets] = joined
	} else {
		delete(next, metaToolSets)
	}
	return next
}

func joinToolSets(ids []string) string {
	kept := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			kept = append(kept, id)
		}
	}
	return strings.Join(kept, toolSetSeparator)
}

func splitToolSets(v string) []string {
	if v == "" {
		return nil
	}
	var out []string
	for id := range strings.SplitSeq(v, toolSetSeparator) {
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}
