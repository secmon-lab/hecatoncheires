package proposal

import (
	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// TurnRequest is the input for one case-draft turn.
type TurnRequest struct {
	// Session is the per-thread Session row. Its ID is the turn's subject, so it
	// must already be persisted before a turn can be spawned.
	Session *model.Session

	// UserInput is the agent's first user message. For app_mention this is the
	// mention text; for thread_reply the reply text; for ws_switch a synthetic
	// system-event sentence.
	UserInput string

	// Trigger discriminates the entry point — it drives prompt hints.
	Trigger Trigger

	// TriggerTS is the Slack TS the turn is deduplicated on. It is empty for the
	// synthetic ws_switch trigger, which has no Slack event behind it.
	TriggerTS string

	// ActorUserID is the Slack user who initiated the turn. It is the run's access
	// actor: without it the usecase layer reads the run as a system context and
	// bypasses private-case access control.
	ActorUserID string

	// ExistingProposal is the prior draft persisted by the host (when this turn
	// resumes an existing draft, e.g. ws-switch or thread reply). The agent does
	// not consume it directly — the host uses it to keep preview state coherent
	// across turns.
	ExistingProposal *model.CaseProposal

	// ProcessingTS and PreviewTS name the Slack message this turn's result
	// replaces, and are mutually exclusive: the "working on it" placeholder a fresh
	// mention posted, or the existing preview a workspace switch updates in place.
	//
	// They are carried on the run because the call that posted them returns before
	// the result exists.
	ProcessingTS string
	PreviewTS    string

	// InheritFrom continues a finished run's conversation in this one. It is how an
	// answered question resumes: the answering turn is a NEW run — its own budget,
	// its own record — but it must see the request, the investigation and the
	// question that produced it. Empty starts a fresh conversation.
	InheritFrom string
}

// Status discriminates what StartTurn did.
type Status int

const (
	// StatusStarted means the turn was spawned; its draft or question is delivered
	// by the run's own completion handler, so the caller has nothing to post.
	StatusStarted Status = iota
	// StatusBusy means another turn holds this thread.
	StatusBusy
	// StatusIdempotent means the trigger duplicates a turn already started; drop
	// it silently.
	StatusIdempotent
)

// Result is the outcome of StartTurn.
type Result struct {
	Status Status
}

func validateTurnRequest(req *TurnRequest) error {
	if req == nil {
		return goerr.New("request is nil")
	}
	if req.Session == nil {
		return goerr.New("Session is required")
	}
	// TriggerTS may be empty for synthetic triggers (ws-switch), which carry no
	// Slack event to deduplicate on.
	return nil
}
