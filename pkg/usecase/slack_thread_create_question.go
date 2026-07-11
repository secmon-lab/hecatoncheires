package usecase

import (
	"context"
	"strings"

	"github.com/m-mizutani/goerr/v2"
	goslack "github.com/slack-go/slack" //nolint:depguard

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/proposal"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/threadcase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// ActionIDThreadCreateQuestionSubmit is the action_id of the Submit button on
// the thread-mode case-initialization question form. It is distinct from the
// open-mode (proposal) submit action so the interactions controller routes it
// to HandleThreadCaseQuestionSubmit. The button value carries the thread TS so
// the handler can re-resolve the thread without parsing block_ids. The form's
// per-item input elements reuse the shared open-mode action_ids
// (ActionIDDraftQuestion*) so the answer parser (parseDraftQuestionAnswers) is
// shared.
const ActionIDThreadCreateQuestionSubmit = "thread_create_question_submit"

// encodeCaseThreadValue / parseCaseThreadValue carry the case thread (channel +
// thread_ts) in the Submit button value. The form may be displayed in a
// different thread than the one the Case is bound to (a cross-channel reaction
// posts the form in the reactor's source thread), so the resume cannot rely on
// the form's own location to find the case thread. Slack channel IDs contain no
// ':' and a message ts uses '.', so the first ':' is an unambiguous separator.
func encodeCaseThreadValue(channelID, threadTS string) string {
	return channelID + ":" + threadTS
}

func parseCaseThreadValue(v string) (channelID, threadTS string, ok bool) {
	i := strings.IndexByte(v, ':')
	if i <= 0 || i >= len(v)-1 {
		return "", "", false
	}
	return v[:i], v[i+1:], true
}

// buildThreadCreateQuestionBlocks renders a planner question for the thread-mode
// create flow as an interactive Block Kit form: a header (@-mentioning the
// reporter), one input per item (radio / checkboxes / free-text plus an "Other"
// fallback for closed lists), and a thread-create Submit button carrying the
// thread TS. It mirrors the open-mode form but routes submission to the
// thread-create handler. items are the persisted snapshot shape so the same
// builder serves both the first post and the re-prompt-on-error path.
func buildThreadCreateQuestionBlocks(ctx context.Context, reason string, items []model.PendingQuestionItem, caseThreadValue, requesterUserID string) ([]goslack.Block, string) {
	header := goslack.NewSectionBlock(
		goslack.NewTextBlockObject(goslack.MarkdownType, questionHeaderText(reason, requesterUserID), false, false),
		nil, nil,
	)
	blocks := []goslack.Block{header, goslack.NewDividerBlock()}

	for _, item := range items {
		if item.Type == string(proposal.QuestionItemFreeText) {
			elem := goslack.NewPlainTextInputBlockElement(
				goslack.NewTextBlockObject(goslack.PlainTextType, "Type your answer here…", false, false),
				ActionIDDraftQuestionFreeText,
			)
			elem.Multiline = true
			input := goslack.NewInputBlock(
				BlockIDDraftQuestionItemPrefix+item.ID,
				goslack.NewTextBlockObject(goslack.PlainTextType, item.Text, false, false),
				nil, elem,
			)
			input.Optional = true
			blocks = append(blocks, input)
			continue
		}

		opts := make([]*goslack.OptionBlockObject, 0, len(item.Options))
		for _, optID := range item.Options {
			opts = append(opts, goslack.NewOptionBlockObject(
				optID, goslack.NewTextBlockObject(goslack.PlainTextType, optID, false, false), nil))
		}
		var element goslack.BlockElement
		if item.Type == string(proposal.QuestionItemMultiSelect) {
			element = goslack.NewCheckboxGroupsBlockElement(ActionIDDraftQuestionChoice, opts...)
		} else {
			element = goslack.NewRadioButtonsBlockElement(ActionIDDraftQuestionChoice, opts...)
		}
		input := goslack.NewInputBlock(
			BlockIDDraftQuestionItemPrefix+item.ID,
			goslack.NewTextBlockObject(goslack.PlainTextType, item.Text, false, false),
			nil, element,
		)
		input.Optional = true
		blocks = append(blocks, input)

		other := goslack.NewPlainTextInputBlockElement(nil, ActionIDDraftQuestionOther)
		otherInput := goslack.NewInputBlock(
			BlockIDDraftQuestionItemPrefix+item.ID+BlockIDDraftQuestionOtherSuffix,
			goslack.NewTextBlockObject(goslack.PlainTextType, "Other (free text)", false, false),
			nil, other,
		)
		otherInput.Optional = true
		blocks = append(blocks, otherInput)
	}

	submit := goslack.NewButtonBlockElement(
		ActionIDThreadCreateQuestionSubmit,
		caseThreadValue,
		goslack.NewTextBlockObject(goslack.PlainTextType, "Submit", false, false),
	)
	submit.Style = goslack.StylePrimary
	blocks = append(blocks, goslack.NewActionBlock(BlockIDDraftQuestionActions, submit))

	return blocks, i18n.T(ctx, i18n.MsgThreadCaseQuestionFallback)
}

// threadQuestionToPending converts the runtime question payload into the
// persisted snapshot stored on Session.PendingQuestion.
func threadQuestionToPending(q threadcase.QuestionPayload, channelID, messageTS string) *model.PendingQuestion {
	items := make([]model.PendingQuestionItem, len(q.Items))
	for i, it := range q.Items {
		items[i] = model.PendingQuestionItem{
			ID:      it.ID,
			Text:    it.Text,
			Type:    string(it.Type),
			Options: it.Options,
		}
	}
	return &model.PendingQuestion{
		PostedChannelID: channelID,
		PostedMessageTS: messageTS,
		Reason:          q.Reason,
		Items:           items,
	}
}

// postThreadCreateQuestionForm posts the interactive question form to the
// thread and records the snapshot on the session so the submit handler can
// parse and validate the answer. It returns the form's message TS. The session
// is mutated (PendingQuestion set) but NOT persisted here — the threadcase
// runtime persists the session when the turn ends on a question.
// postThreadCreateQuestionForm posts the interactive question form to the UI
// thread (uiChannel/uiTS) and records the snapshot on the session so the submit
// handler can parse and validate the answer. The Submit button carries the case
// thread (caseChannel/caseTS), which may differ from the UI thread on a
// cross-channel reaction, so the resume can locate the session by the case
// thread rather than the form's own location.
func (uc *AgentUseCase) postThreadCreateQuestionForm(ctx context.Context, ssn *model.Session, uiChannel, uiTS, caseChannel, caseTS, requesterUserID string, q threadcase.QuestionPayload) error {
	if uc.deps.SlackService == nil {
		return nil
	}
	pending := threadQuestionToPending(q, uiChannel, "")
	blocks, fallback := buildThreadCreateQuestionBlocks(ctx, q.Reason, pending.Items, encodeCaseThreadValue(caseChannel, caseTS), requesterUserID)
	ts, err := uc.deps.SlackService.PostThreadMessage(ctx, uiChannel, uiTS, blocks, fallback)
	if err != nil {
		return goerr.Wrap(err, "post thread-create question form",
			goerr.V("ui_channel", uiChannel), goerr.V("ui_thread_ts", uiTS))
	}
	pending.PostedMessageTS = ts
	ssn.PendingQuestion = pending
	return nil
}

// HandleThreadCaseQuestionSubmit is the Submit-button entry point for the
// thread-mode case-initialization question form. It validates the answer
// against the pending snapshot, swaps the form into a read-only "answered"
// record, clears the pending question, and resumes the create agent with the
// formatted answers as the next-turn input. Missing answers re-render the form
// with an inline error.
func (uc *AgentUseCase) HandleThreadCaseQuestionSubmit(ctx context.Context, callback *goslack.InteractionCallback, action *goslack.BlockAction) error {
	if callback == nil || action == nil {
		return goerr.New("nil callback or action")
	}
	if uc.threadcase == nil || uc.deps.CaseUC == nil || uc.deps.Registry == nil {
		return nil
	}
	ctx = contextWithSlackUserLang(ctx, uc.deps.SlackService, callback.User.ID)

	// The case thread is carried in the Submit button value as "channel:ts". It
	// coincides with the UI thread for normal thread-mode creation and diverges
	// for a cross-channel reaction. A value without the separator is malformed
	// (or a stale pre-encoding form) — fail loudly rather than guess.
	caseChannel, caseTS, ok := parseCaseThreadValue(action.Value)
	if !ok {
		return goerr.New("malformed question submit value", goerr.V("value", action.Value))
	}
	// The UI thread (where the form is displayed and where form updates go) is
	// the callback's own location.
	uiChannel := callback.Channel.ID
	uiTS := callback.Message.ThreadTimestamp
	if uiTS == "" {
		uiTS = callback.Message.Timestamp
	}
	messageTS := callback.Message.Timestamp

	entry, ok := uc.deps.Registry.FindByMonitorChannel(caseChannel)
	if !ok {
		return nil
	}
	wsID := entry.Workspace.ID

	// If the case is already created, the form is stale.
	if c, err := uc.deps.Repo.Case().GetBySlackThread(ctx, wsID, caseChannel, caseTS); err != nil {
		return goerr.Wrap(err, "look up case for question submit")
	} else if c != nil {
		uc.markThreadQuestionStale(ctx, uiChannel, messageTS)
		return nil
	}

	session, err := uc.deps.Repo.Session().GetByThread(ctx, caseChannel, caseTS)
	if err != nil {
		return goerr.Wrap(err, "load session for question submit",
			goerr.V("case_channel", caseChannel), goerr.V("case_thread_ts", caseTS))
	}
	// Stale if the session moved on, has no pending question, or the pending
	// question is a newer form (a mention superseded this one).
	if session == nil || session.PendingQuestion == nil || session.PendingQuestion.PostedMessageTS != messageTS {
		uc.markThreadQuestionStale(ctx, uiChannel, messageTS)
		return nil
	}

	pq := session.PendingQuestion
	answers := parseDraftQuestionAnswers(pq, callback.BlockActionState)
	reporter := session.CreatorUserID
	if reporter == "" {
		reporter = callback.User.ID
	}
	if missing := missingDraftQuestionItems(pq, answers); len(missing) > 0 {
		uc.repostThreadQuestionWithError(ctx, uiChannel, messageTS, encodeCaseThreadValue(caseChannel, caseTS), reporter, pq, answers, missing)
		return nil
	}

	// Swap the form into a read-only answered record.
	answeredBlocks, answeredFallback := buildDraftQuestionAnsweredBlocks(pq, answers)
	if err := uc.deps.SlackService.UpdateMessage(ctx, uiChannel, messageTS, answeredBlocks, answeredFallback); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "update thread question form to answered view"),
			"falling back to text confirmation")
	}

	// Clear the pending question before resuming so a duplicate submit lands on
	// the stale path instead of re-running the agent.
	answerText := formatDraftQuestionAnswers(pq, answers)
	session.PendingQuestion = nil
	if err := uc.deps.Repo.Session().Put(ctx, session); err != nil {
		errutil.Handle(ctx, err, "clear thread PendingQuestion before resuming")
	}

	// Resume the create agent with the structured answers as the latest input.
	// The dialog (trace / any follow-up question / completion link) surfaces in
	// the UI thread; the Case stays bound to the case thread. createInstruction
	// is empty on resume — the seed context already lives in the gollem history
	// keyed on the session.
	_, err = uc.runThreadCaseCreation(ctx, caseCreateReq{
		entry:       entry,
		caseChannel: caseChannel,
		caseTS:      caseTS,
		uiChannel:   uiChannel,
		uiTS:        uiTS,
		reporter:    reporter,
		mentionText: answerText,
		mentionTS:   messageTS,
		triggerTS:   messageTS,
	})
	return err
}

// repostThreadQuestionWithError re-renders the form with a banner listing the
// items that still need an answer, preserving the reporter mention so they are
// paged again to finish.
func (uc *AgentUseCase) repostThreadQuestionWithError(ctx context.Context, channelID, messageTS, caseThreadValue, requesterUserID string, pq *model.PendingQuestion, answers map[string]draftQuestionAnswer, missing []string) {
	if uc.deps.SlackService == nil {
		return
	}
	blocks, fallback := buildThreadCreateQuestionBlocks(ctx, pq.Reason, pq.Items, caseThreadValue, requesterUserID)
	banner := goslack.NewContextBlock("thread_create_question_error",
		goslack.NewTextBlockObject(goslack.MarkdownType,
			":warning: Please answer every question before submitting.", false, false))
	// Insert the banner right after the header/divider.
	withBanner := make([]goslack.Block, 0, len(blocks)+1)
	withBanner = append(withBanner, blocks[0], banner)
	withBanner = append(withBanner, blocks[1:]...)
	if err := uc.deps.SlackService.UpdateMessage(ctx, channelID, messageTS, withBanner, fallback); err != nil {
		errutil.Handle(ctx, err, "repost thread question form with error")
	}
	_ = answers
	_ = missing
}

// markThreadQuestionStale rewrites a thread-create question form into a single
// context line when the underlying session/pending state has gone away or been
// superseded — removing the Submit button so it can no longer be answered while
// leaving the question text visible in the thread for later reference.
func (uc *AgentUseCase) markThreadQuestionStale(ctx context.Context, channelID, messageTS string) {
	if uc.deps.SlackService == nil || messageTS == "" {
		return
	}
	stale := goslack.NewContextBlock("thread_create_question_stale",
		goslack.NewTextBlockObject(goslack.MarkdownType, "_(This question is no longer active.)_", false, false))
	if err := uc.deps.SlackService.UpdateMessage(ctx, channelID, messageTS, []goslack.Block{stale}, "Question is no longer active."); err != nil {
		errutil.Handle(ctx, err, "mark thread question stale")
	}
}
