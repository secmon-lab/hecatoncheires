package uierr_test

import (
	"context"
	"strings"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/uierr"
)

func TestSlackError(t *testing.T) {
	cases := []struct {
		name     string
		code     string
		wantOK   bool
		wantKind uierr.Kind
		wantWhat i18n.MsgKey
	}{
		{"not_in_channel", "not_in_channel", true, uierr.KindPermission, i18n.MsgUIErrNotInChannelWhat},
		{"channel_not_found", "channel_not_found", true, uierr.KindPermission, i18n.MsgUIErrNotInChannelWhat},
		{"missing_scope", "missing_scope", true, uierr.KindPermission, i18n.MsgUIErrMissingScopeWhat},
		{"invalid_auth", "invalid_auth", true, uierr.KindPermission, i18n.MsgUIErrSlackAuthWhat},
		{"token_revoked", "token_revoked", true, uierr.KindPermission, i18n.MsgUIErrSlackAuthWhat},
		{"ratelimited", "ratelimited", true, uierr.KindTransient, i18n.MsgUIErrRateLimitedWhat},
		{"unknown code", "something_else", true, uierr.KindTransient, i18n.MsgUIErrSlackGenericWhat},
		{"empty", "", false, uierr.KindPermission, i18n.MsgUIErrNotInChannelWhat},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uf, ok := uierr.SlackError(tc.code)
			if !tc.wantOK {
				gt.Bool(t, ok).False()
				return
			}
			gt.Bool(t, ok).True()
			gt.Value(t, uf.Kind).Equal(tc.wantKind)
			gt.Value(t, uf.What).Equal(tc.wantWhat)
			gt.Value(t, uf.Cause).Equal(tc.code)
		})
	}
}

func TestRender(t *testing.T) {
	uf := uierr.UserFacing{
		Kind:        uierr.KindPermission,
		What:        i18n.MsgUIErrNotInChannelWhat,
		Detail:      i18n.MsgUIErrNotInChannelDetail,
		Remediation: i18n.MsgUIErrNotInChannelFix,
		Cause:       "not_in_channel",
	}

	t.Run("english 3-part frame with cause and ref", func(t *testing.T) {
		ctx := i18n.ContextWithLang(context.Background(), i18n.LangEN)
		out := uierr.Render(ctx, uf, "abc12345")

		// Every part is present, pulled from the same translation source.
		gt.String(t, out).Contains(i18n.T(ctx, i18n.MsgUIErrNotInChannelWhat))
		gt.String(t, out).Contains(i18n.T(ctx, i18n.MsgUIErrLabelDetail))
		gt.String(t, out).Contains(i18n.T(ctx, i18n.MsgUIErrNotInChannelDetail))
		gt.String(t, out).Contains(i18n.T(ctx, i18n.MsgUIErrLabelFix))
		gt.String(t, out).Contains(i18n.T(ctx, i18n.MsgUIErrNotInChannelFix))
		gt.String(t, out).Contains("(not_in_channel)")
		gt.String(t, out).Contains("ref: `abc12345`")
	})

	t.Run("japanese resolves from the ja table", func(t *testing.T) {
		ctx := i18n.ContextWithLang(context.Background(), i18n.LangJA)
		out := uierr.Render(ctx, uf, "abc12345")
		gt.String(t, out).Contains(i18n.T(ctx, i18n.MsgUIErrNotInChannelWhat))
		gt.String(t, out).Contains(i18n.T(ctx, i18n.MsgUIErrNotInChannelFix))
	})

	t.Run("empty cause omits the parenthetical", func(t *testing.T) {
		ctx := i18n.ContextWithLang(context.Background(), i18n.LangEN)
		noCause := uf
		noCause.Cause = ""
		out := uierr.Render(ctx, noCause, "ref00000")
		gt.String(t, out).Contains(i18n.T(ctx, i18n.MsgUIErrNotInChannelDetail))
		gt.Bool(t, strings.Contains(out, "()")).False()
	})

	t.Run("empty ref omits the ref line", func(t *testing.T) {
		ctx := i18n.ContextWithLang(context.Background(), i18n.LangEN)
		out := uierr.Render(ctx, uf, "")
		gt.Bool(t, strings.Contains(out, "ref:")).False()
	})

	t.Run("long cause is truncated with an ellipsis", func(t *testing.T) {
		ctx := i18n.ContextWithLang(context.Background(), i18n.LangEN)
		long := uf
		long.Cause = strings.Repeat("x", 500)
		out := uierr.Render(ctx, long, "ref00000")
		gt.String(t, out).Contains("…")
		// Clipped to exactly maxCauseLen (300) runes, no more.
		gt.Bool(t, strings.Contains(out, strings.Repeat("x", 300)+"…")).True()
		gt.Bool(t, strings.Contains(out, strings.Repeat("x", 301))).False()
	})
}

func TestNewRef(t *testing.T) {
	a := uierr.NewRef()
	b := uierr.NewRef()

	gt.Number(t, len(a)).Equal(8)
	gt.Value(t, a).NotEqual(b)
	for _, r := range a {
		isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
		gt.Bool(t, isHex).True()
	}
}
