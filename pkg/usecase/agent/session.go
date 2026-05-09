package agent

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// LoadOrCreateSessionInput collects the per-call state used to materialise a
// fresh Session if one does not yet exist. It mirrors the shape of the legacy
// AgentUseCase.loadOrCreateSession parameters and adds the open-mode fields
// for symmetry across the case-bound and draft modes.
type LoadOrCreateSessionInput struct {
	ChannelID string
	ThreadTS  string

	// Case-bound mode fields. Zero values when this is open-mode.
	WorkspaceID string
	CaseID      int64
	// DetectActionID, when non-nil, runs a side lookup for an Action whose
	// SlackMessageTS matches ThreadTS (used by case-bound mode to link the
	// session to the action thread). Pass nil to skip.
	DetectActionID func(ctx context.Context, workspaceID, threadTS string) (int64, error)

	// Open-mode fields. Zero values when case-bound.
	CreatorUserID string
	DraftID       model.CaseDraftID
}

// LoadOrCreateSession fetches the Session for (ChannelID, ThreadTS) or
// builds a fresh in-memory one without persisting. Persistence happens at
// turn finalisation (mode-specific) so a half-failed turn never commits a
// new Session row.
//
// Errors only surface when the repository call itself fails. A missing
// Session is a normal "build a new one" branch.
func (d *CommonDeps) LoadOrCreateSession(ctx context.Context, in LoadOrCreateSessionInput) (*model.Session, error) {
	if d == nil {
		return nil, goerr.New("CommonDeps is nil")
	}
	if in.ChannelID == "" || in.ThreadTS == "" {
		return nil, goerr.New("ChannelID and ThreadTS are required",
			goerr.V("channel_id", in.ChannelID),
			goerr.V("thread_ts", in.ThreadTS),
		)
	}
	existing, err := d.Repo.Session().GetByThread(ctx, in.ChannelID, in.ThreadTS)
	if err != nil {
		return nil, goerr.Wrap(err, "load session by thread")
	}
	if existing != nil {
		return existing, nil
	}

	var actionID int64
	if in.DetectActionID != nil && in.WorkspaceID != "" {
		got, err := in.DetectActionID(ctx, in.WorkspaceID, in.ThreadTS)
		if err != nil {
			errutil.Handle(ctx, err, "detect action by thread TS for new session")
		} else {
			actionID = got
		}
	}

	now := time.Now().UTC()
	return &model.Session{
		ID:            uuid.Must(uuid.NewV7()).String(),
		ChannelID:     in.ChannelID,
		ThreadTS:      in.ThreadTS,
		WorkspaceID:   in.WorkspaceID,
		CaseID:        in.CaseID,
		ActionID:      actionID,
		CreatorUserID: in.CreatorUserID,
		DraftID:       in.DraftID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

// newTurnID returns a fresh UUID v7 for use as both the turn-lock owner
// identifier and the trace ID. UUID v7 is timestamp-prefixed so traces sort
// chronologically when listed.
func newTurnID() string {
	return uuid.Must(uuid.NewV7()).String()
}
