package usecase

import (
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slackmodel "github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/threadcase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// slackUserMentionRe matches a Slack user-mention token: <@U123ABC> or
// <@U123ABC|display name> (W-prefixed Enterprise Grid IDs included). The capture
// group is the bare user ID; the optional |label part is ignored.
var slackUserMentionRe = regexp.MustCompile(`<@([UW][A-Z0-9]+)(?:\|[^>]*)?>`)

// firstSlackUserMention returns the user ID of the first Slack user-mention in
// text that is not in ignoreIDs, or "" if there is none. It is used to attribute
// a bot-relayed intake post (whose author is an app, not a person) to the human
// named in the body; ignoreIDs lets the caller skip our own bot's user ID so a
// form that @-mentions the bot before the requester does not misattribute the
// case to the bot.
func firstSlackUserMention(text string, ignoreIDs ...string) string {
	for _, m := range slackUserMentionRe.FindAllStringSubmatch(text, -1) {
		if len(m) < 2 {
			continue
		}
		if slices.Contains(ignoreIDs, m[1]) {
			continue
		}
		return m[1]
	}
	return ""
}

// HandleThreadCaseCreation processes a channel-root post (by a human or an
// integration bot) in a thread-mode monitored channel — the ONLY trigger that
// initiates case creation. For a bot-relayed post the reporter is best-effort
// resolved from the first Slack user mention in the body, and stays empty when
// none is present (thread-mode cases may have no reporter). It does
// NOT create a case immediately: it runs the initialization (create) agent,
// which investigates, may ask the user, and only commits a validated case once
// it is confident. On success it posts a Block Kit summary; on a question it
// posts an interactive form and waits; on fallback it posts a "couldn't
// conclude" message. Resuming after a question is driven by the question
// form's Submit interaction (HandleThreadCaseQuestionSubmit) — a free-text
// reply or mention in the not-yet-a-case thread is intentionally ignored.
func (uc *AgentUseCase) HandleThreadCaseCreation(ctx context.Context, msg *slackmodel.Message, entry *model.WorkspaceEntry) error {
	if uc.threadcase == nil || uc.deps.CaseUC == nil || entry == nil {
		return nil
	}
	wsID := entry.Workspace.ID
	channelID := msg.ChannelID()
	threadTS := msg.ID()
	reporter := msg.UserID()
	text := msg.Text()

	// Idempotency: a re-delivered Slack event for the same thread must not
	// start a second creation turn (the turn-lock dedups concurrent triggers,
	// and a committed case short-circuits here).
	existing, err := uc.deps.Repo.Case().GetBySlackThread(ctx, wsID, channelID, threadTS)
	if err != nil {
		return goerr.Wrap(err, "look up existing thread case",
			goerr.V("channel_id", channelID), goerr.V("thread_ts", threadTS))
	}
	if existing != nil {
		return nil
	}

	reporter = uc.resolveThreadCaseReporter(ctx, reporter, text)

	_, err = uc.runThreadCaseCreation(ctx, threadCreateReq(entry, channelID, threadTS, reporter,
		[]threadcase.ConversationMessage{{Timestamp: threadTS, UserID: reporter, Text: text}},
		nil, "", "", threadTS, ""))
	return err
}

// resolveThreadCaseReporter returns the reporter for a new thread-mode case.
// The event author is used as-is when present. Body inference is the EXCEPTION,
// used only when the trigger has no human author — a channel-root intake form
// relayed by an integration bot, or a bot-authored @mention. There we
// best-effort attribute the case to the human named in the body (the first
// Slack user mention, typically the requester), skipping our own bot's user ID
// so a form that @-mentions the bot before the requester does not misattribute
// the case to the bot. If none is present the reporter stays empty: a
// thread-mode case is allowed to have no reporter (see model.Case.ValidateNew),
// so creation still proceeds.
func (uc *AgentUseCase) resolveThreadCaseReporter(ctx context.Context, author, text string) string {
	if author != "" {
		return author
	}
	return firstSlackUserMention(text, uc.botUserID(ctx))
}

// botUserID returns the bot's own Slack user ID, or "" when the Slack service is
// unavailable or the lookup fails. Best-effort: callers use it only to skip the
// bot's own messages, so an empty result degrades to "do not skip".
func (uc *AgentUseCase) botUserID(ctx context.Context) string {
	if uc.deps.SlackService == nil {
		return ""
	}
	id, err := uc.deps.SlackService.GetBotUserID(ctx)
	if err != nil {
		return ""
	}
	return id
}

// HandleThreadCaseMentionCreation starts (or resumes) a thread-mode Case from an
// @mention in a monitored channel that has no Case yet. It is the mention-trigger
// counterpart of HandleThreadCaseCreation: the mention — at the channel root or
// inside a case-less thread — is the creation trigger, and the mention text is
// folded into the create agent's seed. The bot-authored / accept_bot gate is
// applied by the caller (handleThreadModeEvent) before this runs.
//
// The first mention on a thread seeds a fresh creation turn; a channel-root
// mention seeds the mention text alone (like instant), while an in-thread
// mention seeds the whole thread (root + replies + this mention). A follow-up
// mention while the thread is still not a Case resumes the in-flight session,
// superseding any pending question with the new intent (mirrors
// ResumeThreadCaseCreation). Per-thread serialization comes from the turn lock
// keyed on (channel, threadTS); TriggerTS is the mention's own TS so a
// re-delivered mention dedups without a follow-up being mistaken for a retry.
func (uc *AgentUseCase) HandleThreadCaseMentionCreation(ctx context.Context, msg *slackmodel.Message, entry *model.WorkspaceEntry) error {
	if uc.threadcase == nil || uc.deps.CaseUC == nil || entry == nil {
		return nil
	}
	wsID := entry.Workspace.ID
	channelID := msg.ChannelID()

	// Resolve the thread root: a channel-root mention roots a new thread at its
	// own TS; an in-thread mention binds to the existing thread root. app_mention
	// does not normalise thread_ts == ts, so treat that as a root too.
	threadTS := msg.ThreadTS()
	isRoot := threadTS == "" || threadTS == msg.ID()
	if isRoot {
		threadTS = msg.ID()
	}

	// Idempotency: once the thread is a Case, creation is done — a mention there
	// is handled by the investigation path (HandleThreadCaseMention), not here.
	existing, err := uc.deps.Repo.Case().GetBySlackThread(ctx, wsID, channelID, threadTS)
	if err != nil {
		return goerr.Wrap(err, "look up existing thread case",
			goerr.V("channel_id", channelID), goerr.V("thread_ts", threadTS))
	}
	if existing != nil {
		return nil
	}

	// Follow-up mention: a session already exists for this still-case-less
	// thread. Resume the in-flight turn with the new mention as the latest intent
	// (supersedes a pending question) instead of seeding a second fresh turn.
	// Re-scan the thread so plain replies the user added between mentions — which
	// trigger nothing and are recorded nowhere in mention mode — still reach the
	// create agent. partitionConversation returns only messages newer than the
	// last processed mention (keyed on Session.LastMentionTS), so the prior
	// turn's context is not re-injected. MentionTS is always passed (here and on
	// the first mention below) so that watermark advances.
	session, err := uc.deps.Repo.Session().GetByThread(ctx, channelID, threadTS)
	if err != nil {
		return goerr.Wrap(err, "look up session for mention creation",
			goerr.V("channel_id", channelID), goerr.V("thread_ts", threadTS))
	}
	if session != nil {
		// Keep the reporter the first turn already attributed (the thread's
		// originator for an in-thread mention), so a follow-up mention by a
		// different person does not overwrite it.
		reporter := uc.resolveThreadCaseReporter(ctx, session.CreatorUserID, msg.Text())
		_, delta, cerr := uc.partitionConversation(ctx, msg, session, uc.botUserID(ctx))
		if cerr != nil {
			return goerr.Wrap(cerr, "partition conversation for mention follow-up",
				goerr.V("channel_id", channelID), goerr.V("thread_ts", threadTS))
		}
		_, err = uc.runThreadCaseCreation(ctx, threadCreateReq(entry, channelID, threadTS, reporter,
			nil, toThreadcaseMessages(delta), msg.Text(), msg.ID(), msg.ID(), ""))
		return err
	}

	// First mention: seed a fresh creation turn. A channel-root mention is its
	// own seed; an in-thread mention pulls the whole thread (root + replies) as
	// context. In both cases the mention text is surfaced separately as the
	// current intent, and MentionTS advances the delta watermark for follow-ups.
	if isRoot {
		// The mentioner is the root author, so they are the reporter.
		reporter := uc.resolveThreadCaseReporter(ctx, msg.UserID(), msg.Text())
		_, err = uc.runThreadCaseCreation(ctx, threadCreateReq(entry, channelID, threadTS, reporter,
			nil, nil, msg.Text(), msg.ID(), msg.ID(), ""))
		return err
	}

	ctxMsgs, cerr := uc.collectContextMessages(ctx, msg)
	if cerr != nil {
		return goerr.Wrap(cerr, "collect thread context for mention creation",
			goerr.V("channel_id", channelID), goerr.V("thread_ts", threadTS))
	}
	// Prefer the thread's originator (the person who raised the issue) as the
	// reporter over the mentioner, who may just be triaging someone else's
	// thread. Fall back to the mentioner, then to body inference.
	rootAuthor := msg.UserID()
	if len(ctxMsgs) > 0 && ctxMsgs[0].UserID != "" {
		rootAuthor = ctxMsgs[0].UserID
	}
	reporter := uc.resolveThreadCaseReporter(ctx, rootAuthor, msg.Text())
	_, err = uc.runThreadCaseCreation(ctx, threadCreateReq(entry, channelID, threadTS, reporter,
		toThreadcaseMessages(ctxMsgs), nil, msg.Text(), msg.ID(), msg.ID(), ""))
	return err
}

// ResumeThreadCaseCreation continues the initialization (create) agent on a
// thread that has no case yet, with msg as the latest user intent; the
// conversation history (keyed on Session.ID) carries the prior turn's
// investigation and question. In production this is reached only through the
// offline eval harness — the live Slack flow resumes a deferred question via
// the question form's Submit interaction, and free-text replies / mentions in
// a not-yet-a-case thread are ignored (see handleThreadModeEvent).
func (uc *AgentUseCase) ResumeThreadCaseCreation(ctx context.Context, msg *slackmodel.Message, entry *model.WorkspaceEntry) error {
	if uc.threadcase == nil || uc.deps.CaseUC == nil || entry == nil {
		return nil
	}
	wsID := entry.Workspace.ID
	channelID := msg.ChannelID()
	threadTS := msg.ThreadTS()
	if threadTS == "" {
		threadTS = msg.ID()
	}
	reporter := msg.UserID()

	// If a case already exists for this thread, the mention flow owns it.
	existing, err := uc.deps.Repo.Case().GetBySlackThread(ctx, wsID, channelID, threadTS)
	if err != nil {
		return goerr.Wrap(err, "look up existing thread case",
			goerr.V("channel_id", channelID), goerr.V("thread_ts", threadTS))
	}
	if existing != nil {
		return nil
	}

	_, err = uc.runThreadCaseCreation(ctx, threadCreateReq(entry, channelID, threadTS, reporter,
		nil, nil, msg.Text(), msg.ID(), msg.ID(), ""))
	return err
}

// caseCreateReq parameterizes the shared thread-mode case-creation flow. It
// separates two threads that are usually the same:
//
//   - caseChannel/caseTS — the thread the Case is bound to (the session key, the
//     GetBySlackThread key, the CreateThreadCaseWithFields target). Post-creation
//     mentions and history continue here.
//   - uiChannel/uiTS — where the creation dialog surfaces (progress trace,
//     question form, and the completion link).
//
// For normal thread-mode creation and same-channel reactions the two are equal.
// For a cross-channel reaction they diverge: the dialog lives in the reactor's
// source thread while the Case lives in the workspace's monitored channel.
type caseCreateReq struct {
	entry                         *model.WorkspaceEntry
	caseChannel, caseTS           string
	uiChannel, uiTS               string
	reporter                      string
	systemMessages, deltaMessages []threadcase.ConversationMessage
	mentionText, mentionTS        string
	triggerTS                     string
	// createInstruction is appended to the ModeCreate planner prompt (trigger
	// context). Empty for normal thread creation.
	createInstruction string
	// sourceChannel/sourceTS carry the reaction-flagged source message for a
	// cross-channel reaction so runThreadCaseCreation can persist it on the
	// Session (for the origin reply's exact link). Empty for every other path,
	// including a resume turn — there the Session already holds the reference.
	sourceChannel, sourceTS string
}

// sameThread reports whether the UI and case threads coincide (the normal /
// same-channel case). When false, the creation dialog is cross-channel.
func (r caseCreateReq) sameThread() bool {
	return r.uiChannel == r.caseChannel && r.uiTS == r.caseTS
}

// threadCreateReq builds a caseCreateReq for the common single-thread case where
// the creation dialog surfaces in the same thread the Case is bound to (normal
// thread-mode creation, resume, and same-channel reactions).
func threadCreateReq(entry *model.WorkspaceEntry, channelID, threadTS, reporter string,
	systemMessages, deltaMessages []threadcase.ConversationMessage,
	mentionText, mentionTS, triggerTS, createInstruction string) caseCreateReq {
	return caseCreateReq{
		entry:             entry,
		caseChannel:       channelID,
		caseTS:            threadTS,
		uiChannel:         channelID,
		uiTS:              threadTS,
		reporter:          reporter,
		systemMessages:    systemMessages,
		deltaMessages:     deltaMessages,
		mentionText:       mentionText,
		mentionTS:         mentionTS,
		triggerTS:         triggerTS,
		createInstruction: createInstruction,
	}
}

// runThreadCaseCreation is the shared body for the initial post, resume, and
// reaction paths. systemMessages seeds the first turn; deltaMessages carries
// thread messages newer than the last processed mention (mention follow-ups);
// mentionText / mentionTS feed the current mention. It returns the terminal
// turn status so a caller (e.g. the cross-channel reaction path) can react to a
// fallback; the status is StatusFallback on an internal error.
func (uc *AgentUseCase) runThreadCaseCreation(ctx context.Context, req caseCreateReq) (threadcase.Status, error) {
	wsID := req.entry.Workspace.ID

	session, err := uc.loadOrCreateSession(ctx, wsID, 0, req.caseChannel, req.caseTS)
	if err != nil {
		// Tell the user what went wrong instead of failing silently — this runs
		// before the progress trace exists, so post directly to the UI thread.
		uc.replyUserError(ctx, err, "thread case: load session for create", req.uiChannel, req.uiTS)
		return threadcase.StatusFallback, nil
	}
	// The session predates the case; record the reporter so the create handler
	// can attribute the case even on a resume turn.
	if session.CreatorUserID == "" {
		session.CreatorUserID = req.reporter
	}
	// Record the reaction-flagged source message on a fresh session (cross-channel
	// reaction only) so the origin reply can link the exact source even on a
	// resume turn, where req no longer carries it. Do not overwrite an existing
	// reference (a resume turn already has it persisted).
	if req.sourceTS != "" && session.ReactionSourceMessageTS == "" {
		session.ReactionSourceChannelID = req.sourceChannel
		session.ReactionSourceMessageTS = req.sourceTS
	}

	// Supersede: when a reply / mention resumes the flow while a question form
	// is still pending, mark that form stale (removing its Submit button) so it
	// can no longer be answered. The form was posted to the UI thread, recorded
	// on the pending snapshot, so stale it there (which may differ from the case
	// thread on a cross-channel reaction).
	if req.mentionText != "" && session.PendingQuestion != nil && session.PendingQuestion.PostedMessageTS != "" {
		postedChannel := session.PendingQuestion.PostedChannelID
		if postedChannel == "" {
			// Defensive: an older session may not have recorded the form's channel;
			// the form is posted in the UI thread, so fall back to it.
			postedChannel = req.uiChannel
		}
		uc.markThreadQuestionStale(ctx, postedChannel, session.PendingQuestion.PostedMessageTS)
	}

	traceMsg := uc.newTraceMessage(req.uiChannel, req.uiTS)
	// Immediate progress so the user is not left staring at silence while the
	// agent investigates.
	traceMsg.appendLine(ctx, i18n.T(ctx, i18n.MsgThreadCaseCreating))

	res, runErr := uc.threadcase.RunTurn(ctx, threadcase.TurnRequest{
		Session:           session,
		Workspace:         req.entry,
		Case:              nil,
		ChannelID:         req.caseChannel,
		ThreadTS:          req.caseTS,
		MentionText:       req.mentionText,
		MentionTS:         req.mentionTS,
		TriggerTS:         req.triggerTS,
		Mode:              threadcase.ModeCreate,
		SystemMessages:    req.systemMessages,
		DeltaMessages:     req.deltaMessages,
		CreateInstruction: req.createInstruction,
		Handler:           uc.newThreadcaseCreateHandler(req, traceMsg),
	})
	if runErr != nil {
		uc.replyUserError(ctx, runErr, "thread case create turn", req.uiChannel, req.uiTS)
		return threadcase.StatusFallback, nil
	}

	switch res.Status {
	case threadcase.StatusCompleted:
		if res.Case == nil {
			return res.Status, nil
		}
		uc.bindSessionToCase(ctx, req.caseChannel, req.caseTS, res.Case.ID)
		uc.postCreatedCaseOutcome(ctx, req, session, res.Case)
	case threadcase.StatusQuestion:
		// The question form was posted by the handler; wait for the user to
		// answer it via the form's Submit interaction (HandleThreadCaseQuestionSubmit).
	case threadcase.StatusFallback:
		uc.finalizeTrace(ctx, traceMsg, req.uiChannel, req.uiTS,
			uc.userErrorText(ctx, fallbackReasonError(res.FallbackReason), "thread case create fallback"))
	case threadcase.StatusBusy, threadcase.StatusIdempotent:
		// Another turn owns this thread, or a duplicate trigger — drop.
	}
	return res.Status, nil
}

// postCreatedCaseOutcome posts the case-creation outcome. For a normal / same-
// channel creation (UI thread == case thread) it posts the summary as a reply
// under the existing thread root. For a cross-channel reaction (UI thread !=
// case thread) the case root is a bot-posted placeholder, so it replaces that
// placeholder in place with the shared summary, posts the reaction-specific
// origin (reporter + exact source link) as a reply under it, and posts a
// back-link in the reactor's source thread so they get the case URL without
// leaving their channel. The summary itself is identical across all creation
// paths; reaction-specific context lives only in the origin reply.
func (uc *AgentUseCase) postCreatedCaseOutcome(ctx context.Context, req caseCreateReq, session *model.Session, c *model.Case) {
	wsID := req.entry.Workspace.ID
	if req.sameThread() {
		uc.postThreadCaseSummary(ctx, wsID, req.entry, c, req.caseChannel, req.caseTS)
		return
	}
	// Cross-channel reaction: caseTS is the bot's placeholder root.
	uc.updateRootCaseSummary(ctx, wsID, req.entry, c, req.caseChannel, req.caseTS)
	uc.postReactionOriginReply(ctx, req.caseChannel, req.caseTS, req.reporter, session)
	url := uc.deps.CaseUC.CaseURL(wsID, c.ID)
	threadLink := uc.slackPermalink(ctx, req.caseChannel, req.caseTS)
	uc.postThreadReply(ctx, req.uiChannel, req.uiTS, i18n.T(ctx, i18n.MsgReactionCaseBacklink, url, threadLink))
}

// updateRootCaseSummary replaces the cross-channel placeholder root with the
// shared case summary in place, so the monitored channel shows a single root
// message identical to any other creation path. If the Block Kit update fails
// (e.g. block validation / payload size), it retries updating the same root with
// plain text so the root never stays stuck on the "Creating a case…"
// placeholder; only if that also fails does it fall back to a threaded reply so
// the summary is never lost.
func (uc *AgentUseCase) updateRootCaseSummary(ctx context.Context, wsID string, entry *model.WorkspaceEntry, c *model.Case, channelID, rootTS string) {
	if uc.deps.SlackService == nil {
		return
	}
	url := uc.deps.CaseUC.CaseURL(wsID, c.ID)
	blocks, fallback := buildThreadCaseSummaryBlocks(ctx, c, entry, url)
	if err := uc.deps.SlackService.UpdateMessage(ctx, channelID, rootTS, blocks, fallback); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "reaction: update root case summary with blocks",
			goerr.V("channel_id", channelID), goerr.V("root_ts", rootTS)), "reaction: update root case summary with blocks")
		// Retry with plain text (no blocks) so the placeholder root is not left
		// stuck; only fall back to a threaded reply if that also fails.
		if txtErr := uc.deps.SlackService.UpdateMessage(ctx, channelID, rootTS, nil, fallback); txtErr != nil {
			errutil.Handle(ctx, goerr.Wrap(txtErr, "reaction: update root case summary with text",
				goerr.V("channel_id", channelID), goerr.V("root_ts", rootTS)), "reaction: update root case summary with text")
			uc.postThreadReply(ctx, channelID, rootTS, fallback)
		}
	}
}

// postReactionOriginReply posts the reaction-specific origin (who flagged it and
// a link to the exact flagged message) as a reply under the summary root. The
// source message reference is read from the session, which was populated at the
// start of the creation turn, so this works on both the direct and the resume
// path. Best-effort: a missing permalink degrades to a reporter-only line.
func (uc *AgentUseCase) postReactionOriginReply(ctx context.Context, channelID, rootTS, reporterID string, session *model.Session) {
	if uc.deps.SlackService == nil {
		return
	}
	src := ""
	if session != nil && session.ReactionSourceMessageTS != "" {
		src = uc.slackPermalink(ctx, session.ReactionSourceChannelID, session.ReactionSourceMessageTS)
	}
	var text string
	if src != "" {
		text = i18n.T(ctx, i18n.MsgReactionCaseOrigin, reporterID, src)
	} else {
		text = i18n.T(ctx, i18n.MsgReactionCaseOriginNoLink, reporterID)
	}
	uc.postThreadReply(ctx, channelID, rootTS, text)
}

// slackPermalink returns the Slack permalink for a message, or "" when the
// Slack service is unavailable or the lookup fails (best-effort; the link
// message degrades to the case URL alone).
func (uc *AgentUseCase) slackPermalink(ctx context.Context, channelID, messageTS string) string {
	if uc.deps.SlackService == nil {
		return ""
	}
	link, err := uc.deps.SlackService.GetPermalink(ctx, channelID, messageTS)
	if err != nil {
		errutil.Handle(ctx, err, "thread case: get permalink")
		return ""
	}
	return link
}

// bindSessionToCase stamps the freshly created case id onto the thread's
// session (Session.ID stays stable so the gollem history stays continuous).
// It re-reads the session on purpose: RunTurn swaps its working session to the
// turn-lock's own object (`req.Session = handle.Session`) and persists the
// turn's LastAction / PendingQuestion there, so the caller's in-flight pointer
// is stale after the turn. Binding via that stale pointer would clobber the
// turn's writes (e.g. re-persist LastAction == question after a resume created
// the case, which would loop a driver waiting on that flag). Best-effort: a
// failure here only means later mentions re-resolve the case by thread lookup.
func (uc *AgentUseCase) bindSessionToCase(ctx context.Context, channelID, threadTS string, caseID int64) {
	ssn, err := uc.deps.Repo.Session().GetByThread(ctx, channelID, threadTS)
	if err != nil || ssn == nil {
		if err != nil {
			errutil.Handle(ctx, err, "thread case: reload session to bind case")
		}
		return
	}
	if ssn.CaseID == caseID && ssn.PendingQuestion == nil {
		return
	}
	ssn.CaseID = caseID
	// The case is created; no question is outstanding anymore.
	ssn.PendingQuestion = nil
	if perr := uc.deps.Repo.Session().Put(ctx, ssn); perr != nil {
		errutil.Handle(ctx, perr, "thread case: bind session to case")
	}
}

// postThreadCaseSummary posts the Block Kit summary of the just-created case as
// a new reply in the case thread. The progress ("creating…") trace message is
// left in place — the summary is a separate message, not a finalization of the
// trace.
func (uc *AgentUseCase) postThreadCaseSummary(ctx context.Context, wsID string, entry *model.WorkspaceEntry, c *model.Case, channelID, threadTS string) {
	if uc.deps.SlackService == nil {
		return
	}
	url := uc.deps.CaseUC.CaseURL(wsID, c.ID)
	blocks, fallback := buildThreadCaseSummaryBlocks(ctx, c, entry, url)
	if _, err := uc.deps.SlackService.PostThreadMessage(ctx, channelID, threadTS, blocks, fallback); err != nil {
		errutil.Handle(ctx, err, "thread case: post create summary")
		// Fall back to a plain text reply so the user still gets the link.
		uc.postThreadReply(ctx, channelID, threadTS, fallback)
	}
}

// HandleThreadCaseMention processes an app_mention inside a thread-mode case
// thread: it runs the investigation agent and applies the resulting decision
// (respond / materialize / close).
func (uc *AgentUseCase) HandleThreadCaseMention(ctx context.Context, msg *slackmodel.Message, entry *model.WorkspaceEntry, foundCase *model.Case) error {
	if uc.threadcase == nil || entry == nil || foundCase == nil {
		return nil
	}
	logger := logging.From(ctx)
	wsID := entry.Workspace.ID
	channelID := msg.ChannelID()
	threadTS := msg.ThreadTS()
	if threadTS == "" {
		threadTS = msg.ID()
	}

	botUserID := ""
	if uc.deps.SlackService != nil {
		if id, berr := uc.deps.SlackService.GetBotUserID(ctx); berr == nil {
			botUserID = id
		}
	}

	session, err := uc.loadOrCreateSession(ctx, wsID, foundCase.ID, channelID, threadTS)
	if err != nil {
		return goerr.Wrap(err, "thread case: load session for mention")
	}

	systemMessages, deltaMessages, err := uc.partitionConversation(ctx, msg, session, botUserID)
	if err != nil {
		return goerr.Wrap(err, "thread case: partition conversation")
	}

	traceMsg := uc.newTraceMessage(channelID, threadTS)
	res, runErr := uc.threadcase.RunTurn(ctx, threadcase.TurnRequest{
		Session:        session,
		Workspace:      entry,
		Case:           foundCase,
		ChannelID:      channelID,
		ThreadTS:       threadTS,
		MentionTS:      msg.ID(),
		MentionText:    msg.Text(),
		SystemMessages: toThreadcaseMessages(systemMessages),
		DeltaMessages:  toThreadcaseMessages(deltaMessages),
		TriggerTS:      msg.ID(),
		Mode:           threadcase.ModeMention,
		Handler:        uc.newThreadcaseHandler(channelID, threadTS, traceMsg),
	})
	if runErr != nil {
		// replyUserError already reports to log/Sentry; return nil so the async
		// dispatcher does not re-Handle (double report) the same error.
		uc.replyUserError(ctx, runErr, "thread case mention turn", channelID, threadTS)
		return nil
	}

	switch res.Status {
	case threadcase.StatusBusy:
		uc.postThreadReply(ctx, channelID, threadTS, i18n.T(ctx, i18n.MsgKeyAgentBusy))
		return nil
	case threadcase.StatusIdempotent, threadcase.StatusQuestion:
		// Question already posted by the handler; idempotent drops silently.
		return nil
	case threadcase.StatusFallback:
		uc.replyUserError(ctx, fallbackReasonError(res.FallbackReason), "thread case mention fallback", channelID, threadTS)
		return nil
	case threadcase.StatusCompleted:
		uc.applyMentionDecision(ctx, wsID, entry, foundCase.ID, channelID, threadTS, traceMsg, res.Decision)
		return nil
	default:
		logger.Warn("unexpected threadcase status", "status", int(res.Status))
		return nil
	}
}

// applyMentionDecision applies a mention-turn terminal decision and posts the
// appropriate reply to the thread. Closing / status changes are NOT handled
// here: the sub-agent performs them via the case__update_case_status tool during
// investigation (see pkg/usecase/agent/threadcase), so the only host-applied
// terminal outcomes are respond and materialize.
func (uc *AgentUseCase) applyMentionDecision(ctx context.Context, wsID string, entry *model.WorkspaceEntry, caseID int64, channelID, threadTS string, traceMsg *traceMessage, d *threadcase.Decision) {
	if d == nil {
		return
	}
	switch d.Kind {
	case threadcase.DecisionRespond:
		uc.finalizeTrace(ctx, traceMsg, channelID, threadTS, d.Message)
	case threadcase.DecisionMaterialize:
		if uc.deps.CaseUC != nil {
			fv := buildThreadFieldValues(entry, d.Fields)
			if _, err := uc.deps.CaseUC.MaterializeThreadCase(ctx, wsID, caseID, d.Title, d.Description, fv); err != nil {
				// Materialize failed — surface it instead of finalizing with the
				// success text, which would mislabel a failed update as done.
				uc.finalizeTrace(ctx, traceMsg, channelID, threadTS,
					uc.userErrorText(ctx, err, "thread case: materialize on mention"))
				return
			}
		}
		text := d.Message
		if text == "" {
			text = i18n.T(ctx, i18n.MsgThreadCaseUpdated)
		}
		uc.finalizeTrace(ctx, traceMsg, channelID, threadTS, text)
	}
}

// finalizeTrace posts the final reply, falling back to a direct thread reply
// when the trace message machinery is unavailable (e.g. nil Slack service).
func (uc *AgentUseCase) finalizeTrace(ctx context.Context, traceMsg *traceMessage, channelID, threadTS, text string) {
	if text == "" {
		return
	}
	if traceMsg != nil {
		if err := traceMsg.finalize(ctx, text); err != nil {
			errutil.Handle(ctx, err, "thread case: finalize trace")
		}
		return
	}
	uc.postThreadReply(ctx, channelID, threadTS, text)
}

// postThreadReply is a nil-safe helper around SlackService.PostThreadReply.
func (uc *AgentUseCase) postThreadReply(ctx context.Context, channelID, threadTS, text string) {
	if uc.deps.SlackService == nil || text == "" {
		return
	}
	if _, err := uc.deps.SlackService.PostThreadReply(ctx, channelID, threadTS, text); err != nil {
		errutil.Handle(ctx, err, "thread case: post thread reply")
	}
}

// newThreadcaseCreateHandler builds the host-side Handler for a ModeCreate
// turn. Create commits the validated case via CaseUC.CreateThreadCaseWithFields
// (the reporter / channel / thread identity is captured here, not carried in
// the payload). Question posts the planner's question to the thread.
func (uc *AgentUseCase) newThreadcaseCreateHandler(req caseCreateReq, traceMsg *traceMessage) threadcase.Handler {
	wsID := req.entry.Workspace.ID
	return threadcase.HandlerFuncs{
		TraceAppendFn: func(ctx context.Context, line string) {
			if traceMsg != nil {
				traceMsg.appendLine(ctx, line)
			}
		},
		TraceReplaceFn: func(ctx context.Context, line string) {
			if traceMsg != nil {
				traceMsg.replaceLine(ctx, line)
			}
		},
		QuestionFn: func(ctx context.Context, ssn *model.Session, q threadcase.QuestionPayload) error {
			// Post the interactive selection form to the UI thread and record the
			// snapshot on the session (PendingQuestion); the threadcase runtime
			// persists the session when the turn ends on this question. The Submit
			// button carries the case thread so the resume can find the session
			// regardless of where the form is displayed.
			return uc.postThreadCreateQuestionForm(ctx, ssn, req.uiChannel, req.uiTS, req.caseChannel, req.caseTS, req.reporter, q)
		},
		CreateFn: func(ctx context.Context, _ *model.Session, p threadcase.CreatePayload) (*model.Case, error) {
			return uc.deps.CaseUC.CreateThreadCaseWithFields(ctx, wsID, req.caseChannel, req.caseTS, req.reporter, p.Title, p.Description, p.Fields)
		},
	}
}

// postThreadcaseQuestion renders a planner question as a thread reply. Shared
// by the mention and create handlers.
func (uc *AgentUseCase) postThreadcaseQuestion(ctx context.Context, channelID, threadTS string, q threadcase.QuestionPayload) error {
	if uc.deps.SlackService == nil {
		return nil
	}
	var b strings.Builder
	b.WriteString(i18n.T(ctx, i18n.MsgThreadCaseQuestion, q.Reason))
	for _, it := range q.Items {
		b.WriteString("\n• ")
		b.WriteString(it.Text)
		if len(it.Options) > 0 {
			b.WriteString(" (")
			b.WriteString(strings.Join(it.Options, " / "))
			b.WriteString(")")
		}
	}
	_, err := uc.deps.SlackService.PostThreadReply(ctx, channelID, threadTS, b.String())
	return err
}

// newThreadcaseHandler builds the host-side Handler for one thread-mode turn.
func (uc *AgentUseCase) newThreadcaseHandler(channelID, threadTS string, traceMsg *traceMessage) threadcase.Handler {
	return threadcase.HandlerFuncs{
		TraceAppendFn: func(ctx context.Context, line string) {
			if traceMsg != nil {
				traceMsg.appendLine(ctx, line)
			}
		},
		TraceReplaceFn: func(ctx context.Context, line string) {
			if traceMsg != nil {
				traceMsg.replaceLine(ctx, line)
			}
		},
		QuestionFn: func(ctx context.Context, _ *model.Session, q threadcase.QuestionPayload) error {
			return uc.postThreadcaseQuestion(ctx, channelID, threadTS, q)
		},
	}
}

// buildThreadFieldValues maps the agent's DecisionField list into typed
// FieldValue entries using the workspace field schema. Unknown field ids and
// empty values are dropped; the resolved Type lets the case validator accept
// them on write.
func buildThreadFieldValues(entry *model.WorkspaceEntry, fields []threadcase.DecisionField) map[string]model.FieldValue {
	if entry == nil || entry.FieldSchema == nil || len(fields) == 0 {
		return nil
	}
	typeByID := make(map[string]types.FieldType, len(entry.FieldSchema.Fields))
	for _, f := range entry.FieldSchema.Fields {
		typeByID[f.ID] = types.FieldType(f.Type)
	}
	out := make(map[string]model.FieldValue, len(fields))
	for _, df := range fields {
		ft, ok := typeByID[df.FieldID]
		if !ok {
			continue
		}
		var val any
		switch ft {
		case types.FieldTypeMultiSelect:
			switch {
			case len(df.Values) > 0:
				val = df.Values
			case df.Value != "":
				val = []string{df.Value}
			default:
				continue
			}
		case types.FieldTypeNumber:
			if df.Value == "" {
				continue
			}
			// Number fields are stored as float64; the LLM emits the value as a
			// string, so parse it. A non-numeric value is dropped rather than
			// written as a string the validator / storage would reject.
			n, parseErr := strconv.ParseFloat(strings.TrimSpace(df.Value), 64)
			if parseErr != nil {
				continue
			}
			val = n
		default:
			if df.Value == "" {
				continue
			}
			val = df.Value
		}
		out[df.FieldID] = model.FieldValue{FieldID: types.FieldID(df.FieldID), Type: ft, Value: val}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// toThreadcaseMessages converts the Slack-service conversation shape into the
// Slack-independent shape consumed by the threadcase runtime.
func toThreadcaseMessages(in []slack.ConversationMessage) []threadcase.ConversationMessage {
	if len(in) == 0 {
		return nil
	}
	out := make([]threadcase.ConversationMessage, len(in))
	for i, m := range in {
		out[i] = threadcase.ConversationMessage{
			UserID:    m.UserID,
			UserName:  m.UserName,
			Text:      m.Text,
			Timestamp: m.Timestamp,
		}
	}
	return out
}
