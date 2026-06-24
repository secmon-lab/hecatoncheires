package job

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/m-mizutani/goerr/v2"
	goslack "github.com/slack-go/slack"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/interaction"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// Slack identifiers for the interactive-Job question form. They are the
// contract between the rendered Block Kit message and the HTTP interactions
// controller, which routes a submit carrying ActionIDJobQuestionSubmit to the
// Job resume handler. They are deliberately distinct from the proposal
// question form's IDs so the two coexist in the same thread without the
// router confusing them.
const (
	// ActionIDJobQuestionSubmit is the action_id of the Submit button. Its
	// value carries the JSON-encoded resume context (workspace/case/job/run).
	ActionIDJobQuestionSubmit = "job_question_submit"
	// ActionIDJobQuestionChoice is the action_id of the radio_buttons /
	// checkboxes element; per-item disambiguation is via block_id.
	ActionIDJobQuestionChoice = "job_question_choice"
	// ActionIDJobQuestionOther is the action_id of the per-item free-text
	// fallback for closed-list items.
	ActionIDJobQuestionOther = "job_question_other"
	// ActionIDJobQuestionFreeText is the action_id of a free_text item's
	// primary multiline input.
	ActionIDJobQuestionFreeText = "job_question_free_text"
	// blockIDJobQuestionActions hosts the Submit button.
	blockIDJobQuestionActions = "job_question_actions"
	// blockIDJobQuestionItemPrefix prefixes each item's input block_id.
	blockIDJobQuestionItemPrefix = "job_question_item:"
	// blockIDJobQuestionOtherSuffix is appended to a closed-list item's
	// block_id to form its free-text fallback block.
	blockIDJobQuestionOtherSuffix = ":other"
)

// jobQuestionRef is the resume context encoded into the Submit button value.
// The submit handler decodes it to rebuild the JobRunKey + RunID and resume
// the exact suspended run — no Session lookup is involved (Jobs hold their
// pending state on the run record, keyed by RunID).
type jobQuestionRef struct {
	WorkspaceID string `json:"w"`
	CaseID      int64  `json:"c"`
	JobID       string `json:"j"`
	RunID       string `json:"r"`
}

func (r jobQuestionRef) encode() (string, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return "", goerr.Wrap(err, "marshal job question ref")
	}
	return string(b), nil
}

func decodeJobQuestionRef(value string) (jobQuestionRef, error) {
	var r jobQuestionRef
	if err := json.Unmarshal([]byte(value), &r); err != nil {
		return jobQuestionRef{}, goerr.Wrap(err, "unmarshal job question ref")
	}
	if r.WorkspaceID == "" || r.CaseID == 0 || r.JobID == "" || r.RunID == "" {
		return jobQuestionRef{}, goerr.New("incomplete job question ref",
			goerr.V("ref", r))
	}
	return r, nil
}

// jobQuestionPoster is the narrow Slack surface the interactive-Job question
// flow needs: post the form into the case thread (Solicit) and update it in
// place to an answered / error view (submit handler). slacksvc.Service
// satisfies it.
type jobQuestionPoster interface {
	PostThreadMessage(ctx context.Context, channelID, threadTS string, blocks []goslack.Block, text string, opts ...slacksvc.PostThreadOption) (string, error)
	UpdateMessage(ctx context.Context, channelID, timestamp string, blocks []goslack.Block, text string) error
}

// JobInteractor is the interaction.Interactor for a single interactive Job
// run. It persists the pending interaction on the run record (Stage=
// AWAITING_INPUT), marks the JobRun suspended (releasing the lease), and
// posts a Slack question form whose Submit button carries the resume context.
// It is constructed per-run by the JobRunner with that run's identity and
// Slack thread, so Solicit needs only the request.
type JobInteractor struct {
	repo            interfaces.Repository
	poster          jobQuestionPoster
	key             model.JobRunKey
	runID           string
	channelID       string
	threadTS        string
	requesterUserID string
	// runningLog is the Stage=RUNNING JobRunLog created at run start; Solicit
	// transitions a copy of it to AWAITING_INPUT.
	runningLog *model.JobRunLog
	now        func() time.Time
}

// newJobInteractor builds the per-run interactor. The caller (JobRunner)
// supplies the run identity and Slack context; now defaults to time.Now when
// nil so tests can inject a fixed clock.
func newJobInteractor(
	repo interfaces.Repository,
	poster jobQuestionPoster,
	key model.JobRunKey,
	runID, channelID, threadTS, requesterUserID string,
	runningLog *model.JobRunLog,
	now func() time.Time,
) *JobInteractor {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &JobInteractor{
		repo:            repo,
		poster:          poster,
		key:             key,
		runID:           runID,
		channelID:       channelID,
		threadTS:        threadTS,
		requesterUserID: requesterUserID,
		runningLog:      runningLog,
		now:             now,
	}
}

var _ interaction.Interactor = (*JobInteractor)(nil)

// Solicit posts the question to the case thread and suspends the run. It
// returns Outcome{Paused: true}; the executor maps that to a terminating
// QuestionResult and the run ends at AWAITING_INPUT until the user answers.
//
// An interactive Job with no Slack thread to ask in is a hard error (not a
// silent skip): the planner asked a question the host cannot deliver, so the
// run must fail loudly rather than drop the question.
func (i *JobInteractor) Solicit(ctx context.Context, req interaction.Request) (interaction.Outcome, error) {
	if err := req.Validate(); err != nil {
		return interaction.Outcome{}, goerr.Wrap(err, "invalid interaction request",
			goerr.V("job_id", i.key.JobID), goerr.V("run_id", i.runID))
	}
	if i.poster == nil || i.channelID == "" || i.threadTS == "" {
		return interaction.Outcome{}, goerr.New("interactive job has no slack thread to ask in",
			goerr.V("job_id", i.key.JobID), goerr.V("run_id", i.runID),
			goerr.V("channel_id", i.channelID), goerr.V("thread_ts", i.threadTS))
	}

	ref := jobQuestionRef{
		WorkspaceID: i.key.WorkspaceID,
		CaseID:      i.key.CaseID,
		JobID:       i.key.JobID,
		RunID:       i.runID,
	}
	refValue, err := ref.encode()
	if err != nil {
		return interaction.Outcome{}, err
	}

	blocks, fallback := buildJobQuestionBlocks(req, refValue, i.requesterUserID)
	// Post the form FIRST so we can record its message ts on the pending
	// interaction (the resume path updates it in place to an answered view).
	messageTS, err := i.poster.PostThreadMessage(ctx, i.channelID, i.threadTS, blocks, fallback)
	if err != nil {
		return interaction.Outcome{}, goerr.Wrap(err, "post job question form",
			goerr.V("job_id", i.key.JobID), goerr.V("run_id", i.runID))
	}

	pending := &model.PendingInteraction{
		PostedChannelID: i.channelID,
		PostedMessageTS: messageTS,
		Reason:          req.Reason,
		Items:           interactionItemsToPending(req.Items),
	}

	// Transition the run log to AWAITING_INPUT carrying the question.
	suspendLog := *i.runningLog
	suspendLog.Stage = model.JobRunStageAwaitingInput
	suspendLog.PendingInteraction = pending
	if err := i.repo.JobRunLog().Suspend(ctx, &suspendLog); err != nil {
		return interaction.Outcome{}, goerr.Wrap(err, "suspend job run log",
			goerr.V("job_id", i.key.JobID), goerr.V("run_id", i.runID))
	}

	// Mark the JobRun suspended and release the lease so a resume can
	// re-acquire it. SuspendedRunID guards against a concurrent new trigger.
	if err := i.repo.JobRun().Suspend(ctx, i.key, i.runID, i.now()); err != nil {
		return interaction.Outcome{}, goerr.Wrap(err, "mark job run suspended",
			goerr.V("job_id", i.key.JobID), goerr.V("run_id", i.runID))
	}

	return interaction.Outcome{Paused: true}, nil
}

// interactionItemsToPending converts the host-neutral request items into the
// persisted form.
func interactionItemsToPending(items []interaction.Item) []model.PendingInteractionItem {
	out := make([]model.PendingInteractionItem, len(items))
	for i, it := range items {
		out[i] = model.PendingInteractionItem{
			ID:      it.ID,
			Text:    it.Text,
			Type:    string(it.Type),
			Options: append([]string(nil), it.Options...),
		}
	}
	return out
}

// buildJobQuestionBlocks renders the question as a Block Kit form: a header
// (reason, prefixed by the requester @mention when known), one input per
// item (radio for select / checkboxes for multi_select / multiline for
// free_text), a per-closed-list-item free-text fallback, and a Submit button
// carrying refValue. The form chrome is fixed English, matching the proposal
// question form's convention; the question text itself comes from the planner
// already in the user's language.
func buildJobQuestionBlocks(req interaction.Request, refValue, requesterUserID string) ([]goslack.Block, string) {
	header := goslack.NewSectionBlock(
		goslack.NewTextBlockObject(goslack.MarkdownType,
			jobQuestionHeaderText(req.Reason, requesterUserID), false, false),
		nil, nil,
	)
	blocks := []goslack.Block{header, goslack.NewDividerBlock()}

	for _, item := range req.Items {
		if item.Type == interaction.ItemFreeText {
			elem := goslack.NewPlainTextInputBlockElement(
				goslack.NewTextBlockObject(goslack.PlainTextType, "Type your answer here…", false, false),
				ActionIDJobQuestionFreeText,
			)
			elem.Multiline = true
			input := goslack.NewInputBlock(
				blockIDJobQuestionItemPrefix+item.ID,
				goslack.NewTextBlockObject(goslack.PlainTextType, item.Text, false, false),
				nil,
				elem,
			)
			input.Optional = true
			blocks = append(blocks, input)
			continue
		}

		opts := make([]*goslack.OptionBlockObject, 0, len(item.Options))
		for _, optID := range item.Options {
			opts = append(opts, goslack.NewOptionBlockObject(
				optID,
				goslack.NewTextBlockObject(goslack.PlainTextType, optID, false, false),
				nil,
			))
		}

		var element goslack.BlockElement
		if item.Type == interaction.ItemMultiSelect {
			element = goslack.NewCheckboxGroupsBlockElement(ActionIDJobQuestionChoice, opts...)
		} else {
			element = goslack.NewRadioButtonsBlockElement(ActionIDJobQuestionChoice, opts...)
		}
		input := goslack.NewInputBlock(
			blockIDJobQuestionItemPrefix+item.ID,
			goslack.NewTextBlockObject(goslack.PlainTextType, item.Text, false, false),
			nil,
			element,
		)
		input.Optional = true
		blocks = append(blocks, input)

		other := goslack.NewPlainTextInputBlockElement(nil, ActionIDJobQuestionOther)
		otherInput := goslack.NewInputBlock(
			blockIDJobQuestionItemPrefix+item.ID+blockIDJobQuestionOtherSuffix,
			goslack.NewTextBlockObject(goslack.PlainTextType, "Other (free text)", false, false),
			nil,
			other,
		)
		otherInput.Optional = true
		blocks = append(blocks, otherInput)
	}

	submit := goslack.NewButtonBlockElement(
		ActionIDJobQuestionSubmit,
		refValue,
		goslack.NewTextBlockObject(goslack.PlainTextType, "Submit", false, false),
	)
	submit.Style = goslack.StylePrimary
	blocks = append(blocks, goslack.NewActionBlock(blockIDJobQuestionActions, submit))

	return blocks, "This job needs a bit more info to continue."
}

// jobQuestionHeaderText renders the header markdown, paging the requester when
// known.
func jobQuestionHeaderText(reason, requesterUserID string) string {
	var header string
	if requesterUserID != "" {
		header = "<@" + requesterUserID + "> :question: *This job needs your input*"
	} else {
		header = ":question: *This job needs your input*"
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return header
	}
	return header + "\n_" + reason + "_"
}

// buildJobQuestionAnsweredBlocks renders the read-only counterpart shown after
// a successful submit. The Submit button is dropped; each item is paired with
// the user's answer so the thread keeps a permanent Q&A record.
func buildJobQuestionAnsweredBlocks(pending *model.PendingInteraction, answers []interaction.Answer) ([]goslack.Block, string) {
	byID := make(map[string]interaction.Answer, len(answers))
	for _, a := range answers {
		byID[a.ID] = a
	}
	header := goslack.NewSectionBlock(
		goslack.NewTextBlockObject(goslack.MarkdownType,
			jobQuestionHeaderText(pending.Reason, ""), false, false),
		nil, nil,
	)
	blocks := []goslack.Block{header, goslack.NewDividerBlock()}
	for _, item := range pending.Items {
		var b strings.Builder
		b.WriteString("*")
		b.WriteString(item.Text)
		b.WriteString("*\n")
		ans := byID[item.ID]
		parts := answerDisplayParts(item.Type, ans)
		if len(parts) == 0 {
			b.WriteString("_(no answer)_")
		} else {
			b.WriteString(strings.Join(parts, ", "))
		}
		blocks = append(blocks, goslack.NewSectionBlock(
			goslack.NewTextBlockObject(goslack.MarkdownType, b.String(), false, false),
			nil, nil,
		))
	}
	blocks = append(blocks,
		goslack.NewContextBlock("job_question_answered_tail",
			goslack.NewTextBlockObject(goslack.MarkdownType, "_:white_check_mark: Answer received._", false, false),
		),
	)
	return blocks, "Answer received."
}

func answerDisplayParts(itemType string, ans interaction.Answer) []string {
	var parts []string
	if itemType == string(interaction.ItemFreeText) {
		if v := strings.TrimSpace(ans.FreeText); v != "" {
			parts = append(parts, "_"+v+"_")
		}
		return parts
	}
	if ans.Choice != "" {
		parts = append(parts, "`"+ans.Choice+"`")
	}
	for _, c := range ans.Choices {
		parts = append(parts, "`"+c+"`")
	}
	if v := strings.TrimSpace(ans.FreeText); v != "" {
		parts = append(parts, "_"+v+"_")
	}
	return parts
}

// parseJobQuestionAnswers walks the submission state and pairs each input with
// its item by block_id, returning one interaction.Answer per item the user
// engaged with. Items left entirely blank are omitted (the caller treats that
// as unanswered). For closed-list items, a populated "Other" fallback lands in
// FreeText alongside any selection.
func parseJobQuestionAnswers(pending *model.PendingInteraction, state *goslack.BlockActionStates) []interaction.Answer {
	out := make([]interaction.Answer, 0, len(pending.Items))
	if state == nil {
		return out
	}
	for _, item := range pending.Items {
		ans := interaction.Answer{ID: item.ID}
		blockID := blockIDJobQuestionItemPrefix + item.ID
		if blk, ok := state.Values[blockID]; ok {
			switch item.Type {
			case string(interaction.ItemFreeText):
				if act, ok := blk[ActionIDJobQuestionFreeText]; ok {
					ans.FreeText = act.Value
				}
			case string(interaction.ItemMultiSelect):
				if act, ok := blk[ActionIDJobQuestionChoice]; ok {
					for _, opt := range act.SelectedOptions {
						if opt.Value != "" {
							ans.Choices = append(ans.Choices, opt.Value)
						}
					}
				}
			default:
				if act, ok := blk[ActionIDJobQuestionChoice]; ok {
					if v := act.SelectedOption.Value; v != "" {
						ans.Choice = v
					}
				}
			}
		}
		if item.Type != string(interaction.ItemFreeText) {
			otherBlockID := blockID + blockIDJobQuestionOtherSuffix
			if blk, ok := state.Values[otherBlockID]; ok {
				if act, ok := blk[ActionIDJobQuestionOther]; ok {
					ans.FreeText = act.Value
				}
			}
		}
		if !jobAnswerIsEmpty(ans) {
			out = append(out, ans)
		}
	}
	return out
}

func jobAnswerIsEmpty(a interaction.Answer) bool {
	return a.Choice == "" && len(a.Choices) == 0 && strings.TrimSpace(a.FreeText) == ""
}

// missingJobQuestionItems returns the IDs of items the user left blank, used to
// gate submit and re-prompt.
func missingJobQuestionItems(pending *model.PendingInteraction, answers []interaction.Answer) []string {
	answered := make(map[string]struct{}, len(answers))
	for _, a := range answers {
		answered[a.ID] = struct{}{}
	}
	var missing []string
	for _, item := range pending.Items {
		if _, ok := answered[item.ID]; !ok {
			missing = append(missing, item.ID)
		}
	}
	return missing
}

// pendingToInteractionRequest rebuilds a host-neutral request from the
// persisted form so the error re-render path can reuse buildJobQuestionBlocks.
func pendingToInteractionRequest(pending *model.PendingInteraction) interaction.Request {
	items := make([]interaction.Item, len(pending.Items))
	for i, it := range pending.Items {
		items[i] = interaction.Item{
			ID:      it.ID,
			Text:    it.Text,
			Type:    interaction.ItemType(it.Type),
			Options: append([]string(nil), it.Options...),
		}
	}
	return interaction.Request{Reason: pending.Reason, Items: items}
}

// HandleQuestionSubmit is the Slack Submit-button entry point for an
// interactive Job's question form. It decodes the resume context from the
// button value, loads the suspended run, validates that every item was
// answered (re-rendering the form with a banner otherwise), swaps the form
// into a read-only answered view, and resumes the run with the parsed
// answers. A stale submit (the run already resumed / expired) degrades to a
// "no longer active" surface and a no-op. It is the single usecase entry
// point the Slack interactions controller dispatches to.
func (r *JobRunner) HandleQuestionSubmit(ctx context.Context, callback *goslack.InteractionCallback, action *goslack.BlockAction) error {
	if callback == nil || action == nil {
		return goerr.New("nil callback or action")
	}
	ref, err := decodeJobQuestionRef(action.Value)
	if err != nil {
		return goerr.Wrap(err, "decode job question ref")
	}
	key := model.JobRunKey{WorkspaceID: ref.WorkspaceID, CaseID: ref.CaseID, JobID: ref.JobID}
	channelID := callback.Channel.ID
	messageTS := callback.Message.Timestamp

	logRec, err := r.deps.Repo.JobRunLog().Get(ctx, key, ref.RunID)
	if err != nil {
		if errors.Is(err, interfaces.ErrJobRunLogNotFound) {
			r.markJobQuestionStale(ctx, channelID, messageTS)
			return nil
		}
		return goerr.Wrap(err, "load run log for question submit",
			goerr.V("run_id", ref.RunID))
	}
	if logRec.Stage != model.JobRunStageAwaitingInput || logRec.PendingInteraction == nil {
		// Already resumed / completed / expired — drop the stale form.
		r.markJobQuestionStale(ctx, channelID, messageTS)
		return nil
	}
	pending := logRec.PendingInteraction

	answers := parseJobQuestionAnswers(pending, callback.BlockActionState)
	if missing := missingJobQuestionItems(pending, answers); len(missing) > 0 {
		r.repostJobQuestionWithError(ctx, channelID, messageTS, action.Value, pending, missing)
		return nil
	}

	// Swap the form into the read-only answered view before resuming so a
	// duplicate Submit lands on the stale path.
	if r.deps.InteractionPoster != nil {
		answeredBlocks, answeredText := buildJobQuestionAnsweredBlocks(pending, answers)
		if updErr := r.deps.InteractionPoster.UpdateMessage(ctx, channelID, messageTS, answeredBlocks, answeredText); updErr != nil {
			errutil.Handle(ctx, updErr, "update job question form to answered view")
		}
	}

	return r.Resume(ctx, key, ref.RunID, answers)
}

// markJobQuestionStale rewrites the form into a single "no longer active"
// line when the underlying run state has gone away. Best-effort.
func (r *JobRunner) markJobQuestionStale(ctx context.Context, channelID, messageTS string) {
	if r.deps.InteractionPoster == nil || messageTS == "" {
		return
	}
	stale := goslack.NewContextBlock(
		"job_question_stale",
		goslack.NewTextBlockObject(goslack.MarkdownType, "_(This question is no longer active.)_", false, false),
	)
	if err := r.deps.InteractionPoster.UpdateMessage(ctx, channelID, messageTS, []goslack.Block{stale}, "Question is no longer active."); err != nil {
		errutil.Handle(ctx, err, "clear stale job question form")
	}
}

// repostJobQuestionWithError re-renders the form with a banner listing the
// items that still need an answer. The button value (resume context) is
// preserved so a corrected submit resumes the same run.
func (r *JobRunner) repostJobQuestionWithError(ctx context.Context, channelID, messageTS, refValue string, pending *model.PendingInteraction, missing []string) {
	if r.deps.InteractionPoster == nil {
		return
	}
	missingSet := make(map[string]struct{}, len(missing))
	for _, id := range missing {
		missingSet[id] = struct{}{}
	}
	missingTexts := make([]string, 0, len(missing))
	for _, item := range pending.Items {
		if _, ok := missingSet[item.ID]; ok {
			missingTexts = append(missingTexts, item.Text)
		}
	}
	blocks, fallback := buildJobQuestionBlocks(pendingToInteractionRequest(pending), refValue, "")
	banner := goslack.NewSectionBlock(
		goslack.NewTextBlockObject(goslack.MarkdownType,
			":warning: Please answer: "+strings.Join(missingTexts, " / "), false, false),
		nil, nil,
	)
	withBanner := append([]goslack.Block{banner}, blocks...)
	if err := r.deps.InteractionPoster.UpdateMessage(ctx, channelID, messageTS, withBanner, fallback); err != nil {
		errutil.Handle(ctx, err, "re-render job question form with validation banner")
	}
}
