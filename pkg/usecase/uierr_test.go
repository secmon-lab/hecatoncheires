package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/uierr"
)

func TestClassifyUserError(t *testing.T) {
	t.Run("origin-attached typed value wins", func(t *testing.T) {
		want := uierr.UserFacing{
			Kind:        uierr.KindPermission,
			What:        i18n.MsgUIErrNotInChannelWhat,
			Detail:      i18n.MsgUIErrNotInChannelDetail,
			Remediation: i18n.MsgUIErrNotInChannelFix,
			Cause:       "not_in_channel",
		}
		// Wrapped further to prove the typed value is read through the chain.
		err := goerr.Wrap(goerr.New("post seed", uierr.Attach(want)), "outer")
		got, ok := usecase.ClassifyUserErrorForTest(err)
		gt.Bool(t, ok).True()
		gt.Value(t, got).Equal(want)
	})

	t.Run("access denied sentinel", func(t *testing.T) {
		err := goerr.Wrap(usecase.TestErrAccessDenied, "load case for write")
		got, ok := usecase.ClassifyUserErrorForTest(err)
		gt.Bool(t, ok).True()
		gt.Value(t, got.Kind).Equal(uierr.KindPermission)
		gt.Value(t, got.What).Equal(i18n.MsgUIErrAccessDeniedWhat)
	})

	t.Run("field validation carries the missing field names as cause", func(t *testing.T) {
		err := goerr.Wrap(model.ErrCaseFieldValidation, "thread case field validation failed",
			goerr.V(usecase.MissingFieldNamesKey, []string{"Priority", "Due date"}))
		got, ok := usecase.ClassifyUserErrorForTest(err)
		gt.Bool(t, ok).True()
		gt.Value(t, got.Kind).Equal(uierr.KindValidation)
		gt.Value(t, got.What).Equal(i18n.MsgUIErrFieldValidationWhat)
		gt.Value(t, got.Cause).Equal("Priority, Due date")
	})

	t.Run("no accessible workspace maps to config", func(t *testing.T) {
		err := goerr.Wrap(usecase.ErrNoAccessibleWorkspace, "resolve workspace")
		got, ok := usecase.ClassifyUserErrorForTest(err)
		gt.Bool(t, ok).True()
		gt.Value(t, got.Kind).Equal(uierr.KindPermission)
		gt.Value(t, got.What).Equal(i18n.MsgUIErrConfigWhat)
	})

	t.Run("unrecognized error is not classified", func(t *testing.T) {
		_, ok := usecase.ClassifyUserErrorForTest(errors.New("boom"))
		gt.Bool(t, ok).False()
	})
}

func TestUnexpectedUserFacing(t *testing.T) {
	uf := usecase.UnexpectedUserFacingForTest(goerr.New("kaboom deep inside"))
	gt.Value(t, uf.Kind).Equal(uierr.KindBug)
	gt.Value(t, uf.What).Equal(i18n.MsgUIErrUnexpectedWhat)
	gt.String(t, uf.Cause).Contains("kaboom deep inside")
}

func TestFallbackReasonError(t *testing.T) {
	err := usecase.FallbackReasonErrorForTest("planner budget exhausted; last failure: timeout")
	got, ok := usecase.ClassifyUserErrorForTest(err)
	gt.Bool(t, ok).True()
	gt.Value(t, got.Kind).Equal(uierr.KindTransient)
	gt.Value(t, got.What).Equal(i18n.MsgUIErrAgentNoConclusionWhat)
	gt.String(t, got.Cause).Contains("timeout")
}

func TestPrepareUserError(t *testing.T) {
	ctx := i18n.ContextWithLang(context.Background(), i18n.LangEN)

	t.Run("classified error renders its 3-part text with a matching ref", func(t *testing.T) {
		uf := uierr.UserFacing{
			Kind:        uierr.KindPermission,
			What:        i18n.MsgUIErrNotInChannelWhat,
			Detail:      i18n.MsgUIErrNotInChannelDetail,
			Remediation: i18n.MsgUIErrNotInChannelFix,
			Cause:       "not_in_channel",
		}
		err := goerr.New("post seed root", uierr.Attach(uf))
		text, ref := usecase.PrepareUserErrorForTest(ctx, err, "reaction cross-channel")

		gt.Number(t, len(ref)).Equal(8)
		gt.String(t, text).Contains(i18n.T(ctx, i18n.MsgUIErrNotInChannelWhat))
		gt.String(t, text).Contains(i18n.T(ctx, i18n.MsgUIErrNotInChannelFix))
		gt.String(t, text).Contains("(not_in_channel)")
		gt.String(t, text).Contains(ref)
	})

	t.Run("nil error yields empty text and ref (no spurious message)", func(t *testing.T) {
		text, ref := usecase.PrepareUserErrorForTest(ctx, nil, "op")
		gt.String(t, text).Equal("")
		gt.String(t, ref).Equal("")
	})

	t.Run("unclassified error still renders a 3-part message, not the generic text", func(t *testing.T) {
		text, ref := usecase.PrepareUserErrorForTest(ctx, errors.New("something broke"), "casebound run turn")
		gt.String(t, text).Contains(i18n.T(ctx, i18n.MsgUIErrUnexpectedWhat))
		gt.String(t, text).Contains(ref)
		// Regression guard: the old flattened generic message is gone.
		gt.Bool(t, strings.Contains(text, i18n.T(ctx, i18n.MsgAgentError))).False()
	})
}
