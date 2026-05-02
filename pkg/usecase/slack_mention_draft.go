package usecase

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
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

// MentionDraftUseCase handles app_mention events that occur in channels NOT
// bound to an existing Case. It collects surrounding context, asks the
// Materializer to produce a workspace-specific Case payload, and presents an
// ephemeral preview with workspace selector + Submit/Edit/Cancel buttons.
type MentionDraftUseCase struct {
	repo         interfaces.Repository
	registry     *model.WorkspaceRegistry
	slackService slacksvc.Service
	collector    *slacksvc.MessageCollector
	materializer *DraftMaterializer
}

// NewMentionDraftUseCase constructs a MentionDraftUseCase.
func NewMentionDraftUseCase(
	repo interfaces.Repository,
	registry *model.WorkspaceRegistry,
	slackService slacksvc.Service,
	materializer *DraftMaterializer,
) *MentionDraftUseCase {
	if slackService == nil {
		return nil
	}
	return &MentionDraftUseCase{
		repo:         repo,
		registry:     registry,
		slackService: slackService,
		collector:    slacksvc.NewMessageCollector(slackService),
		materializer: materializer,
	}
}

// HandleAppMention runs the full initial-mention flow: candidate workspace
// resolution → message collection → draft persistence → AI materialization →
// ephemeral preview post.
//
// It is the caller's responsibility to ensure the channel is NOT bound to an
// existing Case (the dispatch in SlackUseCases handles that branch).
func (uc *MentionDraftUseCase) HandleAppMention(ctx context.Context, ev *slackevents.AppMentionEvent) error {
	if ev == nil {
		return goerr.New("AppMentionEvent is nil")
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

	estimated, estimationReason, err := uc.estimateWorkspace(ctx, candidates, ev.User)
	if err != nil {
		uc.removeProcessingMessage(ctx, ev.Channel, processingTS)
		return goerr.Wrap(err, "failed to estimate workspace for mention")
	}

	mentionTime := parseSlackTS(ev.TimeStamp)

	var msgs []model.DraftMessage
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

	draft := model.NewCaseDraft(time.Now().UTC(), ev.User)
	draft.MentionText = ev.Text
	draft.RawMessages = msgs
	draft.Source = model.DraftSource{
		ChannelID: ev.Channel,
		ThreadTS:  ev.ThreadTimeStamp,
		MentionTS: ev.TimeStamp,
	}
	draft.SelectedWorkspaceID = estimated.Workspace.ID
	draft.InferenceInProgress = true

	if err := uc.repo.CaseDraft().Save(ctx, draft); err != nil {
		uc.removeProcessingMessage(ctx, ev.Channel, processingTS)
		return goerr.Wrap(err, "failed to save initial draft")
	}

	// Run AI inference (this is the longest leg of the flow). Materialize
	// retries internally; if it still fails, surface the error to the user
	// via a thread message so they aren't left silently waiting.
	mat, err := uc.materializer.Materialize(ctx, draft, MaterializeContext{
		Workspace:        estimated,
		EstimationReason: estimationReason,
		OtherCandidates:  candidates,
	})
	if err != nil {
		uc.removeProcessingMessage(ctx, ev.Channel, processingTS)
		uc.notifyMaterializationFailed(ctx, ev, err)
		return goerr.Wrap(err, "failed to materialize draft")
	}

	if err := uc.repo.CaseDraft().SetMaterialization(ctx, draft.ID, estimated.Workspace.ID, mat, false); err != nil {
		return goerr.Wrap(err, "failed to persist materialization")
	}

	// Reload draft so that EphemeralChannelID/MessageTS gets persisted in one place
	// after we know the ephemeral TS.
	draft.Materialization = mat
	draft.InferenceInProgress = false

	blocks, fallback := buildPreviewBlocks(draft, estimated, candidates)

	// If we successfully posted the processing placeholder, update it in
	// place so the preview replaces "processing…" cleanly. Otherwise post a
	// fresh thread message.
	var ts string
	if processingTS != "" {
		if updErr := uc.slackService.UpdateMessage(ctx, ev.Channel, processingTS, blocks, fallback); updErr != nil {
			errutil.Handle(ctx, goerr.Wrap(updErr, "failed to update processing message into preview"),
				"falling back to fresh thread post")
		} else {
			ts = processingTS
		}
	}
	if ts == "" {
		newTS, err := uc.slackService.PostThreadMessage(ctx, ev.Channel, threadTS, blocks, fallback)
		if err != nil {
			return goerr.Wrap(err, "failed to post preview message",
				goerr.V("channel_id", ev.Channel),
				goerr.V("thread_ts", threadTS),
			)
		}
		ts = newTS
	}

	draft.EphemeralChannelID = ev.Channel
	draft.EphemeralMessageTS = ts
	if err := uc.repo.CaseDraft().Save(ctx, draft); err != nil {
		return goerr.Wrap(err, "failed to save draft with ephemeral ref")
	}

	logger.Info("case draft created from slack mention",
		"draft_id", draft.ID,
		"workspace_id", estimated.Workspace.ID,
		"channel_id", ev.Channel,
		"user_id", ev.User,
	)
	return nil
}

func (uc *MentionDraftUseCase) accessibleWorkspaces(_ string) []*model.WorkspaceEntry {
	// Currently every registered workspace is treated as accessible. Per-user
	// workspace authorization is out of scope for this feature; refining it
	// would feed in here without changing the rest of the flow.
	if uc.registry == nil {
		return nil
	}
	return uc.registry.List()
}

// estimateWorkspace picks one workspace from the candidates per F5 and
// returns a short human-readable reason describing why that one was chosen
// (this reason is also surfaced to the LLM via the prompt).
//   - 0 candidates: caller handles upstream
//   - 1 candidate: that one (self-evident)
//   - >1: the user's most recent Case workspace within the past 30d (best
//     effort: scans each candidate's Cases and picks the most recent reporter
//     match), falling back to the first registered when no recent Case is
//     found.
func (uc *MentionDraftUseCase) estimateWorkspace(ctx context.Context, candidates []*model.WorkspaceEntry, userID string) (*model.WorkspaceEntry, string, error) {
	switch len(candidates) {
	case 0:
		return nil, "", ErrNoAccessibleWorkspace
	case 1:
		return candidates[0], "only workspace this user has access to", nil
	}

	if recent, ok := uc.recentCaseWorkspace(ctx, candidates, userID); ok {
		return recent, "user's most recent reporter activity within the past 30 days", nil
	}
	return candidates[0], "fallback to first registered workspace (no recent reporter activity found)", nil
}

const recentCaseLookback = 30 * 24 * time.Hour

func (uc *MentionDraftUseCase) recentCaseWorkspace(ctx context.Context, candidates []*model.WorkspaceEntry, userID string) (*model.WorkspaceEntry, bool) {
	logger := logging.From(ctx)
	cutoff := time.Now().Add(-recentCaseLookback)

	var (
		bestEntry *model.WorkspaceEntry
		bestTime  time.Time
	)
	for _, entry := range candidates {
		cases, err := uc.repo.Case().List(ctx, entry.Workspace.ID)
		if err != nil {
			logger.Debug("Case().List failed; skipping workspace for recency check",
				"workspace_id", entry.Workspace.ID,
				"error", err,
			)
			continue
		}
		for _, c := range cases {
			if c.ReporterID != userID {
				continue
			}
			if c.CreatedAt.Before(cutoff) {
				continue
			}
			if c.CreatedAt.After(bestTime) {
				bestTime = c.CreatedAt
				bestEntry = entry
			}
		}
	}
	if bestEntry == nil {
		return nil, false
	}
	return bestEntry, true
}

func (uc *MentionDraftUseCase) notifyNoWorkspace(ctx context.Context, ev *slackevents.AppMentionEvent) error {
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
func (uc *MentionDraftUseCase) notifyMaterializationFailed(ctx context.Context, ev *slackevents.AppMentionEvent, cause error) {
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
	draft *model.CaseDraft,
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

// buildCreatedBlocks renders the post-create state in the same shape as the
// preview, minus the workspace selector and action buttons, plus a final
// "✅ Created" context line. The created Case's actual fields are used so the
// rendered values reflect any edits made via the Edit modal.
func buildCreatedBlocks(entry *model.WorkspaceEntry, created *model.Case) ([]goslack.Block, string) {
	if created == nil {
		return nil, "Case created"
	}

	blocks := []goslack.Block{
		buildTitleAndDescriptionMarkdown("mention_created_body", created.Title, created.Description),
		goslack.NewDividerBlock(),
	}

	if entry != nil && entry.FieldSchema != nil {
		blocks = append(blocks, buildFieldPairSections(entry.FieldSchema.Fields, created.FieldValues)...)
	}

	tail := fmt.Sprintf("✅ *Created* — Case #%d", created.ID)
	if created.SlackChannelID != "" {
		tail = fmt.Sprintf("✅ *Created* — Case #%d in <#%s>", created.ID, created.SlackChannelID)
	}
	blocks = append(blocks,
		goslack.NewContextBlock(
			"mention_draft_created_tail",
			goslack.NewTextBlockObject(goslack.MarkdownType, tail, false, false),
		),
	)

	fallback := fmt.Sprintf("Created case #%d: %s", created.ID, created.Title)
	return blocks, fallback
}

// buildProcessingContextBlocks renders an immediate "processing…" placeholder
// posted right after a mention is received and before the (slow) LLM call.
// It is later UpdateMessage-replaced with the full preview, or DeleteMessage'd
// on early-failure paths (no workspace / collection failure / etc).
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

// removeProcessingMessage replaces the placeholder with a single context line
// indicating the flow ended without producing a preview. We use UpdateMessage
// (not DeleteMessage) because the Slack API delete scopes are stricter and
// not always granted to bots.
func (uc *MentionDraftUseCase) removeProcessingMessage(ctx context.Context, channelID, ts string) {
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

// buildFieldPairSections renders schema fields as a single SectionBlock
// whose fields[] entries are plain_text. Slack's section.fields[] grid
// reliably renders side-by-side in 2 columns when the entries are
// plain_text — multi-line mrkdwn entries can collapse to one column.
// Markdown emphasis isn't available here, so we use ASCII conventions:
// "Label:" newline "value", and required-missing fields are tagged with
// "⚠ required — not set" instead of italic markers.
func buildFieldPairSections(fields []config.FieldDefinition, values map[string]model.FieldValue) []goslack.Block {
	if len(fields) == 0 {
		return nil
	}
	cells := make([]*goslack.TextBlockObject, 0, len(fields))
	for _, fd := range fields {
		label := fallbackText(fd.Name, fd.ID)
		fv, present := values[fd.ID]
		hasValue := present && !isFieldValueEmpty(fv)
		var valueText string
		switch {
		case hasValue:
			valueText = stripMrkdwnFormatting(formatFieldValueForDisplay(fd, fv))
		case fd.Required:
			valueText = "⚠ required — not set"
		default:
			valueText = "(not set)"
		}
		cells = append(cells, goslack.NewTextBlockObject(
			goslack.PlainTextType,
			fmt.Sprintf("%s:\n%s", label, valueText),
			true, false,
		))
	}
	return []goslack.Block{goslack.NewSectionBlock(nil, cells, nil)}
}

// stripMrkdwnFormatting removes mrkdwn-only markers (backticks, italic
// underscores) from a value string so it renders cleanly inside a
// plain_text TextBlockObject. Slack user/channel mentions ("<@U123>",
// "<#C123>") are left intact — plain_text fields render those as the
// underlying ID, which is acceptable for the at-a-glance preview.
func stripMrkdwnFormatting(s string) string {
	r := strings.NewReplacer("`", "", "_", "")
	return r.Replace(s)
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
	sb.WriteString("# ")
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
