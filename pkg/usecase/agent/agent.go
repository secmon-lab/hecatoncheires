// Package agent contains the Slack-independent agent runtime shared by the
// `casebound` (Case-bound mention) and `draft` (open-mode case draft) modes,
// and reserved for the future `triage` mode that will run after Case
// creation. Slack SDK / pkg/service/slack imports are forbidden inside this
// package; modes must communicate with their host via small handler
// interfaces (e.g. proposal.Handler, casebound.Handler).
package agent

import (
	"context"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casewriter"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	githubtool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/github"
	knowledgetool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/knowledge"
	memotool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/memo"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/webfetch"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// CommonDeps groups dependencies shared across agent modes. Each mode's
// UseCase embeds (or holds a pointer to) one of these; mode-specific
// configuration (e.g. draft's plannerLoopMax) lives on the mode UseCase
// itself.
type CommonDeps struct {
	Repo        interfaces.Repository
	Registry    *model.WorkspaceRegistry
	LLMClient   gollem.LLMClient
	HistoryRepo gollem.HistoryRepository
	TraceRepo   trace.Repository

	// Optional sub-agent service / client deps. nil → the corresponding tool
	// set is empty.
	SlackSearch    slacktool.SearchService
	SlackBot       slacktool.BotService
	SlackRetriever slacktool.MessageRetriever
	NotionClient   notiontool.Client
	GitHubClient   *githubtool.Client
	WebFetchClient *webfetch.Client

	// Action mutator interfaces, used by `core` toolset. Required for
	// case-bound mode (which uses the full mutating tool set). Optional for
	// draft mode (read-only sub-agents).
	ActionUC     core.ActionMutator
	ActionStepUC core.ActionStepMutator

	// CaseUC backs the casewriter tools (case__update_case /
	// case__update_case_status) in case-bound mode. Optional: nil means the
	// case-bound agent cannot edit the case itself (the tools are not built).
	CaseUC casewriter.CaseMutator

	// MemoUC backs the Case-scoped memo tools (memo__*) in case-bound mode.
	// Optional: nil means the agent gets no memo tools.
	MemoUC memotool.MemoMutator

	// KnowledgeAccessor backs the read-only workspace knowledge tools
	// (knowledge__search/get/list_tags). When set, read access is always
	// offered. KnowledgeMutator backs the write tools (create/update); each
	// mode wires the write tools only when the agent is permitted to mutate
	// shared knowledge (i.e. not while processing a private case). Both are
	// optional: nil means the corresponding tools are not built.
	KnowledgeAccessor knowledgetool.KnowledgeAccessor
	KnowledgeMutator  knowledgetool.KnowledgeMutator

	// HeartbeatInterval / HeartbeatStaleAfter govern §5.3 turn-lock activity
	// detection. Zero values fall back to DefaultHeartbeatInterval /
	// DefaultHeartbeatStaleAfter.
	HeartbeatInterval   time.Duration
	HeartbeatStaleAfter time.Duration
}

// NewCommonDeps validates inputs and returns a populated CommonDeps. It
// does not enforce optional fields (slack/notion/github) but does require
// the core trio (Repo / LLMClient / HistoryRepo / TraceRepo) so wiring
// errors fail fast.
func NewCommonDeps(repo interfaces.Repository, llm gollem.LLMClient, historyRepo gollem.HistoryRepository, traceRepo trace.Repository) (*CommonDeps, error) {
	if repo == nil {
		return nil, goerr.New("repository is required")
	}
	if llm == nil {
		return nil, goerr.New("llm client is required")
	}
	if historyRepo == nil {
		return nil, goerr.New("history repository is required")
	}
	if traceRepo == nil {
		return nil, goerr.New("trace repository is required")
	}
	return &CommonDeps{
		Repo:                repo,
		LLMClient:           llm,
		HistoryRepo:         historyRepo,
		TraceRepo:           traceRepo,
		HeartbeatInterval:   DefaultHeartbeatInterval,
		HeartbeatStaleAfter: DefaultHeartbeatStaleAfter,
	}, nil
}

// TurnHandle bundles everything a mode's RunTurn body needs after acquiring
// the turn lock and starting the heartbeat goroutine. Callers MUST defer
// Release() before doing any work; the LIFO defer order guarantees the
// heartbeat goroutine stops before the lock is released.
type TurnHandle struct {
	// Ctx is the cancellable context for this turn. Heartbeat owner-mismatch
	// (and Phase B interrupt) cancels it via cancelTurn; the mode body should
	// honour ctx.Done() at every awaitable boundary.
	Ctx context.Context
	// Session reflects the post-acquire state (TurnOwnerID, TurnHeartbeatAt
	// already populated).
	Session *model.Session
	// OwnerID is what the mode passes back to ReleaseTurnLock / Heartbeat.
	OwnerID string
	// Acquired is true iff the turn body should run.
	Acquired bool
	// Reclaimed is true when the lock was claimed from a stale prior owner.
	Reclaimed bool
	// Idempotent is true when the current trigger matches the live owner's
	// trigger (Slack duplicate event); the mode should drop and return nil.
	Idempotent bool
	// BusyOwner, when not nil, signals the lock is held by a fresh owner;
	// the mode should call its host's PostBusy and return.
	BusyOwner *model.Session
	// Release stops the heartbeat goroutine and releases the lock. Always
	// safe to call (no-op when Acquired is false). Pass an outer-scoped ctx
	// (typically the parent ctx, not Ctx, so a turn cancellation does not
	// strand the release).
	Release func(ctx context.Context)
}

// StartTurn acquires the turn lock for (ssn.ChannelID, ssn.ThreadTS),
// starts the heartbeat goroutine, and returns a TurnHandle the mode driver
// can dispatch on. It does NOT panic on a nil/unset CommonDeps; instead a
// goerr is returned so the host can decide whether to propagate.
//
// The semantics for the four AcquireResult shapes are encoded in TurnHandle:
//   - Acquired=true → run body; defer Release.
//   - Idempotent=true → drop silently (no Slack post).
//   - BusyOwner!=nil → host should PostBusy; do NOT run body.
//   - error → goerr wrapped, surfaced to caller.
func (d *CommonDeps) StartTurn(parent context.Context, ssn *model.Session, triggerKey string) (*TurnHandle, error) {
	if d == nil {
		return nil, goerr.New("CommonDeps is nil")
	}
	if ssn == nil {
		return nil, goerr.New("session is nil")
	}
	if ssn.ChannelID == "" || ssn.ThreadTS == "" {
		return nil, goerr.New("session missing ChannelID/ThreadTS")
	}
	staleAfter := d.HeartbeatStaleAfter
	if staleAfter <= 0 {
		staleAfter = DefaultHeartbeatStaleAfter
	}
	// Each turn gets a fresh UUID v7 — used as both the turn-lock owner
	// identifier and the trace ID. triggerKey (Slack TS or "" for synthetic)
	// is passed through to the lock layer for Slack-side dedup only.
	ownerID := newTurnID()

	seed := func() *model.Session { return ssn }
	res, err := d.Repo.Session().AcquireTurnLock(parent,
		ssn.ChannelID, ssn.ThreadTS, triggerKey, ownerID, staleAfter, seed)
	if err != nil {
		return nil, goerr.Wrap(err, "acquire turn lock")
	}

	if !res.Acquired {
		if res.IdempotentRetry {
			return &TurnHandle{
				Session:    res.Session,
				OwnerID:    ownerID,
				Idempotent: true,
				Release:    func(_ context.Context) {},
			}, nil
		}
		return &TurnHandle{
			Session:   res.Session,
			OwnerID:   ownerID,
			BusyOwner: res.Session,
			Release:   func(_ context.Context) {},
		}, nil
	}

	turnCtx, cancelTurn := context.WithCancel(parent)
	stopHB := d.startHeartbeat(turnCtx, res.Session, ownerID, cancelTurn)

	released := false
	release := func(rctx context.Context) {
		if released {
			return
		}
		released = true
		// Order matters: stop the heartbeat first so it cannot race a
		// Heartbeat call against our Release on the same Session.
		stopHB()
		cancelTurn()
		if err := d.Repo.Session().ReleaseTurnLock(rctx, res.Session.ChannelID, res.Session.ThreadTS, ownerID); err != nil {
			errutil.Handle(rctx, err, "release turn lock")
		}
	}

	return &TurnHandle{
		Ctx:       turnCtx,
		Session:   res.Session,
		OwnerID:   ownerID,
		Acquired:  true,
		Reclaimed: res.Reclaimed,
		Release:   release,
	}, nil
}
