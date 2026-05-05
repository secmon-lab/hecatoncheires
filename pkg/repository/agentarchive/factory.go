// Package agentarchive provides Cloud Storage and in-memory implementations
// of gollem.HistoryRepository and gollem/trace.Repository for the AgentSession
// flow.
//
// Object layout under the configured bucket:
//
//	{prefix}/v1/sessions/{sessionID}/history.json
//	{prefix}/v1/traces/{sessionID}/{traceID}.json
//
// sessionID is the AgentSession.ID (UUIDv7) and is passed verbatim by the
// usecase as the gollem session identifier.
package agentarchive

import (
	"path"
	"strings"
)

const (
	versionDir  = "v1"
	sessionsDir = "sessions"
	tracesDir   = "traces"
)

// joinObjectPath joins the optional prefix with the remaining segments using
// forward slashes (Cloud Storage object names are always slash-separated
// regardless of OS).
func joinObjectPath(prefix string, parts ...string) string {
	segments := []string{}
	if prefix = strings.Trim(prefix, "/"); prefix != "" {
		segments = append(segments, prefix)
	}
	segments = append(segments, parts...)
	return path.Join(segments...)
}

// historyObjectPath returns the Cloud Storage object name for the history blob
// of the given sessionID.
func historyObjectPath(prefix, sessionID string) string {
	return joinObjectPath(prefix, versionDir, sessionsDir, sessionID, "history.json")
}

// traceObjectPath returns the Cloud Storage object name for the trace blob of
// the given (sessionID, traceID).
func traceObjectPath(prefix, sessionID, traceID string) string {
	return joinObjectPath(prefix, versionDir, tracesDir, sessionID, traceID+".json")
}
