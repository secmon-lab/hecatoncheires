package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestAssertCaseWriteAccess(t *testing.T) {
	const (
		reporter = "U-reporter"
		member   = "U-member"
		outsider = "U-outsider"
	)

	// Non-draft private: access is gated purely by ChannelUserIDs (reporter has
	// no special standing once the channel exists).
	openPrivate := &model.Case{ID: 1, IsPrivate: true, Status: types.CaseStatusOpen, ReporterID: reporter, ChannelUserIDs: []string{member}}
	openPublic := &model.Case{ID: 2, IsPrivate: false, Status: types.CaseStatusOpen, ReporterID: reporter}
	// Drafts have no Slack channel, so ChannelUserIDs is empty; private drafts
	// fall back to the reporter.
	draftPrivate := &model.Case{ID: 3, IsPrivate: true, Status: types.CaseStatusDraft, ReporterID: reporter}
	draftPublic := &model.Case{ID: 4, IsPrivate: false, Status: types.CaseStatusDraft, ReporterID: reporter}

	testCases := map[string]struct {
		c           *model.Case
		actorID     string
		checkAccess bool
		wantDenied  bool
	}{
		"bypass when checkAccess is false":  {openPrivate, "", false, false},
		"non-draft private member allowed":  {openPrivate, member, true, false},
		"non-draft private outsider denied": {openPrivate, outsider, true, true},
		"non-draft public allowed":          {openPublic, outsider, true, false},
		"private draft reporter allowed":    {draftPrivate, reporter, true, false},
		"private draft non-reporter denied": {draftPrivate, outsider, true, true},
		"public draft allowed":              {draftPublic, outsider, true, false},
		"empty actor on private denied":     {openPrivate, "", true, true},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			err := usecase.AssertCaseWriteAccessForTest(tc.c, tc.actorID, tc.checkAccess)
			if tc.wantDenied {
				gt.Error(t, err).Is(usecase.ErrAccessDenied)
			} else {
				gt.NoError(t, err)
			}
		})
	}
}

func TestTokenActor(t *testing.T) {
	t.Run("no token bypasses", func(t *testing.T) {
		id, check := usecase.TokenActorForTest(context.Background())
		gt.String(t, id).Equal("")
		gt.Bool(t, check).False()
	})

	t.Run("token present", func(t *testing.T) {
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "U-1"})
		id, check := usecase.TokenActorForTest(ctx)
		gt.String(t, id).Equal("U-1")
		gt.Bool(t, check).True()
	})

	t.Run("empty sub still checks (malformed token denies, not bypasses)", func(t *testing.T) {
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: ""})
		id, check := usecase.TokenActorForTest(ctx)
		gt.String(t, id).Equal("")
		gt.Bool(t, check).True()
	})
}

func TestLoadCaseForWrite(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	now := time.Now().UTC()

	created, err := repo.Case().Create(ctx, testWorkspaceID, &model.Case{
		Title:          "private case",
		Status:         types.CaseStatusOpen,
		IsPrivate:      true,
		ReporterID:     "U-reporter",
		ChannelUserIDs: []string{"U-member"},
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	gt.NoError(t, err).Required()

	t.Run("case not found", func(t *testing.T) {
		_, err := usecase.LoadCaseForWriteForTest(ctx, repo, testWorkspaceID, 999999)
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})

	t.Run("member allowed and returns the case", func(t *testing.T) {
		memberCtx := auth.ContextWithToken(ctx, &auth.Token{Sub: "U-member"})
		got, err := usecase.LoadCaseForWriteForTest(memberCtx, repo, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.ID).Equal(created.ID)
		gt.String(t, got.Title).Equal("private case")
	})

	t.Run("outsider denied", func(t *testing.T) {
		outsiderCtx := auth.ContextWithToken(ctx, &auth.Token{Sub: "U-outsider"})
		_, err := usecase.LoadCaseForWriteForTest(outsiderCtx, repo, testWorkspaceID, created.ID)
		gt.Error(t, err).Is(usecase.ErrAccessDenied)
	})

	t.Run("no token bypasses (system/agent context)", func(t *testing.T) {
		_, err := usecase.LoadCaseForWriteForTest(ctx, repo, testWorkspaceID, created.ID)
		gt.NoError(t, err)
	})
}
