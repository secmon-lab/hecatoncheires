// Package uierr carries user-facing error classification on a goerr error and
// renders it into the fixed 3-part Slack message every failure surfaces:
//
//	⚠️ {What}
//	*{Technical note}*: {Detail}[ (cause)]
//	*{What you can do}*: {Remediation}
//	ref: {id}
//
// It is a leaf: it depends only on i18n and goerr so both the Slack service
// (which attaches classification at the API-failure origin) and the usecase
// layer (which classifies by sentinel and renders) can import it without an
// import cycle. The classification cascade and reporting live in package
// usecase (pkg/usecase/uierr.go), which can see the domain sentinels.
package uierr

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
)

// Kind is the coarse severity/actionability class of a user-facing error. It
// drives the reporting policy (whether the error pages via Sentry): Permission
// and Validation are user/operator-actionable and treated as benign; Transient
// and Bug are not.
type Kind int

const (
	// KindPermission is an operator- or user-actionable access problem:
	// missing channel membership, missing scope, revoked token, workspace
	// misconfiguration, private-case access denial.
	KindPermission Kind = iota
	// KindValidation is a fixable input problem (missing/invalid case fields).
	KindValidation
	// KindTransient is a retryable infrastructure/LLM/rate-limit failure.
	KindTransient
	// KindBug is an unclassified internal error (a defect worth alerting on).
	KindBug
)

// UserFacing is the classification attached to an error and consumed by the
// renderer. What / Detail / Remediation are i18n keys with no printf args; the
// only variable part is Cause, appended in parentheses after Detail. Cause must
// be a safe, non-secret string (a Slack error code, comma-joined field names,
// or a truncated internal reason) — never a raw value dump.
type UserFacing struct {
	Kind        Kind
	What        i18n.MsgKey
	Detail      i18n.MsgKey
	Remediation i18n.MsgKey
	Cause       string
}

// Key carries a UserFacing through a goerr wrap chain. It is the project's
// first use of goerr's typed-value API; GetTypedValue reads it back with
// compile-time type safety and walks the whole chain.
var Key = goerr.NewTypedKey[UserFacing]("ui_error")

// Attach returns the goerr option that puts uf on an error at its origin, e.g.
// goerr.New("...", uierr.Attach(uf)).
func Attach(uf UserFacing) goerr.Option {
	return goerr.TV(Key, uf)
}

// SlackError maps a Slack API error code (SlackErrorResponse.Err) to a
// UserFacing, with Cause preset to the code. ok is false only for an empty
// code; any non-empty unrecognized code maps to a generic Slack failure so a
// Slack error is never left unclassified. The Slack-failure origin attaches
// the result at the chat.postMessage wrap site; keeping the code→class table
// here (plain strings, no slack-go dependency) lets it be shared and tested
// without importing the Slack SDK.
func SlackError(code string) (UserFacing, bool) {
	switch code {
	case "":
		return UserFacing{}, false
	case "not_in_channel", "channel_not_found", "is_archived":
		return UserFacing{
			Kind:        KindPermission,
			What:        i18n.MsgUIErrNotInChannelWhat,
			Detail:      i18n.MsgUIErrNotInChannelDetail,
			Remediation: i18n.MsgUIErrNotInChannelFix,
			Cause:       code,
		}, true
	case "missing_scope", "not_allowed_token_type":
		return UserFacing{
			Kind:        KindPermission,
			What:        i18n.MsgUIErrMissingScopeWhat,
			Detail:      i18n.MsgUIErrMissingScopeDetail,
			Remediation: i18n.MsgUIErrMissingScopeFix,
			Cause:       code,
		}, true
	case "invalid_auth", "account_inactive", "token_revoked", "token_expired", "not_authed":
		return UserFacing{
			Kind:        KindPermission,
			What:        i18n.MsgUIErrSlackAuthWhat,
			Detail:      i18n.MsgUIErrSlackAuthDetail,
			Remediation: i18n.MsgUIErrSlackAuthFix,
			Cause:       code,
		}, true
	case "ratelimited", "rate_limited":
		return UserFacing{
			Kind:        KindTransient,
			What:        i18n.MsgUIErrRateLimitedWhat,
			Detail:      i18n.MsgUIErrRateLimitedDetail,
			Remediation: i18n.MsgUIErrRateLimitedFix,
			Cause:       code,
		}, true
	default:
		return UserFacing{
			Kind:        KindTransient,
			What:        i18n.MsgUIErrSlackGenericWhat,
			Detail:      i18n.MsgUIErrSlackGenericDetail,
			Remediation: i18n.MsgUIErrSlackGenericFix,
			Cause:       code,
		}, true
	}
}

// Render builds the 3-part Slack mrkdwn text for uf. ref is the correlation id
// shown to the user and logged alongside the error; an empty ref omits the
// line. The What message already carries its own ⚠️ prefix.
func Render(ctx context.Context, uf UserFacing, ref string) string {
	var b strings.Builder
	b.WriteString(i18n.T(ctx, uf.What))

	b.WriteString("\n*")
	b.WriteString(i18n.T(ctx, i18n.MsgUIErrLabelDetail))
	b.WriteString("*: ")
	b.WriteString(i18n.T(ctx, uf.Detail))
	if uf.Cause != "" {
		b.WriteString(" (")
		b.WriteString(truncateCause(uf.Cause))
		b.WriteString(")")
	}

	b.WriteString("\n*")
	b.WriteString(i18n.T(ctx, i18n.MsgUIErrLabelFix))
	b.WriteString("*: ")
	b.WriteString(i18n.T(ctx, uf.Remediation))

	if ref != "" {
		b.WriteString("\nref: `")
		b.WriteString(ref)
		b.WriteString("`")
	}
	return b.String()
}

// maxCauseLen bounds the inline technical cause shown to the user. The cause
// is a short diagnostic hint (a Slack error code, a few field names, a
// truncated internal reason), not a full log line, so a tight rune-safe limit
// keeps the message readable and well under Slack's message size.
const maxCauseLen = 300

// truncateCause rune-safely clips cause to maxCauseLen, appending an ellipsis
// when it was cut.
func truncateCause(cause string) string {
	if len(cause) <= maxCauseLen {
		return cause
	}
	runes := []rune(cause)
	if len(runes) <= maxCauseLen {
		return cause
	}
	return string(runes[:maxCauseLen]) + "…"
}

// NewRef returns an 8-hex-character correlation id. It is not required to be
// globally unique — it only needs to let an operator tie a Slack error message
// to its log/Sentry entry within a bounded time window.
func NewRef() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand failing is near-impossible; keep the message renderable
		// rather than propagating an error from a display-only helper.
		return "00000000"
	}
	return hex.EncodeToString(buf[:])
}
