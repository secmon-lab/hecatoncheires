package firestore

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const agentSessionsCollection = "agent_sessions"

type agentSessionRepository struct {
	client *firestore.Client
}

var _ interfaces.AgentSessionRepository = &agentSessionRepository{}

func newAgentSessionRepository(client *firestore.Client) *agentSessionRepository {
	return &agentSessionRepository{client: client}
}

func (r *agentSessionRepository) docRef(workspaceID string, caseID int64, threadTS string) *firestore.DocumentRef {
	return r.client.
		Collection("workspaces").Doc(workspaceID).
		Collection("cases").Doc(fmt.Sprintf("%d", caseID)).
		Collection(agentSessionsCollection).Doc(threadTS)
}

func (r *agentSessionRepository) Get(ctx context.Context, workspaceID string, caseID int64, threadTS string) (*model.AgentSession, error) {
	if workspaceID == "" || caseID == 0 || threadTS == "" {
		return nil, goerr.New("workspaceID, caseID, threadTS are required",
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", caseID),
			goerr.V("thread_ts", threadTS),
		)
	}

	snap, err := r.docRef(workspaceID, caseID, threadTS).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, goerr.Wrap(err, "failed to get agent session",
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", caseID),
			goerr.V("thread_ts", threadTS),
		)
	}

	var s model.AgentSession
	if err := snap.DataTo(&s); err != nil {
		return nil, goerr.Wrap(err, "failed to decode agent session",
			goerr.V("doc_id", snap.Ref.ID),
		)
	}
	return &s, nil
}

func (r *agentSessionRepository) Put(ctx context.Context, s *model.AgentSession) error {
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

	if _, err := r.docRef(s.WorkspaceID, s.CaseID, s.ThreadTS).Set(ctx, s); err != nil {
		return goerr.Wrap(err, "failed to put agent session",
			goerr.V("workspace_id", s.WorkspaceID),
			goerr.V("case_id", s.CaseID),
			goerr.V("thread_ts", s.ThreadTS),
		)
	}
	return nil
}
