package usecase_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

const dashTestUser = "U-me"

func dashTestRegistry(wsIDs ...string) *model.WorkspaceRegistry {
	reg := model.NewWorkspaceRegistry()
	for _, id := range wsIDs {
		reg.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: id, Name: "name-" + id},
		})
	}
	return reg
}

func dashCtx(userID string) context.Context {
	return auth.ContextWithToken(context.Background(), &auth.Token{Sub: userID})
}

func TestDashboardUseCase_ListMyOpenCases(t *testing.T) {
	t.Parallel()
	repo := memory.New()
	reg := dashTestRegistry("ws-1", "ws-2")
	uc := usecase.New(repo, reg, usecase.WithDashboardStaleThreshold(24*time.Hour))
	ctx := dashCtx(dashTestUser)
	now := time.Now().UTC()

	// ws-1: assigned + fresh (should appear, not stalled)
	fresh, err := repo.Case().Create(ctx, "ws-1", &model.Case{
		Title: "fresh assigned", Status: types.CaseStatusOpen, ReporterID: "U-rep",
		AssigneeIDs: []string{dashTestUser}, CreatedAt: now, UpdatedAt: now,
	})
	gt.NoError(t, err).Required()

	// ws-2: assigned + stale (should appear, stalled=true, sorted first)
	stale, err := repo.Case().Create(ctx, "ws-2", &model.Case{
		Title: "stale assigned", Status: types.CaseStatusOpen, ReporterID: "U-rep",
		AssigneeIDs: []string{dashTestUser}, CreatedAt: now, UpdatedAt: now.Add(-48 * time.Hour),
	})
	gt.NoError(t, err).Required()

	// ws-1: not assigned to me (should be excluded)
	_, err = repo.Case().Create(ctx, "ws-1", &model.Case{
		Title: "someone else", Status: types.CaseStatusOpen, ReporterID: "U-rep",
		AssigneeIDs: []string{"U-other"}, CreatedAt: now, UpdatedAt: now,
	})
	gt.NoError(t, err).Required()

	// ws-1: private, assigned to me, but I'm not a channel member (excluded)
	_, err = repo.Case().Create(ctx, "ws-1", &model.Case{
		Title: "private no access", Status: types.CaseStatusOpen, ReporterID: "U-rep",
		AssigneeIDs: []string{dashTestUser}, IsPrivate: true, ChannelUserIDs: []string{"U-other"},
		CreatedAt: now, UpdatedAt: now,
	})
	gt.NoError(t, err).Required()

	got, err := uc.Dashboard.ListMyOpenCases(ctx)
	gt.NoError(t, err).Required()
	gt.Array(t, got).Length(2).Required()
	// Stalled first.
	gt.Value(t, got[0].Case.ID).Equal(stale.ID)
	gt.Bool(t, got[0].Stalled).True()
	gt.Value(t, got[0].WorkspaceID).Equal("ws-2")
	gt.Value(t, got[0].WorkspaceName).Equal("name-ws-2")
	gt.Value(t, got[1].Case.ID).Equal(fresh.ID)
	gt.Bool(t, got[1].Stalled).False()
}

func TestDashboardUseCase_ListMyOpenCases_Unauthenticated(t *testing.T) {
	t.Parallel()
	repo := memory.New()
	uc := usecase.New(repo, dashTestRegistry("ws-1"))

	_, err := uc.Dashboard.ListMyOpenCases(context.Background())
	gt.Error(t, err).Is(usecase.ErrUnauthenticated)
}

func TestDashboardUseCase_ListMyDueActions(t *testing.T) {
	t.Parallel()
	repo := memory.New()
	reg := dashTestRegistry("ws-1", "ws-2")
	uc := usecase.New(repo, reg)
	ctx := dashCtx(dashTestUser)
	now := time.Now().UTC()

	mkCase := func(wsID string) int64 {
		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title: "case", Status: types.CaseStatusOpen, ReporterID: "U-rep",
			AssigneeIDs: []string{dashTestUser}, CreatedAt: now, UpdatedAt: now,
		})
		gt.NoError(t, err).Required()
		return c.ID
	}
	c1 := mkCase("ws-1")
	c2 := mkCase("ws-2")

	overdue := now.Add(-24 * time.Hour)
	future := now.Add(72 * time.Hour)

	// ws-2 overdue, assigned to me, not closed -> should be first
	_, err := repo.Action().Create(ctx, "ws-2", &model.Action{
		CaseID: c2, Title: "overdue", AssigneeID: dashTestUser,
		Status: types.ActionStatus("BACKLOG"), DueDate: &overdue, CreatedAt: now, UpdatedAt: now,
	})
	gt.NoError(t, err).Required()

	// ws-1 future, assigned to me, not closed -> second
	_, err = repo.Action().Create(ctx, "ws-1", &model.Action{
		CaseID: c1, Title: "future", AssigneeID: dashTestUser,
		Status: types.ActionStatus("BACKLOG"), DueDate: &future, CreatedAt: now, UpdatedAt: now,
	})
	gt.NoError(t, err).Required()

	// ws-1 no due date, assigned to me -> last
	_, err = repo.Action().Create(ctx, "ws-1", &model.Action{
		CaseID: c1, Title: "nodue", AssigneeID: dashTestUser,
		Status: types.ActionStatus("BACKLOG"), CreatedAt: now, UpdatedAt: now,
	})
	gt.NoError(t, err).Required()

	// closed action assigned to me -> excluded
	_, err = repo.Action().Create(ctx, "ws-1", &model.Action{
		CaseID: c1, Title: "closed", AssigneeID: dashTestUser,
		Status: types.ActionStatus("COMPLETED"), DueDate: &overdue, CreatedAt: now, UpdatedAt: now,
	})
	gt.NoError(t, err).Required()

	// assigned to someone else -> excluded
	_, err = repo.Action().Create(ctx, "ws-1", &model.Action{
		CaseID: c1, Title: "other", AssigneeID: "U-other",
		Status: types.ActionStatus("BACKLOG"), DueDate: &overdue, CreatedAt: now, UpdatedAt: now,
	})
	gt.NoError(t, err).Required()

	got, err := uc.Dashboard.ListMyDueActions(ctx)
	gt.NoError(t, err).Required()
	gt.Array(t, got).Length(3).Required()
	gt.String(t, got[0].Action.Title).Equal("overdue")
	gt.Value(t, got[0].WorkspaceID).Equal("ws-2")
	gt.Value(t, got[0].CaseID).Equal(c2)
	gt.String(t, got[1].Action.Title).Equal("future")
	gt.String(t, got[2].Action.Title).Equal("nodue")
}

func TestDashboardUseCase_ListMyDueActions_PrivateCaseAccessControl(t *testing.T) {
	t.Parallel()
	ctx := dashCtx(dashTestUser)
	now := time.Now().UTC()
	due := now.Add(-time.Hour)

	// Build a private case whose action is assigned to the caller. The caller's
	// membership is what decides visibility.
	setup := func(channelUsers []string) []*model.MyDueAction {
		repo := memory.New()
		uc := usecase.New(repo, dashTestRegistry("ws-1"))
		c, err := repo.Case().Create(ctx, "ws-1", &model.Case{
			Title: "private case", Status: types.CaseStatusOpen, ReporterID: "U-rep",
			AssigneeIDs: []string{dashTestUser}, IsPrivate: true, ChannelUserIDs: channelUsers,
			CreatedAt: now, UpdatedAt: now,
		})
		gt.NoError(t, err).Required()
		_, err = repo.Action().Create(ctx, "ws-1", &model.Action{
			CaseID: c.ID, Title: "secret action", AssigneeID: dashTestUser,
			Status: types.ActionStatus("BACKLOG"), DueDate: &due, CreatedAt: now, UpdatedAt: now,
		})
		gt.NoError(t, err).Required()
		got, err := uc.Dashboard.ListMyDueActions(ctx)
		gt.NoError(t, err).Required()
		return got
	}

	// Member of the private channel: the action is visible.
	member := setup([]string{dashTestUser})
	gt.Array(t, member).Length(1).Required()
	gt.String(t, member[0].Action.Title).Equal("secret action")

	// Non-member: the action is excluded, even though it is assigned to them.
	nonMember := setup([]string{"U-other"})
	gt.Array(t, nonMember).Length(0)
}

func TestDashboardUseCase_Favorites(t *testing.T) {
	t.Parallel()
	repo := memory.New()
	reg := dashTestRegistry("ws-1", "ws-2", "ws-3")
	uc := usecase.New(repo, reg)
	ctx := dashCtx(dashTestUser)

	// Empty when unset.
	got, err := uc.Dashboard.GetFavoriteWorkspaces(ctx)
	gt.NoError(t, err).Required()
	gt.Array(t, got).Length(0)

	// Unknown workspace is rejected.
	_, err = uc.Dashboard.SetFavoriteWorkspaces(ctx, []string{"ws-1", "ws-nope"})
	gt.Error(t, err).Is(model.ErrUserPreferenceValidation)

	// Set with a duplicate; dedup preserves order.
	set, err := uc.Dashboard.SetFavoriteWorkspaces(ctx, []string{"ws-2", "ws-1", "ws-2"})
	gt.NoError(t, err).Required()
	gt.Array(t, set).Length(2).Required()
	gt.Value(t, set[0]).Equal("ws-2")
	gt.Value(t, set[1]).Equal("ws-1")

	// Get returns the stored list.
	got, err = uc.Dashboard.GetFavoriteWorkspaces(ctx)
	gt.NoError(t, err).Required()
	gt.Array(t, got).Length(2).Required()
	gt.Value(t, got[0]).Equal("ws-2")
	gt.Value(t, got[1]).Equal("ws-1")
}

func TestDashboardUseCase_GetFavorites_FiltersRemovedWorkspace(t *testing.T) {
	t.Parallel()
	repo := memory.New()
	ctx := dashCtx(dashTestUser)

	// Store a favorite pointing at ws-gone directly in the repo, then build a
	// usecase whose registry no longer knows ws-gone.
	now := time.Now().UTC()
	gt.NoError(t, repo.UserPreference().Set(ctx, &model.UserPreference{
		UserID: dashTestUser, FavoriteWorkspaceIDs: []string{"ws-live", "ws-gone"},
		CreatedAt: now, UpdatedAt: now,
	})).Required()

	uc := usecase.New(repo, dashTestRegistry("ws-live"))
	got, err := uc.Dashboard.GetFavoriteWorkspaces(ctx)
	gt.NoError(t, err).Required()
	gt.Array(t, got).Length(1).Required()
	gt.Value(t, got[0]).Equal("ws-live")
}

func mockGreetingLLM(t *testing.T, json string, calls *int32) gollem.LLMClient {
	t.Helper()
	return &mock.LLMClientMock{
		NewSessionFunc: func(ctx context.Context, options ...gollem.SessionOption) (gollem.Session, error) {
			atomic.AddInt32(calls, 1)
			return &mock.SessionMock{
				GenerateFunc: func(ctx context.Context, input []gollem.Input, opts ...gollem.GenerateOption) (*gollem.Response, error) {
					return &gollem.Response{Texts: []string{json}}, nil
				},
			}, nil
		},
	}
}

func TestDashboardUseCase_GenerateHomeMessage_NoLLM(t *testing.T) {
	t.Parallel()
	repo := memory.New()
	uc := usecase.New(repo, dashTestRegistry("ws-1"))
	ctx := dashCtx(dashTestUser)

	msg, err := uc.Dashboard.GenerateHomeMessage(ctx, time.Now(), "en")
	gt.NoError(t, err).Required()
	gt.String(t, msg).Equal("")
}

func TestDashboardUseCase_GenerateHomeMessage_GeneratesAndCaches(t *testing.T) {
	t.Parallel()
	repo := memory.New()
	var calls int32
	llm := mockGreetingLLM(t, `{"message":"good morning"}`, &calls)
	uc := usecase.New(repo, dashTestRegistry("ws-1"), usecase.WithHomeMessageLLMClient(llm))
	ctx := dashCtx(dashTestUser)

	msg, err := uc.Dashboard.GenerateHomeMessage(ctx, time.Now(), "en")
	gt.NoError(t, err).Required()
	gt.String(t, msg).Equal("good morning")
	gt.Number(t, atomic.LoadInt32(&calls)).Equal(int32(1))

	// It was appended to history.
	recent, err := repo.HomeMessage().ListRecent(ctx, dashTestUser, 5)
	gt.NoError(t, err).Required()
	gt.Array(t, recent).Length(1).Required()
	gt.String(t, recent[0].Message).Equal("good morning")
	gt.String(t, recent[0].Lang).Equal("en")
}

func TestDashboardUseCase_GenerateHomeMessage_ReusesFresh(t *testing.T) {
	t.Parallel()
	repo := memory.New()
	ctx := dashCtx(dashTestUser)

	// Seed a fresh message; the LLM must NOT be called.
	gt.NoError(t, repo.HomeMessage().Add(ctx, &model.HomeMessage{
		ID: model.NewHomeMessageID(), UserID: dashTestUser, Message: "cached line",
		Lang: "en", CreatedAt: time.Now(),
	})).Required()

	var calls int32
	llm := mockGreetingLLM(t, `{"message":"should not be used"}`, &calls)
	uc := usecase.New(repo, dashTestRegistry("ws-1"), usecase.WithHomeMessageLLMClient(llm))

	msg, err := uc.Dashboard.GenerateHomeMessage(ctx, time.Now(), "en")
	gt.NoError(t, err).Required()
	gt.String(t, msg).Equal("cached line")
	// The LLM must not be invoked when a fresh message exists.
	gt.Number(t, atomic.LoadInt32(&calls)).Equal(int32(0))
}

func TestDashboardUseCase_GenerateHomeMessage_RegeneratesOnStale(t *testing.T) {
	t.Parallel()
	repo := memory.New()
	ctx := dashCtx(dashTestUser)

	// Stale (2h old) message -> must regenerate.
	gt.NoError(t, repo.HomeMessage().Add(ctx, &model.HomeMessage{
		ID: model.NewHomeMessageID(), UserID: dashTestUser, Message: "old line",
		Lang: "en", CreatedAt: time.Now().Add(-2 * time.Hour),
	})).Required()

	var calls int32
	llm := mockGreetingLLM(t, `{"message":"new line"}`, &calls)
	uc := usecase.New(repo, dashTestRegistry("ws-1"), usecase.WithHomeMessageLLMClient(llm))

	msg, err := uc.Dashboard.GenerateHomeMessage(ctx, time.Now(), "en")
	gt.NoError(t, err).Required()
	gt.String(t, msg).Equal("new line")
	gt.Number(t, atomic.LoadInt32(&calls)).Equal(int32(1))
}

func TestDashboardUseCase_GenerateHomeMessage_RegeneratesOnLangMismatch(t *testing.T) {
	t.Parallel()
	repo := memory.New()
	ctx := dashCtx(dashTestUser)

	// A FRESH English message exists, but the caller asks for Japanese, so the
	// English cache must not be reused.
	gt.NoError(t, repo.HomeMessage().Add(ctx, &model.HomeMessage{
		ID: model.NewHomeMessageID(), UserID: dashTestUser, Message: "english line",
		Lang: "en", CreatedAt: time.Now(),
	})).Required()

	var calls int32
	llm := mockGreetingLLM(t, `{"message":"japanese line"}`, &calls)
	uc := usecase.New(repo, dashTestRegistry("ws-1"), usecase.WithHomeMessageLLMClient(llm))

	msg, err := uc.Dashboard.GenerateHomeMessage(ctx, time.Now(), "ja")
	gt.NoError(t, err).Required()
	gt.String(t, msg).Equal("japanese line")
	gt.Number(t, atomic.LoadInt32(&calls)).Equal(int32(1))
}

func TestDashboardUseCase_GenerateHomeMessage_UnknownLangReusesDefaultBucket(t *testing.T) {
	t.Parallel()
	repo := memory.New()
	ctx := dashCtx(dashTestUser)

	// A fresh message stored under the default bucket ("en"). An unknown lang
	// normalizes to "en", so it must reuse the cache (no LLM call) — this closes
	// the cache-bypass via arbitrary lang strings.
	gt.NoError(t, repo.HomeMessage().Add(ctx, &model.HomeMessage{
		ID: model.NewHomeMessageID(), UserID: dashTestUser, Message: "english line",
		Lang: "en", CreatedAt: time.Now(),
	})).Required()

	var calls int32
	llm := mockGreetingLLM(t, `{"message":"unused"}`, &calls)
	uc := usecase.New(repo, dashTestRegistry("ws-1"), usecase.WithHomeMessageLLMClient(llm))

	msg, err := uc.Dashboard.GenerateHomeMessage(ctx, time.Now(), "zz-random")
	gt.NoError(t, err).Required()
	gt.String(t, msg).Equal("english line")
	gt.Number(t, atomic.LoadInt32(&calls)).Equal(int32(0))
}

func TestDashboardUseCase_GenerateHomeMessage_LLMErrorPropagates(t *testing.T) {
	t.Parallel()
	repo := memory.New()
	llm := &mock.LLMClientMock{
		NewSessionFunc: func(ctx context.Context, options ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(ctx context.Context, input []gollem.Input, opts ...gollem.GenerateOption) (*gollem.Response, error) {
					return nil, goerr.New("llm down")
				},
			}, nil
		},
	}
	uc := usecase.New(repo, dashTestRegistry("ws-1"), usecase.WithHomeMessageLLMClient(llm))
	ctx := dashCtx(dashTestUser)

	_, err := uc.Dashboard.GenerateHomeMessage(ctx, time.Now(), "en")
	gt.Error(t, err)
}

func TestDashboardUseCase_GenerateHomeMessage_AddFailureStillReturnsMessage(t *testing.T) {
	t.Parallel()
	base := memory.New()
	repo := addFailHomeMessageRepo{Repository: base, hm: addFailHomeMessage{HomeMessageRepository: base.HomeMessage()}}
	var calls int32
	llm := mockGreetingLLM(t, `{"message":"generated despite add failure"}`, &calls)
	uc := usecase.New(repo, dashTestRegistry("ws-1"), usecase.WithHomeMessageLLMClient(llm))
	ctx := dashCtx(dashTestUser)

	// Add fails, but the freshly generated message must still be returned
	// (non-fatal cache write).
	msg, err := uc.Dashboard.GenerateHomeMessage(ctx, time.Now(), "en")
	gt.NoError(t, err).Required()
	gt.String(t, msg).Equal("generated despite add failure")
	gt.Number(t, atomic.LoadInt32(&calls)).Equal(int32(1))
}

func TestDashboardUseCase_Unauthenticated(t *testing.T) {
	t.Parallel()
	repo := memory.New()
	uc := usecase.New(repo, dashTestRegistry("ws-1"), usecase.WithHomeMessageLLMClient(mockGreetingLLM(t, `{"message":"x"}`, new(int32))))
	ctx := context.Background()

	_, err := uc.Dashboard.ListMyDueActions(ctx)
	gt.Error(t, err).Is(usecase.ErrUnauthenticated)
	_, err = uc.Dashboard.GetFavoriteWorkspaces(ctx)
	gt.Error(t, err).Is(usecase.ErrUnauthenticated)
	_, err = uc.Dashboard.SetFavoriteWorkspaces(ctx, []string{"ws-1"})
	gt.Error(t, err).Is(usecase.ErrUnauthenticated)
	_, err = uc.Dashboard.GenerateHomeMessage(ctx, time.Now(), "en")
	gt.Error(t, err).Is(usecase.ErrUnauthenticated)
}

// addFailHomeMessageRepo overrides HomeMessage() on an embedded Repository so a
// test can make Add fail while every other repository behaves normally.
type addFailHomeMessageRepo struct {
	interfaces.Repository
	hm interfaces.HomeMessageRepository
}

func (r addFailHomeMessageRepo) HomeMessage() interfaces.HomeMessageRepository { return r.hm }

type addFailHomeMessage struct {
	interfaces.HomeMessageRepository
}

func (addFailHomeMessage) Add(ctx context.Context, msg *model.HomeMessage) error {
	return goerr.New("add failed")
}

func TestRenderHomeMessagePrompt(t *testing.T) {
	t.Parallel()

	t.Run("happy path exercises every template action", func(t *testing.T) {
		got, err := usecase.RenderHomeMessagePromptForTest(usecase.HomeMessagePromptInputForTest{
			Lang:           "ja",
			Date:           "2026-07-17",
			TimeOfDay:      "morning",
			OpenCaseLoad:   "several",
			DueActionLoad:  "a few",
			WorkspaceNames: []string{"Risk", "Legal"},
			Flavor:         "calm and grounding",
			Nonce:          42,
			RecentMessages: []string{"welcome back", "steady as you go"},
		})
		gt.NoError(t, err).Required()
		gt.String(t, got).Contains("Output language: ja")
		gt.String(t, got).Contains("Time of day: morning")
		gt.String(t, got).Contains("Open cases assigned to the user: several")
		gt.String(t, got).Contains("Workspaces the user is involved in: Risk, Legal")
		gt.String(t, got).Contains("Tone for this message: calm and grounding (nonce 42)")
		gt.String(t, got).Contains("do NOT repeat")
		gt.String(t, got).Contains("- welcome back")
		gt.String(t, got).Contains("- steady as you go")
	})

	t.Run("empty workspaces and no history stays well-formed", func(t *testing.T) {
		got, err := usecase.RenderHomeMessagePromptForTest(usecase.HomeMessagePromptInputForTest{
			Lang:          "en",
			Date:          "2026-07-17",
			TimeOfDay:     "night",
			OpenCaseLoad:  "none",
			DueActionLoad: "none",
			Flavor:        "matter-of-fact and brief",
			Nonce:         7,
		})
		gt.NoError(t, err).Required()
		gt.String(t, got).Contains("The user is not currently involved in any active case.")
		gt.String(t, got).Contains("Tone for this message: matter-of-fact and brief (nonce 7)")
		// No history section when there are no recent messages.
		gt.Bool(t, strings.Contains(got, "do NOT repeat")).False()
	})
}

func TestNormalizeHomeMessageLang(t *testing.T) {
	t.Parallel()
	gt.String(t, usecase.NormalizeHomeMessageLangForTest("ja")).Equal("ja")
	gt.String(t, usecase.NormalizeHomeMessageLangForTest("en")).Equal("en")
	gt.String(t, usecase.NormalizeHomeMessageLangForTest("")).Equal("en")
	gt.String(t, usecase.NormalizeHomeMessageLangForTest("fr")).Equal("en")
}

func TestDashboardUseCase_GenerateHomeMessage_EmptyGenerationErrors(t *testing.T) {
	t.Parallel()
	repo := memory.New()
	var calls int32
	llm := mockGreetingLLM(t, `{"message":"   "}`, &calls)
	uc := usecase.New(repo, dashTestRegistry("ws-1"), usecase.WithHomeMessageLLMClient(llm))
	ctx := dashCtx(dashTestUser)

	_, err := uc.Dashboard.GenerateHomeMessage(ctx, time.Now(), "en")
	gt.Error(t, err).Is(model.ErrHomeMessageValidation)
}
