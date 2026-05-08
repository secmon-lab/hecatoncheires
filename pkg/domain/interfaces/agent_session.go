package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// AgentSessionRepository persists AgentSession metadata.
//
// Lookup is by (workspaceID, caseID, threadTS) which uniquely identifies a
// Slack thread within a Case. Get returns (nil, nil) when no session exists.
type AgentSessionRepository interface {
	Get(ctx context.Context, workspaceID string, caseID int64, threadTS string) (*model.AgentSession, error)
	Put(ctx context.Context, s *model.AgentSession) error
}
