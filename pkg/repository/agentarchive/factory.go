// Package agentarchive provides Cloud Storage and in-memory implementations
// of gollem.HistoryRepository and gollem/trace.Repository for the AgentSession
// flow.
//
// Object layout under the configured bucket:
//
//	{prefix}/agent_sessions/{sessionID}/history.json
//	{prefix}/agent_sessions/{sessionID}/traces/{traceID}.json
//
// sessionID is the AgentSession.ID (UUIDv7) and is passed verbatim by the
// usecase as the gollem session identifier.
package agentarchive

import (
	"path"
	"strings"
)

const baseDir = "agent_sessions"

// objectPath joins the optional prefix with the agent_sessions root and the
// remaining segments using forward slashes (Cloud Storage object names are
// always slash-separated regardless of OS).
func objectPath(prefix string, parts ...string) string {
	segments := []string{}
	if prefix = strings.Trim(prefix, "/"); prefix != "" {
		segments = append(segments, prefix)
	}
	segments = append(segments, baseDir)
	segments = append(segments, parts...)
	return path.Join(segments...)
}
