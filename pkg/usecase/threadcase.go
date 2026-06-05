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

// threadCaseTitleMaxRunes bounds the placeholder title derived from the first
// message before the materialize agent fills a better one.
const threadCaseTitleMaxRunes = 80

// HandleThreadCaseCreation processes a top-level human post in a thread-mode
// monitored channel: it idempotently creates a Case bound to the thread, acks
// with a web-UI link, and runs the materialize agent to fill the case fields.
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
	// create a second case or re-run materialize.
	existing, err := uc.deps.Repo.Case().GetBySlackThread(ctx, wsID, channelID, threadTS)
	if err != nil {
		return goerr.Wrap(err, "look up existing thread case",
			goerr.V("channel_id", channelID), goerr.V("thread_ts", threadTS))
	}
	if existing != nil {
		return nil
	}

	created, err := uc.deps.CaseUC.CreateThreadCase(ctx, wsID, channelID, threadTS, reporter, truncateTitle(text), text)
	if err != nil {
		return goerr.Wrap(err, "create thread case",
			goerr.V("channel_id", channelID), goerr.V("thread_ts", threadTS))
	}

	// Ack with the web-UI link immediately.
	if uc.deps.SlackService != nil {
		ack := i18n.T(ctx, i18n.MsgThreadCaseCreated, uc.deps.CaseUC.CaseURL(wsID, created.ID))
		if _, perr := uc.deps.SlackService.PostThreadReply(ctx, channelID, threadTS, ack); perr != nil {
			errutil.Handle(ctx, perr, "post thread case created reply")
		}
	}

	// Run the materialize agent. Failure here is non-fatal — the case exists.
	session, err := uc.loadOrCreateSession(ctx, wsID, created.ID, channelID, threadTS)
	if err != nil {
		errutil.Handle(ctx, err, "thread case: load session for materialize")
		return nil
	}
	traceMsg := uc.newTraceMessage(channelID, threadTS)
	res, runErr := uc.threadcase.RunTurn(ctx, threadcase.TurnRequest{
		Session:        session,
		Workspace:      entry,
		Case:           created,
		ChannelID:      channelID,
		ThreadTS:       threadTS,
		TriggerTS:      threadTS,
		Mode:           threadcase.ModeMaterialize,
		SystemMessages: []threadcase.ConversationMessage{{Timestamp: threadTS, UserID: reporter, Text: text}},
		Handler:        uc.newThreadcaseHandler(channelID, threadTS, traceMsg),
	})
	if runErr != nil {
		errutil.Handle(ctx, runErr, "thread case materialize turn")
		return nil
	}
	if res.Status == threadcase.StatusCompleted && res.Decision != nil && res.Decision.Kind == threadcase.DecisionMaterialize {
		uc.applyMaterialize(ctx, wsID, entry, created.ID, res.Decision)
	}
	return nil
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

// applyMaterialize writes the materialize decision onto the case (silent — the
// creation ack was already posted).
func (uc *AgentUseCase) applyMaterialize(ctx context.Context, wsID string, entry *model.WorkspaceEntry, caseID int64, d *threadcase.Decision) {
	if uc.deps.CaseUC == nil {
		return
	}
	fv := buildThreadFieldValues(entry, d.Fields)
	if _, err := uc.deps.CaseUC.MaterializeThreadCase(ctx, wsID, caseID, d.Title, d.Description, fv); err != nil {
		errutil.Handle(ctx, err, "thread case: materialize")
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

// newThreadcaseHandler builds the host-side Handler for one thread-mode turn.
func (uc *AgentUseCase) newThreadcaseHandler(channelID, threadTS string, traceMsg *traceMessage) threadcase.Handler {
	return threadcase.HandlerFuncs{
		TraceFn: func(ctx context.Context, line string) {
			if traceMsg != nil {
				traceMsg.update(ctx, line)
			}
		},
		QuestionFn: func(ctx context.Context, _ *model.Session, q threadcase.QuestionPayload) error {
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
			if uc.deps.SlackService == nil {
				return nil
			}
			_, err := uc.deps.SlackService.PostThreadReply(ctx, channelID, threadTS, b.String())
			return err
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

// truncateTitle derives a placeholder case title from the first line of the
// triggering message, bounded to threadCaseTitleMaxRunes.
func truncateTitle(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	line = strings.TrimSpace(line)
	r := []rune(line)
	if len(r) > threadCaseTitleMaxRunes {
		return string(r[:threadCaseTitleMaxRunes]) + "…"
	}
	if line == "" {
		return "Untitled case"
	}
	return line
}
