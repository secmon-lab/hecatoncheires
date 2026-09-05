package usecase_test

import (
	"context"
	"sync"
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// reactionCall is one reaction the usecase asked Slack for. Direction is part
// of the record: the whole point of the feature is that a reaction comes off
// again when the state it announced goes away.
type reactionCall struct {
	op      string // "add" or "remove"
	channel string
	ts      string
	emoji   string
}

type reactionFake struct {
	mockSlackService
	mu    sync.Mutex
	calls []reactionCall
}

func (f *reactionFake) AddReaction(_ context.Context, channelID, ts, emoji string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, reactionCall{op: "add", channel: channelID, ts: ts, emoji: emoji})
	return nil
}

func (f *reactionFake) RemoveReaction(_ context.Context, channelID, ts, emoji string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, reactionCall{op: "remove", channel: channelID, ts: ts, emoji: emoji})
	return nil
}

func (f *reactionFake) snapshot() []reactionCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]reactionCall(nil), f.calls...)
}

const (
	reactionWSID     = "support"
	reactionChannel  = "C-MONITOR"
	reactionThreadTS = "1783560454.200959"
	assignedEmoji    = "eyes"
	closedEmoji      = "white_check_mark"
)

// reactionEnv builds a thread-mode workspace carrying the two status
// reactions, and returns a usecase wired to a recording Slack fake.
func reactionEnv(t *testing.T, assigned, closed string) (*usecase.CaseUseCase, *reactionFake, *memory.Memory) {
	t.Helper()

	set, err := model.NewActionStatusSet("triage", []string{"done"}, []model.ActionStatusDefinition{
		{ID: "triage", Name: "Triage"},
		{ID: "done", Name: "Done"},
	})
	gt.NoError(t, err).Required()

	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace:             model.Workspace{ID: reactionWSID},
		CaseMode:              model.CaseModeThread,
		SlackMonitorChannelID: reactionChannel,
		CaseStatusSet:         set,
		AssignedReactionEmoji: assigned,
		ClosedReactionEmoji:   closed,
	})

	repo := memory.New()
	fake := &reactionFake{}
	return usecase.NewCaseUseCase(repo, registry, fake, nil, ""), fake, repo
}

func seedThreadCase(t *testing.T, repo *memory.Memory, ctx context.Context, assignees ...string) *model.Case {
	t.Helper()
	c, err := repo.Case().Create(ctx, reactionWSID, &model.Case{
		Title:          "Reaction case",
		Status:         types.CaseStatusOpen,
		AssigneeIDs:    assignees,
		SlackChannelID: reactionChannel,
		SlackThreadTS:  reactionThreadTS,
		BoardStatus:    "triage",
	})
	gt.NoError(t, err).Required()
	return c
}

func TestCaseUseCase_StatusReactions(t *testing.T) {
	t.Run("closing adds the closed reaction to the case thread", func(t *testing.T) {
		uc, fake, repo := reactionEnv(t, assignedEmoji, closedEmoji)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UACTOR"})
		c := seedThreadCase(t, repo, ctx)

		_, err := uc.UpdateCaseStatus(ctx, reactionWSID, c.ID, "done")
		gt.NoError(t, err).Required()

		calls := fake.snapshot()
		gt.Array(t, calls).Length(1).Required()
		gt.Value(t, calls[0]).Equal(reactionCall{
			op: "add", channel: reactionChannel, ts: reactionThreadTS, emoji: closedEmoji,
		})
	})

	t.Run("reopening takes the closed reaction off again", func(t *testing.T) {
		uc, fake, repo := reactionEnv(t, assignedEmoji, closedEmoji)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UACTOR"})
		c := seedThreadCase(t, repo, ctx)

		_, err := uc.UpdateCaseStatus(ctx, reactionWSID, c.ID, "done")
		gt.NoError(t, err).Required()
		_, err = uc.UpdateCaseStatus(ctx, reactionWSID, c.ID, "triage")
		gt.NoError(t, err).Required()

		calls := fake.snapshot()
		gt.Array(t, calls).Length(2).Required()
		gt.Value(t, calls[1].op).Equal("remove")
		gt.Value(t, calls[1].emoji).Equal(closedEmoji)
	})

	t.Run("moving between two open statuses reacts neither way", func(t *testing.T) {
		set, err := model.NewActionStatusSet("triage", []string{"done"}, []model.ActionStatusDefinition{
			{ID: "triage", Name: "Triage"},
			{ID: "working", Name: "Working"},
			{ID: "done", Name: "Done"},
		})
		gt.NoError(t, err).Required()

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:             model.Workspace{ID: reactionWSID},
			CaseMode:              model.CaseModeThread,
			SlackMonitorChannelID: reactionChannel,
			CaseStatusSet:         set,
			ClosedReactionEmoji:   closedEmoji,
		})
		repo := memory.New()
		fake := &reactionFake{}
		uc := usecase.NewCaseUseCase(repo, registry, fake, nil, "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UACTOR"})
		c := seedThreadCase(t, repo, ctx)

		_, err = uc.UpdateCaseStatus(ctx, reactionWSID, c.ID, "working")
		gt.NoError(t, err).Required()

		gt.Array(t, fake.snapshot()).Length(0)
	})

	t.Run("a workspace that configures no reaction calls Slack for none", func(t *testing.T) {
		uc, fake, repo := reactionEnv(t, "", "")
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UACTOR"})
		c := seedThreadCase(t, repo, ctx)

		_, err := uc.UpdateCaseStatus(ctx, reactionWSID, c.ID, "done")
		gt.NoError(t, err).Required()

		gt.Array(t, fake.snapshot()).Length(0)
	})
}

func TestCaseUseCase_AssignedReaction(t *testing.T) {
	t.Run("the first assignee adds the assigned reaction", func(t *testing.T) {
		uc, fake, repo := reactionEnv(t, assignedEmoji, closedEmoji)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UACTOR"})
		seedSlackUsers(t, repo, "UACTOR", "U001")
		c := seedThreadCase(t, repo, ctx)

		_, err := uc.AssignCase(ctx, reactionWSID, c.ID, []string{"U001"})
		gt.NoError(t, err).Required()

		calls := fake.snapshot()
		gt.Array(t, calls).Length(1).Required()
		gt.Value(t, calls[0]).Equal(reactionCall{
			op: "add", channel: reactionChannel, ts: reactionThreadTS, emoji: assignedEmoji,
		})
	})

	t.Run("losing the last assignee removes it", func(t *testing.T) {
		uc, fake, repo := reactionEnv(t, assignedEmoji, closedEmoji)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UACTOR"})
		seedSlackUsers(t, repo, "UACTOR", "U001")
		c := seedThreadCase(t, repo, ctx, "U001")

		_, err := uc.UnassignCase(ctx, reactionWSID, c.ID, []string{"U001"})
		gt.NoError(t, err).Required()

		calls := fake.snapshot()
		gt.Array(t, calls).Length(1).Required()
		gt.Value(t, calls[0].op).Equal("remove")
		gt.Value(t, calls[0].emoji).Equal(assignedEmoji)
	})

	t.Run("losing one of two assignees keeps it", func(t *testing.T) {
		// "Somebody has this" is still true, so the mark stays put.
		uc, fake, repo := reactionEnv(t, assignedEmoji, closedEmoji)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UACTOR"})
		seedSlackUsers(t, repo, "UACTOR", "U001", "U002")
		c := seedThreadCase(t, repo, ctx, "U001", "U002")

		_, err := uc.UnassignCase(ctx, reactionWSID, c.ID, []string{"U001"})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.snapshot()).Length(0)
	})
}
