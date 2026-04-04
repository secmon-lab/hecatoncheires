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

	// Agent
	MsgAgentThinking
	MsgAgentAnalyzing
	MsgAgentProcessing
	MsgAgentInvestigating
	MsgAgentLookingInto
	MsgAgentOnIt
	MsgAgentError
	MsgAgentSessionInfo

	// Knowledge
	MsgKnowledgeHeader // "Knowledge: %s"
	MsgKnowledgeSource
	MsgKnowledgeLink

	// Bookmark
	MsgBookmarkOpenCase

	// Errors
	MsgErrOpenDialog
	MsgErrWorkspaceSelection
	MsgErrCreateCase

	msgKeyCount // sentinel for validation
)

// Translator provides translations for a given language with a default fallback.
type Translator struct {
	messages    map[Lang][msgKeyCount]string
	defaultLang Lang
}

// New creates a new Translator with the given default language.
func New(defaultLang Lang) *Translator {
	return &Translator{
		messages: map[Lang][msgKeyCount]string{
			LangEN: messagesEN,
			LangJA: messagesJA,
		},
		defaultLang: defaultLang,
	}
}

// T returns the translated string for the language in context.
// Falls back to defaultLang if no language is set in context.
// If args are provided, fmt.Sprintf is used to format the result.
func (tr *Translator) T(ctx context.Context, key MsgKey, args ...any) string {
	lang := LangFromContext(ctx)
	msg := tr.messages[lang][key]
	if msg == "" {
		msg = tr.messages[tr.defaultLang][key]
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
func (tr *Translator) DefaultLang() Lang {
	return tr.defaultLang
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
