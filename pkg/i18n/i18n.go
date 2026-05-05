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
	MsgActionStatus  // "Status: %s"
	MsgActionNew     // "New action: %s"
	MsgActionUpdated // "Action updated: %s"

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
	MsgActionChangeActorSystem        // "system"

	// Agent
	MsgAgentThinking
	MsgAgentAnalyzing
	MsgAgentProcessing
	MsgAgentInvestigating
	MsgAgentLookingInto
	MsgAgentOnIt
	MsgAgentError
	MsgAgentSessionInfo

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

	msgKeyCount // sentinel for validation
)

var (
	messages    map[Lang][msgKeyCount]string
	defaultLang Lang
)

// Init initializes the global translator with the given default language.
// Must be called once at startup before any T() calls.
func Init(lang Lang) {
	defaultLang = lang
	messages = map[Lang][msgKeyCount]string{
		LangEN: messagesEN,
		LangJA: messagesJA,
	}
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
