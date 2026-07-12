package usecase

import (
	"context"
	_ "embed"
	"strings"
	"text/template"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/slack-go/slack/slackevents"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/threadcase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// reactionContextWindow / reactionContextLimit bound how much surrounding
// conversation is pulled into the create-agent seed for a reaction on a
// non-threaded channel post. The agent has read tools to dig further, so this
// only needs to give it enough to orient (FR-8).
const (
	reactionContextWindow = 30 * time.Minute
	reactionContextLimit  = 15
)

//go:embed prompts/reaction_create_context.md
var reactionCreateContextTmplText string

var reactionCreateContextTmpl = template.Must(
	template.New("reaction_create_context").Parse(reactionCreateContextTmplText))

// renderReactionCreateInstruction renders the ModeCreate trigger-context prompt
// that tells the create agent to read the conversation around the anchored
// message. Best-effort: an (unexpected) render failure degrades to no extra
// instruction rather than failing the turn.
func renderReactionCreateInstruction(ctx context.Context, anchorTS string) string {
	var buf strings.Builder
	if err := reactionCreateContextTmpl.Execute(&buf, struct{ AnchorTS string }{AnchorTS: anchorTS}); err != nil {
		errutil.Handle(ctx, err, "reaction: render create instruction")
		return ""
	}
	return buf.String()
}

// normalizeReactionName reduces a Slack reaction name to its base emoji,
// dropping a skin-tone modifier (e.g. "wave::skin-tone-2" → "wave") so it
// matches the configured trigger emoji.
func normalizeReactionName(s string) string {
	if base, _, found := strings.Cut(s, "::"); found {
		return base
	}
	return s
}

// handleReactionEvent dispatches a reaction_added event: it resolves the
// workspace from the emoji, guards against self-loops, and routes to the
// same-channel or cross-channel creation path. It is invoked from
// HandleSlackEvent before the message-conversion path, mirroring the membership
// event branch.
func (uc *SlackUseCases) handleReactionEvent(ctx context.Context, event *slackevents.EventsAPIEvent) error {
	ev, ok := event.InnerEvent.Data.(*slackevents.ReactionAddedEvent)
	if !ok {
		return nil
	}
	if uc.agent == nil || uc.registry == nil || uc.slackService == nil {
		return nil
	}

	entry, ok := uc.registry.FindByReactionEmoji(normalizeReactionName(ev.Reaction))
	if !ok {
		// Not a trigger emoji for any workspace — ignore silently.
		return nil
	}

	// Self-loop guard: never react to our own reactions, nor to reactions placed
	// on our own posts (a case summary, a seed root), which would otherwise nest
	// a case inside a case.
	if botUserID, err := uc.slackService.GetBotUserID(ctx); err == nil && botUserID != "" {
		if ev.User == botUserID || ev.ItemUser == botUserID {
			return nil
		}
	}

	srcChannel := ev.Item.Channel
	srcTS := ev.Item.Timestamp
	if srcChannel == "" || srcTS == "" {
		return nil
	}
	reporter := ev.User
	if reporter == "" {
		return nil
	}
	ctx = uc.contextWithUserLang(ctx, reporter)

	if srcChannel == entry.SlackMonitorChannelID {
		return uc.agent.reactionCreateSameChannel(ctx, entry, reporter, srcChannel, srcTS)
	}
	return uc.agent.reactionCreateCrossChannel(ctx, entry, reporter, srcChannel, srcTS)
}

// reactionCreateSameChannel handles a reaction placed on a message inside the
// workspace's own monitored channel (FR-2b): the reacted message's thread
// becomes the case thread directly, reusing the normal single-thread creation
// path. Idempotency comes from the existing turn lock plus GetBySlackThread, so
// no ReactionClaim is needed here.
func (uc *AgentUseCase) reactionCreateSameChannel(ctx context.Context, entry *model.WorkspaceEntry, reporter, channelID, srcTS string) error {
	if uc.threadcase == nil || uc.deps.CaseUC == nil || entry == nil {
		return nil
	}
	wsID := entry.Workspace.ID

	msgs, anchorTS, threadRoot := uc.fetchReactionContext(ctx, channelID, srcTS)

	existing, err := uc.deps.Repo.Case().GetBySlackThread(ctx, wsID, channelID, threadRoot)
	if err != nil {
		return goerr.Wrap(err, "reaction same-channel: look up existing case",
			goerr.V("channel_id", channelID), goerr.V("thread_ts", threadRoot))
	}
	if existing != nil {
		// The thread is already a case; a reaction on another of its messages is
		// a no-op (the expected behavior for a second reaction in the thread).
		return nil
	}

	_, err = uc.runThreadCaseCreation(ctx, threadCreateReq(entry, channelID, threadRoot, reporter,
		msgs, nil, "", "", threadRoot, renderReactionCreateInstruction(ctx, anchorTS)))
	return err
}

// reactionCreateCrossChannel handles a reaction placed on a message outside the
// workspace's monitored channel (FR-2): it claims the source message (dedup),
// posts a seed root in the monitored channel, and runs the creation dialog in
// the reactor's source thread while binding the Case to the seed thread.
func (uc *AgentUseCase) reactionCreateCrossChannel(ctx context.Context, entry *model.WorkspaceEntry, reporter, srcChannel, srcTS string) error {
	if uc.threadcase == nil || uc.deps.CaseUC == nil || uc.deps.SlackService == nil || entry == nil {
		return nil
	}
	wsID := entry.Workspace.ID

	claimed, err := uc.deps.Repo.ReactionClaim().Claim(ctx, wsID, srcChannel, srcTS)
	if err != nil {
		return goerr.Wrap(err, "reaction cross-channel: claim source",
			goerr.V("channel_id", srcChannel), goerr.V("message_ts", srcTS))
	}
	if !claimed {
		// A concurrent / duplicate reaction already owns this source message.
		return nil
	}

	msgs, anchorTS, uiRoot := uc.fetchReactionContext(ctx, srcChannel, srcTS)
	permalink := uc.slackPermalink(ctx, srcChannel, srcTS)

	dest := entry.SlackMonitorChannelID
	seedText := i18n.T(ctx, i18n.MsgReactionSeedRoot, reporter, permalink)
	seedTS, perr := uc.deps.SlackService.PostMessage(ctx, dest, nil, seedText)
	if perr != nil {
		uc.releaseReactionClaim(ctx, wsID, srcChannel, srcTS)
		// The reactor pressed the emoji but the seed post failed — tell them in
		// their own thread instead of leaving the reaction with no response. The
		// Slack client attached the classification (e.g. not_in_channel) at the
		// origin, so this renders the actionable "invite the bot" message.
		uc.replyUserError(ctx, goerr.Wrap(perr, "reaction cross-channel: post seed root",
			goerr.V("dest_channel", dest)), "reaction cross-channel: post seed root", srcChannel, uiRoot)
		return nil
	}

	st, _ := uc.runThreadCaseCreation(ctx, caseCreateReq{
		entry:             entry,
		caseChannel:       dest,
		caseTS:            seedTS,
		uiChannel:         srcChannel,
		uiTS:              uiRoot,
		reporter:          reporter,
		systemMessages:    msgs,
		triggerTS:         seedTS,
		createInstruction: renderReactionCreateInstruction(ctx, anchorTS),
	})
	// Release the claim only on a hard failure with no pending question, so a
	// future reaction can retry. A pending question (StatusQuestion) or a
	// committed case (StatusCompleted) keeps the claim.
	if st == threadcase.StatusFallback {
		uc.releaseReactionClaim(ctx, wsID, srcChannel, srcTS)
	}
	return nil
}

// releaseReactionClaim releases a claim best-effort, funnelling any error
// through errutil so a failed release does not mask the creation outcome.
func (uc *AgentUseCase) releaseReactionClaim(ctx context.Context, wsID, srcChannel, srcTS string) {
	if err := uc.deps.Repo.ReactionClaim().Release(ctx, wsID, srcChannel, srcTS); err != nil {
		errutil.Handle(ctx, err, "reaction: release claim")
	}
}

// fetchReactionContext returns the conversation around the reacted message to
// seed the create agent, plus the anchor ts and the thread root. When the
// reacted message is threaded it returns the whole thread (the thread root is
// the parent); otherwise it returns a bounded window of surrounding channel
// history (and the thread root is the message itself). All results are
// best-effort: on a fetch error it degrades to no seed messages.
func (uc *AgentUseCase) fetchReactionContext(ctx context.Context, channelID, ts string) (msgs []threadcase.ConversationMessage, anchorTS, threadRoot string) {
	anchorTS = ts
	threadRoot = ts
	if uc.deps.SlackService == nil {
		return nil, anchorTS, threadRoot
	}

	replies, err := uc.deps.SlackService.GetConversationReplies(ctx, channelID, ts, reactionContextLimit)
	if err != nil {
		errutil.Handle(ctx, err, "reaction: fetch conversation replies")
	} else if len(replies) > 1 {
		// Threaded: conversations.replies returns the parent first even when the
		// reacted message is a reply, so the parent ts is the thread root.
		if replies[0].Timestamp != "" {
			threadRoot = replies[0].Timestamp
		}
		return toThreadcaseMessages(replies), anchorTS, threadRoot
	}

	// Lone root message: pull a window of surrounding channel history so the
	// agent sees what came before and after the anchor.
	anchorTime := parseSlackTS(ts)
	hist, err := uc.deps.SlackService.GetConversationHistory(ctx, channelID, anchorTime.Add(-reactionContextWindow), reactionContextLimit)
	if err != nil {
		errutil.Handle(ctx, err, "reaction: fetch conversation history")
		return nil, anchorTS, threadRoot
	}
	out := toThreadcaseMessages(hist)
	// conversations.history is newest-first; present it oldest-first so the
	// agent reads the conversation in order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, anchorTS, threadRoot
}
