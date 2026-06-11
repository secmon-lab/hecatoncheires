package usecase

import (
	"context"
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

// HandleThreadCaseCreation processes a channel-root human post in a thread-mode
// monitored channel — the ONLY trigger that initiates case creation. It does
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

	return uc.runThreadCaseCreation(ctx, entry, channelID, threadTS, reporter,
		[]threadcase.ConversationMessage{{Timestamp: threadTS, UserID: reporter, Text: text}},
		"", "", threadTS)
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

	return uc.runThreadCaseCreation(ctx, entry, channelID, threadTS, reporter,
		nil, msg.Text(), msg.ID(), msg.ID())
}

// runThreadCaseCreation is the shared body for the initial post and the resume
// paths. systemMessages seeds the first turn; mentionText / mentionTS feed a
// resume. triggerTS is the turn-lock dedup key.
func (uc *AgentUseCase) runThreadCaseCreation(
	ctx context.Context,
	entry *model.WorkspaceEntry,
	channelID, threadTS, reporter string,
	systemMessages []threadcase.ConversationMessage,
	mentionText, mentionTS, triggerTS string,
) error {
	wsID := entry.Workspace.ID

	session, err := uc.loadOrCreateSession(ctx, wsID, 0, channelID, threadTS)
	if err != nil {
		errutil.Handle(ctx, err, "thread case: load session for create")
		return nil
	}
	// The session predates the case; record the reporter so the create handler
	// can attribute the case even on a resume turn.
	if session.CreatorUserID == "" {
		session.CreatorUserID = reporter
	}

	// Supersede: when a reply / mention resumes the flow while a question form
	// is still pending, mark that form stale (removing its Submit button) so it
	// can no longer be answered. The new message is the latest intent. The form
	// text stays visible in the thread for later reference; the snapshot is
	// overwritten when the resumed turn asks again or cleared when it creates.
	if mentionText != "" && session.PendingQuestion != nil && session.PendingQuestion.PostedMessageTS != "" {
		uc.markThreadQuestionStale(ctx, channelID, session.PendingQuestion.PostedMessageTS)
	}

	traceMsg := uc.newTraceMessage(channelID, threadTS)
	// Immediate progress so the user is not left staring at silence while the
	// agent investigates.
	traceMsg.update(ctx, i18n.T(ctx, i18n.MsgThreadCaseCreating))

	res, runErr := uc.threadcase.RunTurn(ctx, threadcase.TurnRequest{
		Session:        session,
		Workspace:      entry,
		Case:           nil,
		ChannelID:      channelID,
		ThreadTS:       threadTS,
		MentionText:    mentionText,
		MentionTS:      mentionTS,
		TriggerTS:      triggerTS,
		Mode:           threadcase.ModeCreate,
		SystemMessages: systemMessages,
		Handler:        uc.newThreadcaseCreateHandler(channelID, threadTS, reporter, entry, traceMsg),
	})
	if runErr != nil {
		errutil.Handle(ctx, runErr, "thread case create turn")
		uc.postThreadReply(ctx, channelID, threadTS, "⚠️ "+i18n.T(ctx, i18n.MsgAgentError))
		return nil
	}

	switch res.Status {
	case threadcase.StatusCompleted:
		if res.Case == nil {
			return nil
		}
		uc.bindSessionToCase(ctx, channelID, threadTS, res.Case.ID)
		uc.postThreadCaseSummary(ctx, wsID, entry, res.Case, traceMsg, channelID, threadTS)
	case threadcase.StatusQuestion:
		// The question form was posted by the handler; wait for the user to
		// answer it via the form's Submit interaction (HandleThreadCaseQuestionSubmit).
	case threadcase.StatusFallback:
		uc.finalizeTrace(ctx, traceMsg, channelID, threadTS, i18n.T(ctx, i18n.MsgThreadCaseCreateFallback))
	case threadcase.StatusBusy, threadcase.StatusIdempotent:
		// Another turn owns this thread, or a duplicate trigger — drop.
	}
	return nil
}

// bindSessionToCase stamps the freshly created case id onto the thread's
// session (Session.ID stays stable so the gollem history stays continuous).
// Best-effort: a failure here only means later mentions re-resolve the case by
// thread lookup.
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

// postThreadCaseSummary posts the Block Kit summary of the just-created case,
// finalizing the progress message into it when possible.
func (uc *AgentUseCase) postThreadCaseSummary(ctx context.Context, wsID string, entry *model.WorkspaceEntry, c *model.Case, traceMsg *traceMessage, channelID, threadTS string) {
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
		uc.postThreadReply(ctx, channelID, threadTS, "⚠️ "+i18n.T(ctx, i18n.MsgAgentError))
		return goerr.Wrap(runErr, "thread case mention turn")
	}

	switch res.Status {
	case threadcase.StatusBusy:
		uc.postThreadReply(ctx, channelID, threadTS, i18n.T(ctx, i18n.MsgKeyAgentBusy))
		return nil
	case threadcase.StatusIdempotent, threadcase.StatusQuestion:
		// Question already posted by the handler; idempotent drops silently.
		return nil
	case threadcase.StatusFallback:
		uc.postThreadReply(ctx, channelID, threadTS, "⚠️ "+i18n.T(ctx, i18n.MsgAgentError))
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
// appropriate reply to the thread.
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
				errutil.Handle(ctx, err, "thread case: materialize on mention")
			}
		}
		text := d.Message
		if text == "" {
			text = i18n.T(ctx, i18n.MsgThreadCaseUpdated)
		}
		uc.finalizeTrace(ctx, traceMsg, channelID, threadTS, text)
	case threadcase.DecisionClose:
		statusName := d.CloseStatus
		if uc.deps.CaseUC != nil {
			if _, err := uc.deps.CaseUC.UpdateCaseStatus(ctx, wsID, caseID, d.CloseStatus); err != nil {
				errutil.Handle(ctx, err, "thread case: close")
			}
		}
		if entry.CaseStatusSet != nil {
			if def, ok := entry.CaseStatusSet.Get(d.CloseStatus); ok {
				statusName = def.Name
			}
		}
		text := d.Message
		if text == "" {
			text = i18n.T(ctx, i18n.MsgThreadCaseClosed, statusName)
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
func (uc *AgentUseCase) newThreadcaseCreateHandler(channelID, threadTS, reporter string, entry *model.WorkspaceEntry, traceMsg *traceMessage) threadcase.Handler {
	wsID := entry.Workspace.ID
	return threadcase.HandlerFuncs{
		TraceFn: func(ctx context.Context, line string) {
			if traceMsg != nil {
				traceMsg.update(ctx, line)
			}
		},
		QuestionFn: func(ctx context.Context, ssn *model.Session, q threadcase.QuestionPayload) error {
			// Post the interactive selection form and record the snapshot on
			// the session (PendingQuestion); the threadcase runtime persists
			// the session when the turn ends on this question.
			return uc.postThreadCreateQuestionForm(ctx, ssn, channelID, threadTS, reporter, q)
		},
		CreateFn: func(ctx context.Context, _ *model.Session, p threadcase.CreatePayload) (*model.Case, error) {
			return uc.deps.CaseUC.CreateThreadCaseWithFields(ctx, wsID, channelID, threadTS, reporter, p.Title, p.Description, p.Fields)
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
		TraceFn: func(ctx context.Context, line string) {
			if traceMsg != nil {
				traceMsg.update(ctx, line)
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
