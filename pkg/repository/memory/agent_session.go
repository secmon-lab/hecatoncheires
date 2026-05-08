package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type agentSessionRepository struct {
	mu       sync.RWMutex
	sessions map[string]model.AgentSession
}

var _ interfaces.AgentSessionRepository = &agentSessionRepository{}

func newAgentSessionRepository() *agentSessionRepository {
	return &agentSessionRepository{
		sessions: make(map[string]model.AgentSession),
	}
}

func agentSessionKey(workspaceID string, caseID int64, threadTS string) string {
	return fmt.Sprintf("%s/%d/%s", workspaceID, caseID, threadTS)
}

func (r *agentSessionRepository) Get(_ context.Context, workspaceID string, caseID int64, threadTS string) (*model.AgentSession, error) {
	if workspaceID == "" || caseID == 0 || threadTS == "" {
		return nil, goerr.New("workspaceID, caseID, threadTS are required")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sessions[agentSessionKey(workspaceID, caseID, threadTS)]
	if !ok {
		return nil, nil
	}
	copied := s
	return &copied, nil
}

func (r *agentSessionRepository) Put(_ context.Context, s *model.AgentSession) error {
	if s == nil {
		return goerr.New("agent session is nil")
	}
	if s.ID == "" || s.WorkspaceID == "" || s.CaseID == 0 || s.ThreadTS == "" {
		return goerr.New("agent session missing required fields",
			goerr.V("id", s.ID),
			goerr.V("workspace_id", s.WorkspaceID),
			goerr.V("case_id", s.CaseID),
			goerr.V("thread_ts", s.ThreadTS),
		)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[agentSessionKey(s.WorkspaceID, s.CaseID, s.ThreadTS)] = *s
	return nil
}
