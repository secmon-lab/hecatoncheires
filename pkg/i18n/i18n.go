package i18n

import (
	"context"
	"fmt"
	"strings"

	"github.com/m-mizutani/goerr/v2"
)

type ctxKey struct{}

// ContextWithLang returns a new context with the given language.
func ContextWithLang(ctx context.Context, lang Lang) context.Context {
	return context.WithValue(ctx, ctxKey{}, lang)
}

// LangFromContext extracts the language from context, or returns empty string.
func LangFromContext(ctx context.Context) Lang {
	if lang, ok := ctx.Value(ctxKey{}).(Lang); ok {
		return lang
	}
	return ""
}

// Lang represents a supported language.
type Lang string

const (
	LangEN Lang = "en"
	LangJA Lang = "ja"
)

// MsgKey is an iota-based enum for translation keys.
type MsgKey int

const (
	// Slash Command Modal
	MsgModalCreateCaseTitle MsgKey = iota
	MsgModalCreateCaseSubmit
	MsgModalCreateCaseCancel
	MsgModalNextButton
	MsgFieldTitle
	MsgFieldDescription
	MsgFieldTitlePlaceholder
	MsgFieldDescPlaceholder
	MsgFieldWorkspace

	// Case creation confirmation
	MsgCaseCreated            // "Case #%d *%s* has been created."
	MsgCaseCreatedWithChannel // "Case #%d *%s* has been created. Channel: <#%s>"

	// Action notifications
	MsgActionHeader // "Action: %s %s"
	MsgActionAssignToMe
	MsgActionInProgress
	MsgActionCompleted
	MsgActionNoAssign
	MsgActionStatus // "Status: %s"

	// Action interactive controls (Slack Block Kit)
	MsgActionOpenInWeb
	MsgActionStatusPlaceholder
	MsgActionAssigneePlaceholder

	// Action change notifications (Slack thread)
	MsgActionChangeTitle              // ":pencil2: %s changed title: %q -> %q"
	MsgActionChangeStatus             // ":arrows_counterclockwise: %s changed status: %s -> %s"
	MsgActionChangeAssigneeAssigned   // ":bust_in_silhouette: %s assigned %s"
	MsgActionChangeAssigneeUnassigned // ":bust_in_silhouette: %s unassigned %s"
	MsgActionChangeAssigneeReplaced   // ":bust_in_silhouette: %s changed assignee: %s -> %s"
	MsgActionChangeArchived           // ":file_cabinet: %s archived action %q"
	MsgActionChangeUnarchived         // ":outbox_tray: %s unarchived action %q"
	MsgActionChangeActorSystem        // "system"
	MsgActionStepAdded                // ":heavy_plus_sign: %s added step %q"
	MsgActionStepRemoved              // ":heavy_minus_sign: %s removed step %q"
	MsgActionStepDone                 // ":white_check_mark: %s completed step %q"
	MsgActionStepReopened             // ":arrow_backward: %s reopened step %q"
	MsgActionStepRenamed              // ":pencil2: %s renamed step %q -> %q"

	// Agent
	MsgAgentThinking
	MsgAgentAnalyzing
	MsgAgentProcessing
	MsgAgentInvestigating
	MsgAgentLookingInto
	MsgAgentOnIt
	MsgAgentError
	MsgAgentSessionInfo
	// MsgKeyAgentBusy is used when a new mention arrives while a previous
	// turn on the same Slack thread is still running. The host posts this
	// reply so the user knows the agent is occupied and will respond once
	// the prior turn finishes.
	MsgKeyAgentBusy

	// Thread-mode case messages. Posted to the monitored channel's thread.
	MsgThreadCaseCreated  // "🧵 Case registered. <%s|Open in web UI>" (url)
	MsgThreadCaseUpdated  // ":pencil2: Updated the case details."
	MsgThreadCaseClosed   // ":white_check_mark: Closed this case (%s)." (status name)
	MsgThreadCaseQuestion // ":question: %s" (reason) — question header line

	// Thread-mode case initialization (create) flow.
	MsgThreadCaseCreating       // "🔍 Got it — looking into this…" (initial progress)
	MsgThreadCaseCreateFallback // "I couldn't pull this together into a case yet. Add a little more detail and mention me again."
	MsgThreadCaseSummaryHeader  // "✅ Created a case" (Block Kit summary header)
	MsgThreadCaseSummaryTitle   // "Title" (field label)
	MsgThreadCaseSummaryDesc    // "Description" (field label)
	MsgThreadCaseSummaryStatus  // "Status" (field label)
	MsgThreadCaseSummaryLink    // "<%s|Open in web UI>" (url)

	// Draft (open-mode) planner / sub-agent trace lines. These are rendered
	// into the per-turn Slack progress message so the user can follow what
	// the agent is doing.
	MsgProposalTracePlanning           // "🤔 Planning…"
	MsgProposalTracePlannerRetry       // "⚠️ planner output rejected; retrying"
	MsgProposalTracePlannerAction      // "→ %s — %s" (action, reasoning)
	MsgProposalTracePlannerTool        // "🛠 Planning — calling %s" (tool name)
	MsgProposalTracePlannerMessage     // "🤔 Planning — %s" (one-line excerpt)
	MsgProposalTracePhase              // "🧭 %s" (planner.investigate.message)
	MsgProposalTraceTaskPending        // "⏳ Task: %s" (title) — block reserved before sub-agent starts
	MsgProposalTraceTaskRunning        // "🔍 Task: %s — running…" (title) — initial state when sub-agent begins
	MsgProposalTraceTaskRunningTool    // "🔍 Task: %s — 🛠 calling %s" (title, tool name) — fires per ToolRequestHook
	MsgProposalTraceTaskRunningMessage // "🔍 Task: %s — %s" (title, one-line excerpt) — fires per MessageHook
	MsgProposalTraceTaskDone           // "✅ Task: %s — done (%s, %d/%d inner loops)" (title, elapsed, used, max)
	MsgProposalTraceTaskFailedPrompt   // "❌ Task: %s — failed (%s, build prompt): %v" (title, elapsed, err)
	MsgProposalTraceTaskFailed         // "❌ Task: %s — failed (%s, %d/%d inner loops): %v" (title, elapsed, used, max, err)
	// MsgProposalProcessingCompleted replaces the initial "⏳ Drafting…"
	// placeholder once Materialize has posted the preview at the thread
	// end. It tells the user the placeholder is no longer the live status
	// and to look at the new preview message further down the thread.
	MsgProposalProcessingCompleted

	// Bookmark
	MsgBookmarkOpenCase

	// Cross-workspace
	MsgCrossWorkspaceConnectUnavailable

	// Edit Case Modal
	MsgModalEditCaseTitle
	MsgModalEditCaseSubmit
	MsgCaseUpdated
	MsgErrCaseNotAccessible

	// Private case
	MsgFieldPrivateCase
	MsgFieldPrivateCaseDesc

	// Case-creation "Options" checkbox group (parent label and
	// the additional Draft-mode option that sits alongside Private)
	MsgFieldCaseOptions   // Block label "Options"
	MsgFieldDraftMode     // Option label "Draft mode"
	MsgFieldDraftModeDesc // Description for the Draft mode option

	// Case assignees field
	MsgFieldCaseAssignees

	// Errors
	MsgErrOpenDialog
	MsgErrWorkspaceSelection
	MsgErrCreateCase
	MsgErrEditCase

	// Command choice modal (case channel /cmd without subcommand)
	MsgModalCommandChoiceTitle
	MsgFieldCommandChoice
	MsgChoiceUpdateCase
	MsgChoiceCreateAction

	// Action creation modal
	MsgModalCreateActionTitle
	MsgModalCreateActionSubmit
	MsgFieldAction // "Action" label
	MsgFieldActionTitle
	MsgFieldActionTitlePlaceholder
	MsgFieldActionDescription
	MsgFieldActionDescPlaceholder
	MsgFieldActionAssignee
	MsgFieldActionStatusLabel
	MsgFieldActionDueDate

	// Errors related to commands
	MsgErrUnknownSubcommand // "Unknown subcommand: %s. Available: update, action."
	MsgErrCreateAction      // "Failed to create action. Please try again."

	// Save-as-Draft (Case creation modal)
	MsgDraftSaveAsButton           // Button label "Save as draft"
	MsgDraftSavedEphemeral         // "Saved as draft #%d. Open the Drafts page on the web to continue." (%d = case ID). Used when no web URL is configured.
	MsgDraftSavedEphemeralWithLink // "Saved as draft #%d. <%s|%s> to continue." (%d = case ID, %s = draft URL, %s = link label). Preferred when baseURL is set.
	MsgDraftLinkFallbackLabel      // Link label used when the draft has no title; e.g. "Open the draft on the web"
	MsgDraftSavedModalTitle        // Title of the splash modal shown after Save as Draft.
	MsgDraftSavedModalBody         // Body text of the splash modal: "Saved as draft #%d." (%d = case ID)
	MsgDraftSaveFailedEphemeral    // Error ephemeral when Save as Draft fails server-side.

	// Job run session log (Slack thread). Posted by JobRunner around each
	// Job run to consolidate the run's operational log into one thread.
	MsgJobRunStarting  // "starting... `%s`" (%s = job id)
	MsgJobRunCompleted // "✅ job `%s` completed" (%s = job id)
	MsgJobRunFailed    // "❌ job `%s` failed: %s" (%s = job id, %s = error)

	msgKeyCount // sentinel for validation
)

var (
	messages    map[Lang][msgKeyCount]string
	defaultLang Lang
)

// Init initializes the global translator with the given default language.
// Must be called once at startup before any T() calls.
//
// The translation tables themselves are populated by package init() in
// messages.go — calling Init() only updates the default language fallback,
// so T() is safe to call from tests without an explicit Init().
func Init(lang Lang) {
	defaultLang = lang
}

// T returns the translated string for the language in context.
// Falls back to defaultLang if no language is set in context.
func T(ctx context.Context, key MsgKey, args ...any) string {
	lang := LangFromContext(ctx)
	msg := messages[lang][key]
	if msg == "" {
		msg = messages[defaultLang][key]
	}
	if msg == "" {
		return fmt.Sprintf("[missing:%d]", key)
	}
	if len(args) > 0 {
		return fmt.Sprintf(msg, args...)
	}
	return msg
}

// DefaultLang returns the configured default language.
func DefaultLang() Lang {
	return defaultLang
}

// DetectLang returns the Lang for a Slack locale string.
// Returns empty string if no match (caller should fall back to defaultLang).
func DetectLang(slackLocale string) Lang {
	if strings.HasPrefix(slackLocale, "ja") {
		return LangJA
	}
	if strings.HasPrefix(slackLocale, "en") {
		return LangEN
	}
	return ""
}

// ParseLang validates and returns a Lang from a string (for CLI flags).
func ParseLang(s string) (Lang, error) {
	switch Lang(s) {
	case LangEN:
		return LangEN, nil
	case LangJA:
		return LangJA, nil
	default:
		return "", goerr.New("unsupported language", goerr.V("lang", s))
	}
}
