package usecase

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/proposal"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	goslack "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

// ErrNoAccessibleWorkspace is returned when the mentioning user has no
// workspace they can create a Case in.
var ErrNoAccessibleWorkspace = errors.New("no accessible workspace for user")

// ErrInferenceInProgress is returned when an interaction tries to operate on
// a draft whose Materializer call is still in flight.
var ErrInferenceInProgress = errors.New("draft inference in progress")

// MentionProposalUseCase handles app_mention events that occur in channels NOT
// bound to an existing Case. It funnels each mention into proposal.UseCase
// (the open-mode planner / sub-agent runtime), passing a per-mention
// slackDraftHandler that translates terminal actions and trace updates
// into Slack messages.
type MentionProposalUseCase struct {
	repo         interfaces.Repository
	registry     *model.WorkspaceRegistry
	slackService slacksvc.Service
	collector    *slacksvc.MessageCollector
	draftUC      *proposal.UseCase
}

// NewMentionProposalUseCase constructs a MentionProposalUseCase. All dependencies
// are mandatory; callers that cannot supply a Slack service should refrain
// from constructing this usecase (and the Slack interaction handler that
// depends on it) entirely.
func NewMentionProposalUseCase(
	repo interfaces.Repository,
	registry *model.WorkspaceRegistry,
	slackService slacksvc.Service,
	draftUC *proposal.UseCase,
) *MentionProposalUseCase {
	return &MentionProposalUseCase{
		repo:         repo,
		registry:     registry,
		slackService: slackService,
		collector:    slacksvc.NewMessageCollector(slackService),
		draftUC:      draftUC,
	}
}

// HandleAppMention runs the initial-mention flow: candidate workspace
// resolution → message collection → draft persistence → planner-driven
// turn (proposal.UseCase.RunTurn). The slackDraftHandler renders the planner's
// terminal action (post_message / post_question / materialize) into Slack.
//
// It is the caller's responsibility to ensure the channel is NOT bound to
// an existing Case (the dispatch in SlackUseCases handles that branch).
func (uc *MentionProposalUseCase) HandleAppMention(ctx context.Context, ev *slackevents.AppMentionEvent) error {
	if ev == nil {
		return goerr.New("AppMentionEvent is nil")
	}
	if uc.draftUC == nil {
		return goerr.New("draft usecase is not configured")
	}
	logger := logging.From(ctx)

	// Post a "processing…" context block immediately so the user sees we're
	// working on it before the (slow) LLM call starts. The TS is reused later
	// to UpdateMessage the same row into the final preview.
	threadTS := ev.ThreadTimeStamp
	if threadTS == "" {
		threadTS = ev.TimeStamp
	}
	processingBlocks, processingFallback := buildProcessingContextBlocks()
	processingTS, postErr := uc.slackService.PostThreadMessage(ctx, ev.Channel, threadTS, processingBlocks, processingFallback)
	if postErr != nil {
		// Non-fatal: continue without the placeholder; the final preview will
		// still be posted at the end.
		errutil.Handle(ctx, goerr.Wrap(postErr, "failed to post processing context block",
			goerr.V("channel_id", ev.Channel),
			goerr.V("thread_ts", threadTS),
		), "could not show 'processing…' message to user")
	}

	candidates := uc.accessibleWorkspaces(ev.User)
	if len(candidates) == 0 {
		uc.removeProcessingMessage(ctx, ev.Channel, processingTS)
		return uc.notifyNoWorkspace(ctx, ev)
	}

	mentionTime := parseSlackTS(ev.TimeStamp)

	// Fetch channel descriptor (name, topic, purpose, privacy) so the
	// planner has channel-level context, not just the mention text.
	// Failure is non-fatal — fall back to nil and rely purely on the
	// surrounding messages.
	channelInfo, channelInfoErr := uc.slackService.GetChannelInfo(ctx, ev.Channel)
	if channelInfoErr != nil {
		errutil.Handle(ctx, goerr.Wrap(channelInfoErr, "fetch channel info for draft prompt",
			goerr.V("channel_id", ev.Channel),
		), "could not enrich draft prompt with channel info; continuing without it")
		channelInfo = nil
	}

	var (
		msgs []model.ProposalMessage
		err  error
	)
	if ev.ThreadTimeStamp != "" {
		msgs, err = uc.collector.CollectThread(ctx, ev.Channel, ev.ThreadTimeStamp)
	} else {
		msgs, err = uc.collector.CollectChannelRecent(ctx, ev.Channel, mentionTime)
	}
	if err != nil {
		return goerr.Wrap(err, "failed to collect messages",
			goerr.V("channel_id", ev.Channel),
			goerr.V("thread_ts", ev.ThreadTimeStamp),
		)
	}

	d := model.NewCaseProposal(time.Now().UTC(), ev.User)
	d.MentionText = ev.Text
	d.RawMessages = msgs
	d.Source = model.ProposalSource{
		ChannelID: ev.Channel,
		ThreadTS:  ev.ThreadTimeStamp,
		MentionTS: ev.TimeStamp,
	}
	// SelectedWorkspaceID is intentionally left empty: the planner picks the
	// workspace via its `list_workspaces` / `get_workspace` tools and surfaces
	// the choice through the materialise terminal action, where the host
	// handler stamps the workspace onto the draft.
	d.InferenceInProgress = true

	if err := uc.repo.CaseProposal().Save(ctx, d); err != nil {
		uc.removeProcessingMessage(ctx, ev.Channel, processingTS)
		return goerr.Wrap(err, "failed to save initial draft")
	}

	session, err := uc.loadOrCreateDraftSession(ctx, ev.Channel, threadTS, ev.User, d.ID)
	if err != nil {
		uc.removeProcessingMessage(ctx, ev.Channel, processingTS)
		return goerr.Wrap(err, "load or create draft session")
	}

	handler := newSlackDraftHandler(
		uc.repo, uc.registry, uc.slackService,
		ev.Channel, threadTS, ev.TimeStamp, ev.User,
		candidates, d.ID, processingTS, "",
	)

	userInput := buildProposalUserInput(d, ev.Text, channelInfo)

	result, runErr := uc.draftUC.RunTurn(ctx, proposal.TurnRequest{
		Session:          session,
		UserInput:        userInput,
		Trigger:          proposal.TriggerAppMention,
		TriggerTS:        ev.TimeStamp,
		ActorUserID:      ev.User,
		ExistingProposal: d,
		Handler:          handler,
	})
	if runErr != nil {
		uc.removeProcessingMessage(ctx, ev.Channel, processingTS)
		uc.notifyMaterializationFailed(ctx, ev, runErr)
		return goerr.Wrap(runErr, "draft turn failed")
	}
	switch result.Status {
	case proposal.StatusBusy, proposal.StatusIdempotent:
		// Handler.PostBusy already posted the busy notice (StatusBusy);
		// StatusIdempotent is silent. The processing placeholder may
		// still be showing — replace it with the "ended" footer.
		uc.removeProcessingMessage(ctx, ev.Channel, processingTS)
	case proposal.StatusFallback:
		// Planner exhausted budget / hit an internal error before reaching
		// a terminal action. Surface a system fallback message so the user
		// is not left waiting on the processing placeholder.
		uc.removeProcessingMessage(ctx, ev.Channel, processingTS)
		uc.notifyDraftFallback(ctx, ev.Channel, threadTS, result.FallbackReason)
	}

	logger.Info("case draft turn finished",
		"proposal_id", d.ID,
		"channel_id", ev.Channel,
		"user_id", ev.User,
		"status", int(result.Status),
		"ended_with", string(result.EndedWith),
	)
	return nil
}

// notifyDraftFallback posts a thread reply telling the user the planner
// ran out of budget or hit an internal error. Best-effort; secondary
// failures are funneled through errutil.Handle.
func (uc *MentionProposalUseCase) notifyDraftFallback(ctx context.Context, channelID, threadTS, reason string) {
	const text = ":warning: I couldn't reach a conclusion within the budget for this turn. Please mention me again with more context."
	if _, err := uc.slackService.PostThreadReply(ctx, channelID, threadTS, text); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "post draft fallback reply",
			goerr.V("channel_id", channelID),
			goerr.V("thread_ts", threadTS),
			goerr.V("fallback_reason", reason),
		), "could not surface draft fallback to user")
	}
}

// loadOrCreateDraftSession returns the Session for the given thread,
// stamping the draft-specific fields (CreatorUserID, ProposalID) when a fresh
// session is created. An existing session simply has its ProposalID updated
// when the caller has just freshly created a draft.
func (uc *MentionProposalUseCase) loadOrCreateDraftSession(ctx context.Context, channelID, threadTS, creatorUserID string, proposalID model.CaseProposalID) (*model.Session, error) {
	existing, err := uc.repo.Session().GetByThread(ctx, channelID, threadTS)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get session")
	}
	if existing != nil {
		existing.ProposalID = proposalID
		if creatorUserID != "" && existing.CreatorUserID == "" {
			existing.CreatorUserID = creatorUserID
		}
		return existing, nil
	}

	now := time.Now().UTC()
	return &model.Session{
		ID:            uuid.Must(uuid.NewV7()).String(),
		ChannelID:     channelID,
		ThreadTS:      threadTS,
		CreatorUserID: creatorUserID,
		ProposalID:    proposalID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

// buildProposalUserInput assembles the planner's first user message. The
// system prompt already advertises every registered workspace's identity,
// so this function only surfaces the mention text, the channel
// descriptor (so the planner can read the channel name / topic /
// purpose to anchor workspace and intent inference), and the surrounding
// conversation. Workspace selection is the planner's job — driven by
// the `list_workspaces` / `get_workspace` tools.
//
// channelInfo may be nil when the upstream `conversations.info` lookup
// failed; in that case the section is omitted but the rest of the
// prompt still renders.
func buildProposalUserInput(d *model.CaseProposal, mentionText string, channelInfo *slacksvc.ChannelInfo) string {
	var b strings.Builder
	b.WriteString("# User mention\n")
	b.WriteString(mentionText)
	b.WriteString("\n\n")

	if section := formatChannelContext(channelInfo); section != "" {
		b.WriteString(section)
	}

	if d != nil && len(d.RawMessages) > 0 {
		b.WriteString("# Surrounding conversation (chronological, oldest first)\n")
		for _, m := range d.RawMessages {
			fmt.Fprintf(&b, "- [%s] %s: %s\n", m.TS, m.UserID, strings.ReplaceAll(m.Text, "\n", " "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// formatChannelContext renders the "# Channel context" prompt section
// from a ChannelInfo. Returns the empty string when ci is nil so the
// caller can write the result unconditionally without a guard. The
// section is intentionally compact — channel name / topic / purpose
// carry the most signal for workspace inference; privacy / member
// count / archive flag set the audience tone.
func formatChannelContext(ci *slacksvc.ChannelInfo) string {
	if ci == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Channel context\n")
	if ci.Name != "" {
		fmt.Fprintf(&b, "- name: #%s\n", ci.Name)
	} else {
		fmt.Fprintf(&b, "- id: %s\n", ci.ID)
	}
	if topic := strings.TrimSpace(ci.Topic); topic != "" {
		fmt.Fprintf(&b, "- topic: %s\n", flattenLines(topic))
	}
	if purpose := strings.TrimSpace(ci.Purpose); purpose != "" {
		fmt.Fprintf(&b, "- description: %s\n", flattenLines(purpose))
	}
	privacy := "public"
	if ci.IsPrivate {
		privacy = "private"
	}
	fmt.Fprintf(&b, "- privacy: %s\n", privacy)
	if ci.IsShared {
		b.WriteString("- shared: yes (cross-workspace / connected)\n")
	}
	if ci.IsArchived {
		b.WriteString("- archived: yes\n")
	}
	if ci.NumMembers > 0 {
		fmt.Fprintf(&b, "- members: %d\n", ci.NumMembers)
	}
	if ci.Creator != "" {
		fmt.Fprintf(&b, "- creator: %s\n", ci.Creator)
	}
	if !ci.CreatedAt.IsZero() {
		fmt.Fprintf(&b, "- created_at: %s\n", ci.CreatedAt.Format(time.RFC3339))
	}
	b.WriteString("\n")
	return b.String()
}

// flattenLines collapses CR/LF/Tab into spaces so a multi-line topic /
// purpose stays on one prompt row.
func flattenLines(s string) string {
	r := strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ", "\t", " ")
	return strings.TrimSpace(r.Replace(s))
}

// HandleThreadReply runs when the dispatcher's F1-F8 filter chain has
// decided that a non-mention thread reply should resume the open-mode
// draft turn. The Session's LastAction is post_question — the planner
// will read the new user input from history and produce the next action.
func (uc *MentionProposalUseCase) HandleThreadReply(ctx context.Context, ev *slackevents.MessageEvent) error {
	if ev == nil {
		return goerr.New("MessageEvent is nil")
	}
	if uc.draftUC == nil {
		return goerr.New("draft usecase is not configured")
	}
	logger := logging.From(ctx)

	threadTS := ev.ThreadTimeStamp
	if threadTS == "" {
		return goerr.New("thread reply has no thread_ts; dispatcher should have filtered")
	}

	session, err := uc.repo.Session().GetByThread(ctx, ev.Channel, threadTS)
	if err != nil {
		return goerr.Wrap(err, "load session for thread reply")
	}
	if session == nil {
		// Defensive: dispatcher's F6 filter should have caught this.
		return goerr.New("session vanished between dispatch and HandleThreadReply",
			goerr.V("channel_id", ev.Channel),
			goerr.V("thread_ts", threadTS),
		)
	}

	// Locate draft for context. It may be missing if the user replied after
	// the draft was canceled / submitted; in that case we still try the turn
	// (planner can post a clarifying message) but with no draft state.
	var d *model.CaseProposal
	if session.ProposalID != "" {
		d, err = uc.repo.CaseProposal().Get(ctx, session.ProposalID)
		if err != nil {
			errutil.Handle(ctx, err, "thread-reply: failed to load draft; continuing without it")
		}
	}

	candidates := uc.accessibleWorkspaces(ev.User)

	// processingTS is empty — we don't post a placeholder for thread reply
	// resume; the planner trace block will appear when needed.
	var proposalID model.CaseProposalID
	if d != nil {
		proposalID = d.ID
	}
	handler := newSlackDraftHandler(
		uc.repo, uc.registry, uc.slackService,
		ev.Channel, threadTS, ev.TimeStamp, ev.User,
		candidates, proposalID, "", "",
	)

	result, runErr := uc.draftUC.RunTurn(ctx, proposal.TurnRequest{
		Session:          session,
		UserInput:        ev.Text,
		Trigger:          proposal.TriggerThreadReply,
		TriggerTS:        ev.TimeStamp,
		ActorUserID:      ev.User,
		ExistingProposal: d,
		Handler:          handler,
	})
	if runErr != nil {
		return goerr.Wrap(runErr, "thread reply turn failed")
	}
	if result.Status == proposal.StatusFallback {
		uc.notifyDraftFallback(ctx, ev.Channel, threadTS, result.FallbackReason)
	}

	logger.Info("thread reply turn finished",
		"channel_id", ev.Channel,
		"thread_ts", threadTS,
		"user_id", ev.User,
		"status", int(result.Status),
		"ended_with", string(result.EndedWith),
	)
	return nil
}

func (uc *MentionProposalUseCase) accessibleWorkspaces(_ string) []*model.WorkspaceEntry {
	// Currently every registered workspace is treated as accessible. Per-user
	// workspace authorization is out of scope for this feature; refining it
	// would feed in here without changing the rest of the flow.
	if uc.registry == nil {
		return nil
	}
	return uc.registry.List()
}

func (uc *MentionProposalUseCase) notifyNoWorkspace(ctx context.Context, ev *slackevents.AppMentionEvent) error {
	threadTS := ev.ThreadTimeStamp
	if threadTS == "" {
		threadTS = ev.TimeStamp
	}
	text := "No workspace is available for creating a Case from this channel."
	if _, err := uc.slackService.PostThreadMessage(ctx, ev.Channel, threadTS, nil, text); err != nil {
		errutil.Handle(ctx, err, "failed to post no-workspace thread message")
	}
	return nil
}

// notifyMaterializationFailed posts a thread reply telling the user that AI
// generation failed after all retries. Best-effort: secondary failures are
// logged via errutil.Handle but not propagated.
func (uc *MentionProposalUseCase) notifyMaterializationFailed(ctx context.Context, ev *slackevents.AppMentionEvent, cause error) {
	threadTS := ev.ThreadTimeStamp
	if threadTS == "" {
		threadTS = ev.TimeStamp
	}
	text := ":warning: Sorry — AI failed to generate a Case draft after several attempts. Please mention me again, or try in a different channel."
	if _, err := uc.slackService.PostThreadMessage(ctx, ev.Channel, threadTS, nil, text); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to post materialization-failure message",
			goerr.V("channel_id", ev.Channel),
			goerr.V("thread_ts", threadTS),
			goerr.V("cause", cause.Error()),
		), "could not surface materialization failure to user")
	}
}

// parseSlackTS converts a Slack timestamp (e.g., "1700000000.000200") to a
// time.Time. Slack timestamps are seconds.microseconds since unix epoch.
func parseSlackTS(ts string) time.Time {
	if ts == "" {
		return time.Now().UTC()
	}
	parts := strings.SplitN(ts, ".", 2)
	sec, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Now().UTC()
	}
	var usec int64
	if len(parts) == 2 {
		// Pad/truncate to microseconds (6 digits).
		frac := parts[1]
		switch {
		case len(frac) > 6:
			frac = frac[:6]
		case len(frac) < 6:
			frac = frac + strings.Repeat("0", 6-len(frac))
		}
		usec, _ = strconv.ParseInt(frac, 10, 64)
	}
	return time.Unix(sec, usec*1000).UTC()
}

// --- Block Kit construction (preview ephemeral) ---

// PreviewActionIDs identify the buttons inside the preview ephemeral.
const (
	ActionIDDraftSelectWS = "mention_draft_select_ws"
	ActionIDDraftSubmit   = "mention_draft_submit"
	ActionIDDraftEdit     = "mention_draft_edit"
	ActionIDDraftCancel   = "mention_draft_cancel"

	BlockIDDraftWSSelect = "mention_draft_ws_block"
	BlockIDDraftActions  = "mention_draft_actions"
)

func buildPreviewBlocks(
	draft *model.CaseProposal,
	selected *model.WorkspaceEntry,
	candidates []*model.WorkspaceEntry,
) ([]goslack.Block, string) {
	if draft == nil || selected == nil {
		return nil, "Case draft preview"
	}
	mat := draft.Materialization
	if mat == nil {
		return nil, "Case draft preview"
	}

	// 1. Title + description rendered as a single Slack `markdown` block.
	//    `# heading` gives the title heading-level styling, and the
	//    description follows underneath as italicised body text. Using one
	//    markdown block also dodges the per-section "Show more" cutoff.
	blocks := []goslack.Block{
		buildTitleAndDescriptionMarkdown("mention_draft_body", mat.Title, mat.Description),
		goslack.NewDividerBlock(),
	}

	// 3. Custom fields (definition list, required-missing flagged).
	if selected.FieldSchema != nil {
		blocks = append(blocks, buildFieldPairSections(selected.FieldSchema.Fields, mat.CustomFieldValues)...)
	}

	// 4. Workspace selector + action buttons co-located at the bottom.
	//    The selector lives in the same ActionBlock as the buttons so it
	//    sits next to them. When required fields are missing, Submit is
	//    hidden and Edit is promoted to primary so the UI matches the
	//    only viable next action.
	wsOptions := make([]*goslack.OptionBlockObject, 0, len(candidates))
	var initial *goslack.OptionBlockObject
	for _, c := range candidates {
		opt := goslack.NewOptionBlockObject(
			c.Workspace.ID,
			goslack.NewTextBlockObject(goslack.PlainTextType, fallbackText(c.Workspace.Name, c.Workspace.ID), false, false),
			nil,
		)
		wsOptions = append(wsOptions, opt)
		if c.Workspace.ID == selected.Workspace.ID {
			initial = opt
		}
	}
	wsSelect := goslack.NewOptionsSelectBlockElement(
		goslack.OptTypeStatic,
		goslack.NewTextBlockObject(goslack.PlainTextType, "Workspace", false, false),
		ActionIDDraftSelectWS,
		wsOptions...,
	)
	if initial != nil {
		wsSelect.InitialOption = initial
	}

	hasMissingRequired := schemaHasMissingRequired(selected.FieldSchema, mat)
	editBtn := goslack.NewButtonBlockElement(
		ActionIDDraftEdit,
		string(draft.ID),
		goslack.NewTextBlockObject(goslack.PlainTextType, "Edit", false, false),
	)
	cancelBtn := goslack.NewButtonBlockElement(
		ActionIDDraftCancel,
		string(draft.ID),
		goslack.NewTextBlockObject(goslack.PlainTextType, "Cancel", false, false),
	)
	cancelBtn.Style = goslack.StyleDanger

	elements := []goslack.BlockElement{wsSelect}
	if hasMissingRequired {
		editBtn.Style = goslack.StylePrimary
		elements = append(elements, editBtn, cancelBtn)
	} else {
		submitBtn := goslack.NewButtonBlockElement(
			ActionIDDraftSubmit,
			string(draft.ID),
			goslack.NewTextBlockObject(goslack.PlainTextType, "Submit", false, false),
		)
		submitBtn.Style = goslack.StylePrimary
		elements = append(elements, submitBtn, editBtn, cancelBtn)
	}
	actions := goslack.NewActionBlock(BlockIDDraftActions, elements...)
	// Encode the draft ID into the action block's BlockID so the WS
	// static_select handler can recover the draft from action.BlockID
	// (the static_select carries only the workspace ID in its value).
	actions.BlockID = BlockIDDraftWSSelect + ":" + string(draft.ID)
	blocks = append(blocks, actions)

	fallback := fmt.Sprintf("Case draft: %s", mat.Title)
	return blocks, fallback
}

// buildCaseCreatedTailBlocks renders the post-create state as a single
// context block carrying the case number, title, and a clickable mention
// of the case's Slack channel. This replaces the prior full preview-style
// re-render: the case body is already viewable via the linked channel, so
// the thread message just needs to point users at the new home.
func buildCaseCreatedTailBlocks(ctx context.Context, created *model.Case) ([]goslack.Block, string) {
	if created == nil {
		return nil, "Case created"
	}
	// Title is interpolated into a markdown-bold (`*%s*`) slot in the i18n
	// strings, so any literal `*` / `_` / `~` / `\`` would corrupt Slack
	// formatting. Escape inline like buildTitleAndDescriptionMarkdown does.
	// Also collapse whitespace and supply a placeholder when the title is
	// blank so the rendered line never reads "Case #N ** has been created.".
	title := strings.TrimSpace(created.Title)
	if title == "" {
		title = "(untitled)"
	}
	escapedTitle := escapeMarkdownInline(title)
	var line string
	if created.SlackChannelID != "" {
		line = "✅ " + i18n.T(ctx, i18n.MsgCaseCreatedWithChannel, created.ID, escapedTitle, created.SlackChannelID)
	} else {
		line = "✅ " + i18n.T(ctx, i18n.MsgCaseCreated, created.ID, escapedTitle)
	}
	blocks := []goslack.Block{
		goslack.NewContextBlock(
			"mention_draft_created_tail",
			goslack.NewTextBlockObject(goslack.MarkdownType, line, false, false),
		),
	}
	fallback := fmt.Sprintf("Created case #%d: %s", created.ID, title)
	return blocks, fallback
}

// buildProcessingContextBlocks renders an immediate "processing…" placeholder
// posted right after a mention is received and before the (slow) LLM call.
// On the happy path the placeholder is later replaced with the "drafted —
// see preview below" breadcrumb (see buildProcessingCompletedBlocks); on
// early-failure paths it is replaced with the "flow ended" breadcrumb
// (see removeProcessingMessage). The preview itself is always posted as
// a fresh thread reply at the bottom so it sits chronologically after
// the planner trace messages.
func buildProcessingContextBlocks() ([]goslack.Block, string) {
	ctxBlock := goslack.NewContextBlock(
		"mention_draft_processing_ctx",
		goslack.NewTextBlockObject(goslack.MarkdownType,
			"⏳ Drafting a Case from this mention…",
			false, false,
		),
	)
	return []goslack.Block{ctxBlock}, "Drafting a Case…"
}

// buildProcessingCompletedBlocks renders the placeholder state shown after
// Materialize has posted the preview as a fresh message at the thread end.
// Updating (rather than deleting) the placeholder leaves a visible
// breadcrumb at the position the user first looked at, pointing them to
// the new preview further down — the bot does not always have the scopes
// required by chat.delete, so an update is the most reliable cleanup.
func buildProcessingCompletedBlocks(ctx context.Context) ([]goslack.Block, string) {
	text := i18n.T(ctx, i18n.MsgProposalProcessingCompleted)
	ctxBlock := goslack.NewContextBlock(
		"mention_draft_processing_completed",
		goslack.NewTextBlockObject(goslack.MarkdownType, text, false, false),
	)
	return []goslack.Block{ctxBlock}, text
}

// removeProcessingMessage replaces the placeholder with a single context line
// indicating the flow ended without producing a preview. We use UpdateMessage
// (not DeleteMessage) because the Slack API delete scopes are stricter and
// not always granted to bots.
func (uc *MentionProposalUseCase) removeProcessingMessage(ctx context.Context, channelID, ts string) {
	if ts == "" {
		return
	}
	endBlock := goslack.NewContextBlock(
		"mention_draft_processing_ended",
		goslack.NewTextBlockObject(goslack.MarkdownType, "_(Case draft flow ended.)_", false, false),
	)
	if err := uc.slackService.UpdateMessage(ctx, channelID, ts, []goslack.Block{endBlock}, "Case draft flow ended."); err != nil {
		errutil.Handle(ctx, err, "failed to clear processing placeholder")
	}
}

// buildLockBlocks renders the "inference in progress" state. Title/Description
// and custom fields are stripped, the workspace selector is rendered as a
// plain context line (disabled), and action buttons are removed.
func buildLockBlocks(targetWorkspaceName string) ([]goslack.Block, string) {
	ctxText := fmt.Sprintf("Materializing case for *%s*…", escapeForSection(targetWorkspaceName))
	ctxBlock := goslack.NewContextBlock(
		"mention_draft_lock_ctx",
		goslack.NewTextBlockObject(goslack.MarkdownType, ctxText, false, false),
	)
	return []goslack.Block{ctxBlock}, "Materializing case…"
}

// buildMaterializationErrorBlocks renders an error state when the LLM
// inference exhausted all retries during a workspace switch.
func buildMaterializationErrorBlocks(targetWorkspaceName string) ([]goslack.Block, string) {
	text := fmt.Sprintf(":warning: AI failed to generate a Case draft for *%s* after several attempts. Please try a different workspace, or mention me again.", escapeForSection(targetWorkspaceName))
	section := goslack.NewSectionBlock(
		goslack.NewTextBlockObject(goslack.MarkdownType, text, false, false),
		nil, nil,
	)
	return []goslack.Block{section}, "AI failed to generate a Case draft"
}

// formatFieldValueForDisplay returns a Markdown-friendly rendering of a
// FieldValue for inclusion in a preview section block. All values are wrapped
// in `code spans` for visual contrast against labels.
func formatFieldValueForDisplay(fd config.FieldDefinition, fv model.FieldValue) string {
	switch fd.Type {
	case types.FieldTypeSelect:
		if s, ok := fv.Value.(string); ok && s != "" {
			return "`" + lookupOptionName(fd, s) + "`"
		}
	case types.FieldTypeMultiSelect:
		switch arr := fv.Value.(type) {
		case []string:
			if len(arr) == 0 {
				return "`(empty)`"
			}
			out := make([]string, len(arr))
			for i, s := range arr {
				out[i] = "`" + lookupOptionName(fd, s) + "`"
			}
			return strings.Join(out, ", ")
		}
		return fmt.Sprintf("`%v`", fv.Value)
	case types.FieldTypeMultiUser:
		switch arr := fv.Value.(type) {
		case []string:
			if len(arr) == 0 {
				return "`(empty)`"
			}
			out := make([]string, len(arr))
			for i, s := range arr {
				if s != "" {
					out[i] = fmt.Sprintf("<@%s>", s)
				} else {
					out[i] = "`" + s + "`"
				}
			}
			return strings.Join(out, ", ")
		}
		return fmt.Sprintf("`%v`", fv.Value)
	case types.FieldTypeUser:
		if s, ok := fv.Value.(string); ok && s != "" {
			return fmt.Sprintf("<@%s>", s)
		}
	case types.FieldTypeURL:
		if s, ok := fv.Value.(string); ok && s != "" {
			return fmt.Sprintf("<%s>", s)
		}
	case types.FieldTypeNumber:
		switch n := fv.Value.(type) {
		case float64:
			return fmt.Sprintf("`%v`", n)
		case int, int64:
			return fmt.Sprintf("`%v`", n)
		}
	}
	if s, ok := fv.Value.(string); ok {
		if s == "" {
			return "`(empty)`"
		}
		return "`" + s + "`"
	}
	return fmt.Sprintf("`%v`", fv.Value)
}

// buildFieldPairSections renders schema fields as a single mrkdwn section,
// one line per field formatted `*Label:* value`. We tried Slack's
// section.fields[] (both mrkdwn and plain_text) for a 2-column grid, but
// the in-thread width consistently collapses it to a single column —
// the flat definition-list reads more cleanly there than half-broken
// columns.
func buildFieldPairSections(fields []config.FieldDefinition, values map[string]model.FieldValue) []goslack.Block {
	if len(fields) == 0 {
		return nil
	}
	lines := make([]string, 0, len(fields))
	for _, fd := range fields {
		label := fallbackText(fd.Name, fd.ID)
		fv, present := values[fd.ID]
		hasValue := present && !isFieldValueEmpty(fv)
		var valueText string
		switch {
		case hasValue:
			valueText = formatFieldValueForDisplay(fd, fv)
		case fd.Required:
			valueText = "⚠️ _required — not set_"
		default:
			valueText = "_not set_"
		}
		lines = append(lines, fmt.Sprintf("*%s:* %s", label, valueText))
	}
	return []goslack.Block{
		goslack.NewSectionBlock(
			goslack.NewTextBlockObject(goslack.MarkdownType, strings.Join(lines, "\n"), false, false),
			nil, nil,
		),
	}
}

// buildTitleAndDescriptionMarkdown renders the title as a level-1 markdown
// heading followed by the description as italic body text inside a single
// Slack `markdown` block. The markdown block reliably renders headings and
// is not subject to the per-section "Show more" collapse.
func buildTitleAndDescriptionMarkdown(blockID, title, description string) goslack.Block {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "(untitled)"
	}
	desc := strings.TrimSpace(description)

	var sb strings.Builder
	sb.WriteString("# 🎫 ")
	sb.WriteString(escapeMarkdownInline(title))
	sb.WriteString("\n")
	if desc != "" {
		sb.WriteString("\n")
		// One italic span per line so blank lines remain blank.
		for line := range strings.SplitSeq(desc, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				sb.WriteString("\n")
				continue
			}
			sb.WriteString("_")
			sb.WriteString(trimmed)
			sb.WriteString("_\n")
		}
	}
	return goslack.NewMarkdownBlock(blockID, sb.String())
}

// escapeMarkdownInline neutralises markdown characters that would otherwise
// transform a heading line into something else (e.g. a stray leading `#`
// turning into a deeper heading, or `_` toggling italics mid-title).
func escapeMarkdownInline(s string) string {
	r := strings.NewReplacer(
		"\n", " ",
		"\\", `\\`,
		"_", `\_`,
		"*", `\*`,
		"`", "\\`",
	)
	return r.Replace(s)
}

// schemaHasMissingRequired reports whether any required field defined in the
// schema lacks a non-empty value in the materialization. Used to drop the
// Submit button (which would fail validation server-side) and promote Edit
// to primary when something still needs to be filled in.
func schemaHasMissingRequired(schema *config.FieldSchema, mat *model.WorkspaceMaterialization) bool {
	if schema == nil || mat == nil {
		return false
	}
	for _, fd := range schema.Fields {
		if !fd.Required {
			continue
		}
		fv, ok := mat.CustomFieldValues[fd.ID]
		if !ok || isFieldValueEmpty(fv) {
			return true
		}
	}
	return false
}

// isFieldValueEmpty treats nil values, empty strings, and empty slices as
// "not present" for the purposes of required-field gating and preview
// rendering.
func isFieldValueEmpty(fv model.FieldValue) bool {
	switch v := fv.Value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case []string:
		return len(v) == 0
	}
	return false
}

// lookupOptionName returns the display Name for a select/multi-select option
// ID. Falls back to the ID itself (with an "(unknown option)" hint) when the
// option is not found in the field definition.
func lookupOptionName(fd config.FieldDefinition, id string) string {
	for _, opt := range fd.Options {
		if opt.ID == id {
			if opt.Name != "" {
				return opt.Name
			}
			return id
		}
	}
	return id + " (unknown option)"
}

func fallbackText(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

// escapeForSection performs minimal escaping to keep a free-form string from
// breaking section markdown formatting. Slack's mrkdwn escape rules are
// limited; preserving the value as-is is acceptable for preview purposes.
func escapeForSection(s string) string {
	if s == "" {
		return "_(empty)_"
	}
	return s
}
