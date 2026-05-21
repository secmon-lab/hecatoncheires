package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/draft"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	goslack "github.com/slack-go/slack"
)

// Slack identifiers for the open-mode question form. The action_ids and
// block_ids are the contract between the rendered Block Kit message and the
// HTTP interactions controller.
const (
	// ActionIDDraftQuestionChoice is the action_id of the radio_buttons /
	// checkboxes element rendering the choices for a single question item.
	// Same id across all items; per-item disambiguation is done via block_id.
	ActionIDDraftQuestionChoice = "mention_draft_question_choice"
	// ActionIDDraftQuestionSubmit is the action_id of the Submit button.
	// The button's value carries the draft id so the submit handler can
	// re-enter the draft flow without parsing block_ids.
	ActionIDDraftQuestionSubmit = "mention_draft_question_submit"
	// ActionIDDraftQuestionOther is the action_id of the plain_text_input
	// element rendering the per-item free-form fallback. Same id across all
	// items; per-item disambiguation is via block_id (item suffix `:other`).
	ActionIDDraftQuestionOther = "mention_draft_question_other"
	// ActionIDDraftQuestionFreeText is the action_id of the plain_text_input
	// element rendering a `free_text` question item's primary control. The
	// item has no select / checkbox companion; its single multiline input
	// IS the answer surface, so a distinct action_id keeps the parser path
	// clean from the per-item Other fallback used by the closed-list
	// types.
	ActionIDDraftQuestionFreeText = "mention_draft_question_free_text"
	// BlockIDDraftQuestionActions is the block_id of the actions block
	// hosting the Submit button. Distinct from the per-question input
	// block_ids so dispatch can ignore it when matching answers.
	BlockIDDraftQuestionActions = "mention_draft_question_actions"
	// BlockIDDraftQuestionItemPrefix is prepended to each question item's
	// block_id so the submit handler can recognise (and skip) the actions
	// block while iterating state values. Item ID follows the prefix.
	BlockIDDraftQuestionItemPrefix = "mention_draft_question_item:"
	// BlockIDDraftQuestionOtherSuffix is appended to a per-item block_id to
	// form the block_id of its free-text fallback InputBlock. Pairing the
	// fallback against the item is done by stripping this suffix.
	BlockIDDraftQuestionOtherSuffix = ":other"
)

// draftQuestionAnswer is the parsed per-item answer.
//
// For `select` / `multi_select` items: Selections holds the chosen
// option IDs and OtherText holds the user's free-form fallback (the
// "Other (free text)" companion input).
//
// For `free_text` items: Selections is always empty and OtherText holds
// the user's prose answer (the item's primary input). Treating the
// prose as OtherText keeps the data model uniform; downstream code
// reads it via the same field.
type draftQuestionAnswer struct {
	Selections []string
	OtherText  string
}

// IsEmpty reports whether the user provided no answer for this item — neither
// a selection nor any free-form text. Used by the validation gate.
func (a draftQuestionAnswer) IsEmpty() bool {
	return len(a.Selections) == 0 && strings.TrimSpace(a.OtherText) == ""
}

// buildProposalQuestionBlocks renders the planner's question payload as a
// Block Kit form: a header (reason) prefixed by an @mention of the
// requester so they get paged immediately, one input section per question
// item (radio_buttons for select / checkboxes for multi_select), a
// per-item free-text fallback, and a primary Submit button at the bottom.
// The Submit button's value carries the draft id so the submit handler
// can re-enter the draft flow. requesterUserID is the Slack user id of
// the person who originally @-mentioned the bot; pass empty to suppress
// the mention (used in tests / synthetic flows).
func buildProposalQuestionBlocks(q draft.QuestionPayload, proposalID model.CaseProposalID, requesterUserID string) ([]goslack.Block, string) {
	header := goslack.NewSectionBlock(
		goslack.NewTextBlockObject(goslack.MarkdownType,
			questionHeaderText(q.Reason, requesterUserID), false, false),
		nil, nil,
	)
	blocks := []goslack.Block{header, goslack.NewDividerBlock()}

	for _, item := range q.Items {
		if item.Type == draft.QuestionItemFreeText {
			// `free_text`: the item's only control is a multiline
			// plain-text input. No closed-list companion, no Other
			// fallback — the prose is the answer.
			elem := goslack.NewPlainTextInputBlockElement(
				goslack.NewTextBlockObject(goslack.PlainTextType, "Type your answer here…", false, false),
				ActionIDDraftQuestionFreeText,
			)
			elem.Multiline = true
			input := goslack.NewInputBlock(
				BlockIDDraftQuestionItemPrefix+item.ID,
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
		if item.Type == draft.QuestionItemMultiSelect {
			element = goslack.NewCheckboxGroupsBlockElement(ActionIDDraftQuestionChoice, opts...)
		} else {
			element = goslack.NewRadioButtonsBlockElement(ActionIDDraftQuestionChoice, opts...)
		}

		input := goslack.NewInputBlock(
			BlockIDDraftQuestionItemPrefix+item.ID,
			goslack.NewTextBlockObject(goslack.PlainTextType, item.Text, false, false),
			nil,
			element,
		)
		// Validation is enforced server-side in HandleQuestionSubmit so we
		// can re-render the form with a clear error on missing answers,
		// rather than relying on Slack's terse non-optional rejection.
		input.Optional = true
		blocks = append(blocks, input)

		// Free-text fallback. Both `select` and `multi_select` items get one
		// so the user can supply an answer that does not appear in the
		// planner-supplied options. Single-line (Multiline=false) — answers
		// here are short labels, not paragraphs. Optional=true: server-side
		// validation (in HandleQuestionSubmit) accepts the item if EITHER
		// the choice or the fallback is populated.
		other := goslack.NewPlainTextInputBlockElement(nil, ActionIDDraftQuestionOther)
		otherInput := goslack.NewInputBlock(
			BlockIDDraftQuestionItemPrefix+item.ID+BlockIDDraftQuestionOtherSuffix,
			goslack.NewTextBlockObject(goslack.PlainTextType, "Other (free text)", false, false),
			nil,
			other,
		)
		otherInput.Optional = true
		blocks = append(blocks, otherInput)
	}

	submit := goslack.NewButtonBlockElement(
		ActionIDDraftQuestionSubmit,
		string(proposalID),
		goslack.NewTextBlockObject(goslack.PlainTextType, "Submit", false, false),
	)
	submit.Style = goslack.StylePrimary
	blocks = append(blocks, goslack.NewActionBlock(BlockIDDraftQuestionActions, submit))

	fallback := "We need a bit more info to draft this case."
	return blocks, fallback
}

// buildDraftQuestionAnsweredBlocks renders the read-only counterpart of
// buildProposalQuestionBlocks for use after a successful Submit. The header is
// preserved, each question is paired with the user's selections plus any
// free-text fallback, and the Submit button is dropped — the message
// becomes a permanent record of the Q&A in the thread.
func buildDraftQuestionAnsweredBlocks(pq *model.PendingQuestion, answers map[string]draftQuestionAnswer) ([]goslack.Block, string) {
	header := goslack.NewSectionBlock(
		goslack.NewTextBlockObject(goslack.MarkdownType,
			// No mention on the read-only view: the requester already
			// answered, paging them again on every re-render is noise.
			questionHeaderText(pq.Reason, ""), false, false),
		nil, nil,
	)
	blocks := []goslack.Block{header, goslack.NewDividerBlock()}

	for _, item := range pq.Items {
		var b strings.Builder
		fmt.Fprintf(&b, "*%s*\n", escapeMarkdownInline(item.Text))
		ans := answers[item.ID]
		var parts []string
		if item.Type == string(draft.QuestionItemFreeText) {
			// `free_text` items render the prose directly without
			// the backtick "selection" framing used for closed lists.
			if v := strings.TrimSpace(ans.OtherText); v != "" {
				parts = append(parts, "_"+escapeMarkdownInline(v)+"_")
			}
		} else {
			for _, p := range ans.Selections {
				parts = append(parts, "`"+p+"`")
			}
			if other := strings.TrimSpace(ans.OtherText); other != "" {
				parts = append(parts, "_"+escapeMarkdownInline(other)+"_")
			}
		}
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
		goslack.NewContextBlock("mention_draft_question_answered_tail",
			goslack.NewTextBlockObject(goslack.MarkdownType,
				"_:white_check_mark: Answers received._", false, false),
		),
	)
	return blocks, "Answers received."
}

// questionHeaderText renders the header section markdown. When
// requesterUserID is non-empty, the line is prefixed with `<@USER>` so the
// requester is paged the moment the form posts.
func questionHeaderText(reason, requesterUserID string) string {
	var header string
	if requesterUserID != "" {
		header = "<@" + requesterUserID + "> :question: *Need a bit more info*"
	} else {
		header = ":question: *Need a bit more info*"
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return header
	}
	return header + "\n_" + reason + "_"
}

// parseDraftQuestionAnswers walks the submission state and pairs each input
// value with its question item by block_id. Returns a per-item answer pair
// (selected option IDs + free-form fallback text). Items the user did not
// interact with at all are absent from the map (callers treat that as
// unanswered); items where only the choice or only the fallback was filled
// have the populated half non-zero.
func parseDraftQuestionAnswers(pq *model.PendingQuestion, state *goslack.BlockActionStates) map[string]draftQuestionAnswer {
	out := make(map[string]draftQuestionAnswer, len(pq.Items))
	if state == nil {
		return out
	}
	for _, item := range pq.Items {
		var ans draftQuestionAnswer

		choiceBlockID := BlockIDDraftQuestionItemPrefix + item.ID
		if blk, ok := state.Values[choiceBlockID]; ok {
			switch item.Type {
			case string(draft.QuestionItemFreeText):
				// `free_text`: the item's primary control is a single
				// plain_text_input with our free-text action_id. Park
				// the prose into OtherText so downstream code reads
				// the answer through the same field as the
				// closed-list "Other" fallback.
				if act, ok := blk[ActionIDDraftQuestionFreeText]; ok {
					ans.OtherText = act.Value
				}
			case string(draft.QuestionItemMultiSelect):
				if act, ok := blk[ActionIDDraftQuestionChoice]; ok {
					for _, opt := range act.SelectedOptions {
						if opt.Value == "" {
							continue
						}
						ans.Selections = append(ans.Selections, opt.Value)
					}
				}
			default:
				if act, ok := blk[ActionIDDraftQuestionChoice]; ok {
					if v := act.SelectedOption.Value; v != "" {
						ans.Selections = []string{v}
					}
				}
			}
		}

		// Closed-list items also carry a per-item "Other (free text)"
		// fallback in a sibling block. `free_text` items don't render
		// that block, so the lookup is a harmless miss.
		if item.Type != string(draft.QuestionItemFreeText) {
			otherBlockID := BlockIDDraftQuestionItemPrefix + item.ID + BlockIDDraftQuestionOtherSuffix
			if blk, ok := state.Values[otherBlockID]; ok {
				if act, ok := blk[ActionIDDraftQuestionOther]; ok {
					ans.OtherText = act.Value
				}
			}
		}

		if !ans.IsEmpty() {
			out[item.ID] = ans
		}
	}
	return out
}

// formatDraftQuestionAnswers renders the user's answers as a markdown text
// to feed back into the planner as the next-turn user input. Each item is
// labelled with its question text so the planner sees a self-describing
// dialogue line rather than opaque IDs. Free-text content is preserved
// verbatim — the planner needs the literal user copy to decide.
func formatDraftQuestionAnswers(pq *model.PendingQuestion, answers map[string]draftQuestionAnswer) string {
	var b strings.Builder
	b.WriteString("# Answers to my prior questions\n\n")
	for _, item := range pq.Items {
		fmt.Fprintf(&b, "## %s\n", item.Text)
		ans := answers[item.ID]
		var parts []string
		if item.Type == string(draft.QuestionItemFreeText) {
			// `free_text` items have no closed-list companion, so the
			// "selected:" / "other:" framing would be misleading.
			// Surface the prose under a clear label.
			if v := strings.TrimSpace(ans.OtherText); v != "" {
				parts = append(parts, "answer: "+v)
			}
		} else {
			if len(ans.Selections) > 0 {
				parts = append(parts, "selected: "+strings.Join(ans.Selections, ", "))
			}
			if other := strings.TrimSpace(ans.OtherText); other != "" {
				parts = append(parts, "other: "+other)
			}
		}
		if len(parts) == 0 {
			b.WriteString("(no answer)\n\n")
			continue
		}
		fmt.Fprintf(&b, "%s\n\n", strings.Join(parts, "; "))
	}
	return b.String()
}

// missingDraftQuestionItems returns the IDs of items the user left blank
// (no selection AND no free-text fallback). Used to gate Submit and
// re-prompt the form with an inline error.
func missingDraftQuestionItems(pq *model.PendingQuestion, answers map[string]draftQuestionAnswer) []string {
	var missing []string
	for _, item := range pq.Items {
		ans, ok := answers[item.ID]
		if !ok || ans.IsEmpty() {
			missing = append(missing, item.ID)
		}
	}
	return missing
}

// HandleQuestionSubmit is the Submit-button entry point for the open-mode
// question form. It loads the live session, parses the user's selections
// against the pending question snapshot, validates that every item has an
// answer, swaps the form message into a read-only "answered" record, and
// resumes the planner with the formatted answers as the next-turn user
// input. Validation failures re-render the form with an inline error so
// the user can fix and resubmit.
func (uc *MentionProposalUseCase) HandleQuestionSubmit(ctx context.Context, callback *goslack.InteractionCallback, action *goslack.BlockAction) error {
	if uc.draftUC == nil {
		return goerr.New("draft usecase is not configured")
	}
	if callback == nil || action == nil {
		return goerr.New("nil callback or action")
	}
	ctx = contextWithSlackUserLang(ctx, uc.slackService, callback.User.ID)
	logger := logging.From(ctx)

	channelID := callback.Channel.ID
	threadTS := callback.Message.ThreadTimestamp
	if threadTS == "" {
		threadTS = callback.Message.Timestamp
	}
	messageTS := callback.Message.Timestamp

	session, err := uc.repo.Session().GetByThread(ctx, channelID, threadTS)
	if err != nil {
		return goerr.Wrap(err, "load session for question submit",
			goerr.V("channel_id", channelID),
			goerr.V("thread_ts", threadTS),
		)
	}
	if session == nil || session.PendingQuestion == nil {
		// Form must have been answered by a parallel submission, or the
		// session moved on. Drop the surface state so the user is not
		// stuck looking at a stale form.
		uc.markQuestionStale(ctx, channelID, messageTS)
		return nil
	}

	pq := session.PendingQuestion
	answers := parseDraftQuestionAnswers(pq, callback.BlockActionState)
	requesterID := session.CreatorUserID
	if requesterID == "" {
		requesterID = callback.User.ID
	}
	if missing := missingDraftQuestionItems(pq, answers); len(missing) > 0 {
		uc.repostQuestionWithError(ctx, channelID, messageTS, requesterID, pq, answers, missing)
		return nil
	}

	answeredBlocks, answeredFallback := buildDraftQuestionAnsweredBlocks(pq, answers)
	if err := uc.slackService.UpdateMessage(ctx, channelID, messageTS, answeredBlocks, answeredFallback); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "update question form to answered view"),
			"falling back to text confirmation")
	}

	// Clear the pending snapshot before resuming so a duplicate Submit
	// (network retry, double-click) lands on the stale-form path instead
	// of re-running the planner.
	session.PendingQuestion = nil
	if err := uc.repo.Session().Put(ctx, session); err != nil {
		errutil.Handle(ctx, err, "clear PendingQuestion before resuming planner")
	}

	var (
		d          *model.CaseProposal
		proposalID model.CaseProposalID
	)
	if session.ProposalID != "" {
		d, err = uc.repo.CaseProposal().Get(ctx, session.ProposalID)
		if err != nil {
			errutil.Handle(ctx, err, "thread-reply: failed to load draft; continuing without it")
		}
	}
	if d != nil {
		proposalID = d.ID
	}
	candidates := uc.accessibleWorkspaces(callback.User.ID)

	handler := newSlackDraftHandler(
		uc.repo, uc.registry, uc.slackService,
		channelID, threadTS, messageTS, callback.User.ID,
		candidates, proposalID, "", "",
	)

	userInput := formatDraftQuestionAnswers(pq, answers)
	result, runErr := uc.draftUC.RunTurn(ctx, draft.TurnRequest{
		Session:          session,
		UserInput:        userInput,
		Trigger:          draft.TriggerThreadReply,
		TriggerTS:        messageTS,
		ActorUserID:      callback.User.ID,
		ExistingProposal: d,
		Handler:          handler,
	})
	if runErr != nil {
		return goerr.Wrap(runErr, "draft question submit turn failed")
	}
	if result.Status == draft.StatusFallback {
		uc.notifyDraftFallback(ctx, channelID, threadTS, result.FallbackReason)
	}

	logger.Info("draft question submit turn finished",
		"channel_id", channelID,
		"thread_ts", threadTS,
		"user_id", callback.User.ID,
		"status", int(result.Status),
		"ended_with", string(result.EndedWith),
	)
	return nil
}

// markQuestionStale rewrites the form message into a single context line
// when the underlying session/pending state has gone away. Best-effort.
func (uc *MentionProposalUseCase) markQuestionStale(ctx context.Context, channelID, messageTS string) {
	if messageTS == "" {
		return
	}
	staleBlock := goslack.NewContextBlock(
		"mention_draft_question_stale",
		goslack.NewTextBlockObject(goslack.MarkdownType,
			"_(This question is no longer active.)_", false, false),
	)
	if err := uc.slackService.UpdateMessage(ctx, channelID, messageTS, []goslack.Block{staleBlock}, "Question is no longer active."); err != nil {
		errutil.Handle(ctx, err, "clear stale question form")
	}
}

// repostQuestionWithError re-renders the form with a banner listing items
// that still need an answer. The user's prior selections are preserved by
// re-rendering the original blocks (Slack will keep state.Values for
// unchanged elements when the message id is reused). requesterUserID is
// re-mentioned in the header so the original requester gets paged again
// to finish answering.
func (uc *MentionProposalUseCase) repostQuestionWithError(ctx context.Context, channelID, messageTS, requesterUserID string, pq *model.PendingQuestion, answers map[string]draftQuestionAnswer, missing []string) {
	blocks, fallback := buildProposalQuestionBlocks(draft.QuestionPayload{
		Reason: pq.Reason,
		Items:  pendingItemsToDraftItems(pq.Items),
	}, "", requesterUserID)
	missingSet := make(map[string]struct{}, len(missing))
	for _, id := range missing {
		missingSet[id] = struct{}{}
	}
	missingTexts := make([]string, 0, len(missing))
	for _, item := range pq.Items {
		if _, ok := missingSet[item.ID]; ok {
			missingTexts = append(missingTexts, item.Text)
		}
	}
	banner := goslack.NewSectionBlock(
		goslack.NewTextBlockObject(goslack.MarkdownType,
			":warning: Please answer: "+strings.Join(missingTexts, " / "),
			false, false),
		nil, nil,
	)
	withBanner := append([]goslack.Block{banner}, blocks...)
	if err := uc.slackService.UpdateMessage(ctx, channelID, messageTS, withBanner, fallback); err != nil {
		errutil.Handle(ctx, err, "re-render question form with validation banner")
	}
	_ = answers
}

// pendingItemsToDraftItems is a thin shim to reuse buildProposalQuestionBlocks
// for the re-render path without leaking model.PendingQuestionItem into the
// draft package.
func pendingItemsToDraftItems(in []model.PendingQuestionItem) []draft.QuestionItem {
	out := make([]draft.QuestionItem, len(in))
	for i, it := range in {
		out[i] = draft.QuestionItem{
			ID: it.ID, Text: it.Text,
			Type:    draft.QuestionItemType(it.Type),
			Options: append([]string(nil), it.Options...),
		}
	}
	return out
}
