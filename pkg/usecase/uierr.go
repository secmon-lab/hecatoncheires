package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/uierr"
)

// classifyUserError derives the user-facing classification for err. The order
// is deliberate and most-specific-first:
//
//  1. an origin-authored uierr.UserFacing carried as a goerr typed value
//     (the Slack API-failure path attaches this) — highest precision;
//  2. a domain sentinel matched via errors.Is (access, field validation,
//     workspace availability);
//
// It returns ok=false when nothing matches, so the caller falls back to
// unexpectedUserFacing. It never inspects error message strings.
func classifyUserError(err error) (uierr.UserFacing, bool) {
	if err == nil {
		return uierr.UserFacing{}, false
	}

	if uf, ok := goerr.GetTypedValue(err, uierr.Key); ok {
		return uf, true
	}

	switch {
	case errors.Is(err, ErrAccessDenied):
		return uierr.UserFacing{
			Kind:        uierr.KindPermission,
			What:        i18n.MsgUIErrAccessDeniedWhat,
			Detail:      i18n.MsgUIErrAccessDeniedDetail,
			Remediation: i18n.MsgUIErrAccessDeniedFix,
		}, true

	case errors.Is(err, ErrFieldValidationFailed),
		errors.Is(err, ErrMissingRequiredOnSubmit),
		errors.Is(err, ErrDraftTitleRequired),
		errors.Is(err, model.ErrCaseFieldValidation),
		errors.Is(err, model.ErrMissingRequired),
		errors.Is(err, model.ErrInvalidFieldType),
		errors.Is(err, model.ErrInvalidOptionID):
		return uierr.UserFacing{
			Kind:        uierr.KindValidation,
			What:        i18n.MsgUIErrFieldValidationWhat,
			Detail:      i18n.MsgUIErrFieldValidationDetail,
			Remediation: i18n.MsgUIErrFieldValidationFix,
			Cause:       missingFieldNames(err),
		}, true

	case errors.Is(err, ErrNoAccessibleWorkspace):
		return uierr.UserFacing{
			Kind:        uierr.KindPermission,
			What:        i18n.MsgUIErrConfigWhat,
			Detail:      i18n.MsgUIErrConfigDetail,
			Remediation: i18n.MsgUIErrConfigFix,
		}, true
	}

	return uierr.UserFacing{}, false
}

// missingFieldNames pulls the human-readable missing/invalid field names off a
// field-validation error's goerr values (set as MissingFieldNamesKey by the
// case write path). Returns "" when unavailable, so the rendered detail simply
// omits the parenthetical.
func missingFieldNames(err error) string {
	if v, ok := goerr.Values(err)[MissingFieldNamesKey]; ok {
		if names, ok := v.([]string); ok && len(names) > 0 {
			return strings.Join(names, ", ")
		}
	}
	return ""
}

// unexpectedUserFacing is the fallback for an error that no classifier branch
// recognized. It is still a full 3-part message (never the old flattened
// generic text): the technical note carries the error's own message chain
// (English developer text, kept secret-free by project policy; the renderer
// clips it), and the remediation tells the user to retry and quote the ref.
func unexpectedUserFacing(err error) uierr.UserFacing {
	cause := ""
	if err != nil {
		cause = err.Error()
	}
	return uierr.UserFacing{
		Kind:        uierr.KindBug,
		What:        i18n.MsgUIErrUnexpectedWhat,
		Detail:      i18n.MsgUIErrUnexpectedDetail,
		Remediation: i18n.MsgUIErrUnexpectedFix,
		Cause:       cause,
	}
}

// prepareUserError is the single entry point for turning any error into a
// user-facing Slack message. It classifies err, reports it exactly once
// (structured log + Sentry via errutil, with a shared ref_id), and returns the
// rendered 3-part text plus that ref. Callers post the text on whatever
// transport fits (thread reply, response_url) and MUST NOT also return/re-Handle
// err, which would double-report it.
//
// The benign decision is centralized here off the classification Kind:
// Permission/Validation are operator/user-actionable and tagged benign (logged,
// not paged); Transient/Bug page. An error already tagged benign deeper down
// stays benign regardless (goerr.With only adds).
func prepareUserError(ctx context.Context, err error, opMsg string) (text, ref string) {
	// Defensive: a nil err means there is nothing to surface. Without this a
	// caller that slipped a nil through would render and post a spurious
	// "unexpected error" message. Returning empty makes the poster a no-op.
	if err == nil {
		return "", ""
	}
	uf, ok := classifyUserError(err)
	if !ok {
		uf = unexpectedUserFacing(err)
	}

	ref = uierr.NewRef()

	reportErr := goerr.With(err, goerr.V("ref_id", ref))
	if uf.Kind == uierr.KindPermission || uf.Kind == uierr.KindValidation {
		reportErr = goerr.With(reportErr, goerr.T(errutil.TagBenign))
	}
	errutil.Handle(ctx, reportErr, opMsg)

	return uierr.Render(ctx, uf, ref), ref
}

// replyUserError classifies/reports err and posts the resulting 3-part message
// as a reply in the given thread. It is the thread-transport convenience over
// prepareUserError for the AgentUseCase error-post sites.
func (uc *AgentUseCase) replyUserError(ctx context.Context, err error, opMsg, channelID, threadTS string) {
	text, _ := prepareUserError(ctx, err, opMsg)
	if text == "" {
		// nil err (or otherwise nothing to say) — do not post an empty reply.
		return
	}
	uc.postThreadReply(ctx, channelID, threadTS, text)
}

// userErrorText is the trace-finalize variant of replyUserError: it
// classifies/reports err and returns the rendered text, for callers that post
// through an existing progress-trace message rather than a fresh reply.
func (uc *AgentUseCase) userErrorText(ctx context.Context, err error, opMsg string) string {
	text, _ := prepareUserError(ctx, err, opMsg)
	return text
}

// fallbackReasonError wraps a threadcase StatusFallback reason string into an
// error carrying the "couldn't finish this turn" classification, so the turn
// surfaces the real reason (budget exhaustion, a persistence failure) as the
// technical note instead of a generic message. The reason is a plain string
// because it already crossed the planexec boundary as one; the renderer clips
// it to a safe length.
func fallbackReasonError(reason string) error {
	return goerr.New("agent turn ended without a decision",
		uierr.Attach(uierr.UserFacing{
			Kind:        uierr.KindTransient,
			What:        i18n.MsgUIErrAgentNoConclusionWhat,
			Detail:      i18n.MsgUIErrAgentNoConclusionDetail,
			Remediation: i18n.MsgUIErrAgentNoConclusionFix,
			Cause:       reason,
		}))
}
