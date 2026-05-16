package usecase_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	goslack "github.com/slack-go/slack"
)

// slotSlackFake is a minimal slack.Service fake that records the calls the
// notification-slot coordinator makes. The remaining Service surface is
// satisfied by embedding mockSlackService (defined in source_test.go).
type slotSlackFake struct {
	mockSlackService

	mu              sync.Mutex
	postCalls       []slotPostCall
	updateCalls     []slotUpdateCall
	permalinkCalls  []slotPermalinkCall
	postReturnTS    string
	postReturnErr   error
	updateReturnErr error
	permalinkFn     func(channelID, messageTS string) (string, error)
}

type slotPostCall struct {
	ChannelID     string
	Text          string
	Blocks        []goslack.Block
	DisableUnfurl bool
}

type slotUpdateCall struct {
	ChannelID string
	Timestamp string
	Text      string
	Blocks    []goslack.Block
}

type slotPermalinkCall struct {
	ChannelID string
	MessageTS string
}

func (s *slotSlackFake) PostMessage(_ context.Context, channelID string, blocks []goslack.Block, text string, opts ...slacksvc.PostMessageOption) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := slacksvc.ApplyPostMessageOptions(opts...)
	s.postCalls = append(s.postCalls, slotPostCall{
		ChannelID:     channelID,
		Text:          text,
		Blocks:        blocks,
		DisableUnfurl: cfg.DisableLinkUnfurl && cfg.DisableMediaUnfurl,
	})
	if s.postReturnErr != nil {
		return "", s.postReturnErr
	}
	ts := s.postReturnTS
	if ts == "" {
		ts = "1700000000.000001"
	}
	return ts, nil
}

func (s *slotSlackFake) UpdateMessage(_ context.Context, channelID string, timestamp string, blocks []goslack.Block, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateCalls = append(s.updateCalls, slotUpdateCall{
		ChannelID: channelID,
		Timestamp: timestamp,
		Text:      text,
		Blocks:    blocks,
	})
	return s.updateReturnErr
}

func (s *slotSlackFake) GetPermalink(_ context.Context, channelID, messageTS string) (string, error) {
	s.mu.Lock()
	s.permalinkCalls = append(s.permalinkCalls, slotPermalinkCall{ChannelID: channelID, MessageTS: messageTS})
	fn := s.permalinkFn
	s.mu.Unlock()
	if fn != nil {
		return fn(channelID, messageTS)
	}
	return "https://slack.test/" + channelID + "/" + messageTS, nil
}

func (s *slotSlackFake) postCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.postCalls)
}

func (s *slotSlackFake) updateCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.updateCalls)
}

// contextBlockText extracts the mrkdwn text from a context block; fails the
// test if the block isn't a single-mrkdwn-element context block.
func contextBlockText(t *testing.T, blocks []goslack.Block, idx int) string {
	t.Helper()
	ctxBlock, ok := blocks[idx].(*goslack.ContextBlock)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, ctxBlock.ContextElements.Elements).Length(1).Required()
	txt, ok := ctxBlock.ContextElements.Elements[0].(*goslack.TextBlockObject)
	gt.Bool(t, ok).True().Required()
	gt.Value(t, txt.Type).Equal(goslack.MarkdownType)
	return txt.Text
}

func TestNotificationSlotCoordinator_FirstEventPosts(t *testing.T) {
	i18n.Init(i18n.LangEN)
	repo := memory.New()
	fake := &slotSlackFake{postReturnTS: "ts-slot"}
	clock := newSlotClock(time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC))

	coord := usecase.NewNotificationSlotCoordinatorForTest(repo.NotificationSlot(), fake, time.Hour, clock.Now)
	gt.Bool(t, usecase.NotificationSlotCoordinatorEnabledForTest(coord)).True()

	ctx := context.Background()
	usecase.EnqueueChannelLineForTest(coord, ctx, "C-room", usecase.SlotEntryForTest{
		ActionMessageTS: "ts-action-1",
		ActionTitle:     "Investigate suspicious login",
		Body:            "status: open -> in_progress",
	})

	gt.Number(t, fake.postCount()).Equal(1)
	gt.Number(t, fake.updateCount()).Equal(0)
	gt.Value(t, fake.postCalls[0].ChannelID).Equal("C-room")
	gt.Bool(t, fake.postCalls[0].DisableUnfurl).True()

	// Fallback text must be empty — Slack otherwise renders it as a
	// duplicate body alongside the Block Kit content.
	gt.Value(t, fake.postCalls[0].Text).Equal("")

	// One context block per Action group; mrkdwn carries linked title + lines.
	gt.Array(t, fake.postCalls[0].Blocks).Length(1).Required()
	body := contextBlockText(t, fake.postCalls[0].Blocks, 0)
	gt.String(t, body).Contains("<https://slack.test/C-room/ts-action-1|Investigate suspicious login>")
	gt.String(t, body).Contains("status: open -> in_progress")
	// No per-row absolute time — timezone differences would confuse readers.
	gt.Bool(t, strings.Contains(body, "10:00")).False()

	slot, err := repo.NotificationSlot().GetActive(ctx, "C-room", clock.Now())
	gt.NoError(t, err).Required()
	gt.Value(t, slot).NotNil().Required()
	gt.Value(t, slot.MessageTS).Equal("ts-slot")
	gt.Array(t, slot.Entries).Length(1).Required()
	gt.Value(t, slot.Entries[0].Body).Equal("status: open -> in_progress")
	gt.Value(t, slot.Entries[0].ActionTitle).Equal("Investigate suspicious login")
	gt.Value(t, slot.Entries[0].ActionPermalink).Equal("https://slack.test/C-room/ts-action-1")
	gt.Bool(t, slot.ExpiresAt.Equal(slot.SlotStart.Add(time.Hour))).True()
}

func TestNotificationSlotCoordinator_SecondEventUpdates(t *testing.T) {
	i18n.Init(i18n.LangEN)
	repo := memory.New()
	fake := &slotSlackFake{postReturnTS: "ts-slot"}
	clock := newSlotClock(time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC))

	coord := usecase.NewNotificationSlotCoordinatorForTest(repo.NotificationSlot(), fake, time.Hour, clock.Now)
	ctx := context.Background()

	// Two events on the same Action collapse into a single context block.
	usecase.EnqueueChannelLineForTest(coord, ctx, "C-room", usecase.SlotEntryForTest{
		ActionMessageTS: "ts-action-1",
		ActionTitle:     "Action A",
		Body:            "first",
	})
	clock.Advance(5 * time.Minute)
	usecase.EnqueueChannelLineForTest(coord, ctx, "C-room", usecase.SlotEntryForTest{
		ActionMessageTS: "ts-action-1",
		ActionTitle:     "Action A",
		Body:            "second",
	})

	gt.Number(t, fake.postCount()).Equal(1)
	gt.Number(t, fake.updateCount()).Equal(1)
	gt.Value(t, fake.updateCalls[0].Timestamp).Equal("ts-slot")
	gt.Value(t, fake.updateCalls[0].Text).Equal("")
	gt.Array(t, fake.updateCalls[0].Blocks).Length(1).Required()

	body := contextBlockText(t, fake.updateCalls[0].Blocks, 0)
	gt.String(t, body).Contains("Action A")
	gt.String(t, body).Contains("first")
	gt.String(t, body).Contains("second")
}

func TestNotificationSlotCoordinator_GroupsByAction(t *testing.T) {
	i18n.Init(i18n.LangEN)
	repo := memory.New()
	fake := &slotSlackFake{postReturnTS: "ts-slot"}
	clock := newSlotClock(time.Date(2026, 5, 16, 0, 46, 0, 0, time.UTC))

	coord := usecase.NewNotificationSlotCoordinatorForTest(repo.NotificationSlot(), fake, time.Hour, clock.Now)
	ctx := context.Background()

	usecase.EnqueueChannelLineForTest(coord, ctx, "C-room", usecase.SlotEntryForTest{
		ActionMessageTS: "ts-A", ActionTitle: "Title A", Body: "status: triage -> analyze",
	})
	clock.Advance(time.Second)
	usecase.EnqueueChannelLineForTest(coord, ctx, "C-room", usecase.SlotEntryForTest{
		ActionMessageTS: "ts-A", ActionTitle: "Title A", Body: "status: analyze -> accepted",
	})
	clock.Advance(time.Minute)
	usecase.EnqueueChannelLineForTest(coord, ctx, "C-room", usecase.SlotEntryForTest{
		ActionMessageTS: "ts-B", ActionTitle: "Title B", Body: "assigned @mizutani",
	})

	last := fake.updateCalls[len(fake.updateCalls)-1]
	// One context block per Action group.
	gt.Array(t, last.Blocks).Length(2).Required()

	groupA := contextBlockText(t, last.Blocks, 0)
	gt.String(t, groupA).Contains("<https://slack.test/C-room/ts-A|Title A>")
	gt.String(t, groupA).Contains("status: triage -> analyze")
	gt.String(t, groupA).Contains("status: analyze -> accepted")
	gt.Bool(t, strings.Contains(groupA, "Title B")).False()

	groupB := contextBlockText(t, last.Blocks, 1)
	gt.String(t, groupB).Contains("<https://slack.test/C-room/ts-B|Title B>")
	gt.String(t, groupB).Contains("assigned @mizutani")
	gt.Bool(t, strings.Contains(groupB, "Title A")).False()
}

func TestNotificationSlotCoordinator_ExpiredRollsOver(t *testing.T) {
	i18n.Init(i18n.LangEN)
	repo := memory.New()
	fake := &slotSlackFake{postReturnTS: "ts-1"}
	clock := newSlotClock(time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC))

	coord := usecase.NewNotificationSlotCoordinatorForTest(repo.NotificationSlot(), fake, 10*time.Minute, clock.Now)
	ctx := context.Background()

	usecase.EnqueueChannelLineForTest(coord, ctx, "C-room", usecase.SlotEntryForTest{
		ActionMessageTS: "ts-A", ActionTitle: "A", Body: "first",
	})
	clock.Advance(11 * time.Minute) // past ExpiresAt
	fake.postReturnTS = "ts-2"
	usecase.EnqueueChannelLineForTest(coord, ctx, "C-room", usecase.SlotEntryForTest{
		ActionMessageTS: "ts-A", ActionTitle: "A", Body: "second",
	})

	gt.Number(t, fake.postCount()).Equal(2)
	gt.Number(t, fake.updateCount()).Equal(0)

	slot, err := repo.NotificationSlot().GetActive(ctx, "C-room", clock.Now())
	gt.NoError(t, err).Required()
	gt.Value(t, slot).NotNil().Required()
	gt.Value(t, slot.MessageTS).Equal("ts-2")
	gt.Array(t, slot.Entries).Length(1).Required()
	gt.Value(t, slot.Entries[0].Body).Equal("second")
}

func TestNotificationSlotCoordinator_DisabledIsNoop(t *testing.T) {
	i18n.Init(i18n.LangEN)
	repo := memory.New()
	fake := &slotSlackFake{}

	coord := usecase.NewNotificationSlotCoordinatorForTest(repo.NotificationSlot(), fake, 0, nil)
	gt.Bool(t, usecase.NotificationSlotCoordinatorEnabledForTest(coord)).False()

	usecase.EnqueueChannelLineForTest(coord, context.Background(), "C-room", usecase.SlotEntryForTest{
		ActionMessageTS: "ts-A", ActionTitle: "A", Body: "ignored",
	})

	gt.Number(t, fake.postCount()).Equal(0)
	gt.Number(t, fake.updateCount()).Equal(0)

	slot, err := repo.NotificationSlot().GetActive(context.Background(), "C-room", time.Now().UTC())
	gt.NoError(t, err).Required()
	gt.Value(t, slot).Nil()
}

func TestNotificationSlotCoordinator_UpdateFailureDropsSlot(t *testing.T) {
	i18n.Init(i18n.LangEN)
	repo := memory.New()
	fake := &slotSlackFake{postReturnTS: "ts-1", updateReturnErr: goerr.New("slack down")}
	clock := newSlotClock(time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC))

	coord := usecase.NewNotificationSlotCoordinatorForTest(repo.NotificationSlot(), fake, time.Hour, clock.Now)
	ctx := context.Background()

	usecase.EnqueueChannelLineForTest(coord, ctx, "C-room", usecase.SlotEntryForTest{
		ActionMessageTS: "ts-A", ActionTitle: "A", Body: "first",
	})
	clock.Advance(time.Minute)
	usecase.EnqueueChannelLineForTest(coord, ctx, "C-room", usecase.SlotEntryForTest{
		ActionMessageTS: "ts-A", ActionTitle: "A", Body: "second",
	})

	gt.Number(t, fake.postCount()).Equal(1)
	gt.Number(t, fake.updateCount()).Equal(1)

	// Slot must be dropped so the next event starts a fresh channel message.
	slot, err := repo.NotificationSlot().GetActive(ctx, "C-room", clock.Now())
	gt.NoError(t, err).Required()
	gt.Value(t, slot).Nil()

	// Third event: a brand-new post should now happen.
	fake.updateReturnErr = nil
	fake.postReturnTS = "ts-2"
	clock.Advance(time.Minute)
	usecase.EnqueueChannelLineForTest(coord, ctx, "C-room", usecase.SlotEntryForTest{
		ActionMessageTS: "ts-A", ActionTitle: "A", Body: "third",
	})

	gt.Number(t, fake.postCount()).Equal(2)
	gt.Number(t, fake.updateCount()).Equal(1)
}

func TestNotificationSlotCoordinator_PermalinkCachedAcrossEntries(t *testing.T) {
	i18n.Init(i18n.LangEN)
	repo := memory.New()
	fake := &slotSlackFake{postReturnTS: "ts-slot"}
	clock := newSlotClock(time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC))
	coord := usecase.NewNotificationSlotCoordinatorForTest(repo.NotificationSlot(), fake, time.Hour, clock.Now)
	ctx := context.Background()

	// Three events on the same Action within one slot must trigger exactly
	// one chat.getPermalink call — subsequent entries reuse the cached
	// permalink from the prior entry.
	for i := 0; i < 3; i++ {
		usecase.EnqueueChannelLineForTest(coord, ctx, "C-room", usecase.SlotEntryForTest{
			ActionMessageTS: "ts-A",
			ActionTitle:     "Action A",
			Body:            "event",
		})
		clock.Advance(time.Second)
	}

	gt.Number(t, len(fake.permalinkCalls)).Equal(1)
	gt.Value(t, fake.permalinkCalls[0].ChannelID).Equal("C-room")
	gt.Value(t, fake.permalinkCalls[0].MessageTS).Equal("ts-A")

	// A different Action in the same slot is a fresh lookup.
	usecase.EnqueueChannelLineForTest(coord, ctx, "C-room", usecase.SlotEntryForTest{
		ActionMessageTS: "ts-B",
		ActionTitle:     "Action B",
		Body:            "other",
	})
	gt.Number(t, len(fake.permalinkCalls)).Equal(2)
	gt.Value(t, fake.permalinkCalls[1].MessageTS).Equal("ts-B")
}

func TestNotificationSlotCoordinator_PermalinkFailureFallsBackToPlainTitle(t *testing.T) {
	i18n.Init(i18n.LangEN)
	repo := memory.New()
	fake := &slotSlackFake{
		postReturnTS: "ts-1",
		permalinkFn:  func(string, string) (string, error) { return "", goerr.New("permalink unavailable") },
	}
	clock := newSlotClock(time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC))
	coord := usecase.NewNotificationSlotCoordinatorForTest(repo.NotificationSlot(), fake, time.Hour, clock.Now)

	usecase.EnqueueChannelLineForTest(coord, context.Background(), "C-room", usecase.SlotEntryForTest{
		ActionMessageTS: "ts-A", ActionTitle: "Plain Title", Body: "first",
	})

	gt.Array(t, fake.postCalls[0].Blocks).Length(1).Required()
	body := contextBlockText(t, fake.postCalls[0].Blocks, 0)
	gt.String(t, body).Contains("Plain Title")
	gt.Bool(t, strings.Contains(body, "<http")).False()
}

func TestBuildSlotBlocks_CapsBlocksToSlackLimit(t *testing.T) {
	i18n.Init(i18n.LangEN)
	now := time.Date(2026, 5, 16, 9, 30, 0, 0, time.UTC)

	// 60 distinct Actions — Slack's 50-block limit must not be exceeded.
	entries := make([]model.NotificationSlotEntry, 0, 60)
	for i := 0; i < 60; i++ {
		ts := "ts-" + strings.Repeat("x", i+1)
		title := "Action " + ts
		entries = append(entries, model.NotificationSlotEntry{
			ActionMessageTS: ts,
			ActionTitle:     title,
			ActionPermalink: "https://slack.test/" + ts,
			Body:            "body",
			EventTime:       now.Add(time.Duration(i) * time.Second),
		})
	}

	blocks := usecase.BuildSlotBlocksForTest(context.Background(), entries)
	// 50 = Slack's hard ceiling.
	gt.Array(t, blocks).Length(50).Required()

	// Most recent Actions win; the oldest 10 (ts-x ... ts-xxxxxxxxxx) were dropped.
	first := contextBlockText(t, blocks, 0)
	gt.String(t, first).Contains("Action ts-" + strings.Repeat("x", 11))
	last := contextBlockText(t, blocks, 49)
	gt.String(t, last).Contains("Action ts-" + strings.Repeat("x", 60))
}

func TestBuildSlotBlocks_TrimsLongGroupText(t *testing.T) {
	i18n.Init(i18n.LangEN)
	now := time.Date(2026, 5, 16, 9, 30, 0, 0, time.UTC)

	// Build many short lines under one Action so the rendered text exceeds
	// Slack's 3000-char-per-text ceiling. Each line is unique so we can tell
	// which got dropped.
	entries := make([]model.NotificationSlotEntry, 0, 200)
	for i := 0; i < 200; i++ {
		entries = append(entries, model.NotificationSlotEntry{
			ActionMessageTS: "ts-A",
			ActionTitle:     "Action A",
			ActionPermalink: "https://slack.test/A",
			Body:            "event-" + strings.Repeat("x", 50) + "-line-" + string(rune('A'+i%26)),
			EventTime:       now.Add(time.Duration(i) * time.Second),
		})
	}

	blocks := usecase.BuildSlotBlocksForTest(context.Background(), entries)
	gt.Array(t, blocks).Length(1).Required()
	body := contextBlockText(t, blocks, 0)

	// Under Slack's 3000-char hard ceiling with headroom.
	gt.Number(t, len(body)).LessOrEqual(2900)
	// Trim happens from the front, so the tail (most recent lines) survives.
	gt.String(t, body).Contains("event-")
	// The Action title block at the very top of the rendered text may be
	// clipped along with the oldest lines — that's the documented tradeoff.
}

func TestNotificationSlotCoordinator_CapsStoredEntries(t *testing.T) {
	i18n.Init(i18n.LangEN)
	repo := memory.New()
	fake := &slotSlackFake{postReturnTS: "ts-slot"}
	clock := newSlotClock(time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC))
	coord := usecase.NewNotificationSlotCoordinatorForTest(repo.NotificationSlot(), fake, time.Hour, clock.Now)
	ctx := context.Background()

	// Feed in well over the storage cap; the persisted slot must stay bounded.
	for i := 0; i < 250; i++ {
		usecase.EnqueueChannelLineForTest(coord, ctx, "C-room", usecase.SlotEntryForTest{
			ActionMessageTS: "ts-A",
			ActionTitle:     "Action A",
			Body:            "event-" + string(rune('A'+i%26)),
		})
		clock.Advance(time.Second)
	}

	slot, err := repo.NotificationSlot().GetActive(ctx, "C-room", clock.Now())
	gt.NoError(t, err).Required()
	gt.Value(t, slot).NotNil().Required()
	gt.Number(t, len(slot.Entries)).LessOrEqual(200)
	gt.Number(t, len(slot.Entries)).GreaterOrEqual(1)
}

func TestBuildSlotBlocks_RendersGroupedContextBlocks(t *testing.T) {
	i18n.Init(i18n.LangEN)
	slotStart := time.Date(2026, 5, 16, 9, 30, 0, 0, time.UTC)
	entries := []model.NotificationSlotEntry{
		{ActionMessageTS: "ts-A", ActionTitle: "Action A", ActionPermalink: "https://slack.test/A", Body: "first", EventTime: slotStart},
		{ActionMessageTS: "ts-A", ActionTitle: "Action A", ActionPermalink: "https://slack.test/A", Body: "second", EventTime: slotStart.Add(time.Minute)},
		{ActionMessageTS: "ts-B", ActionTitle: "Action B", ActionPermalink: "https://slack.test/B", Body: "third", EventTime: slotStart.Add(2 * time.Minute)},
	}

	blocks := usecase.BuildSlotBlocksForTest(context.Background(), entries)

	// One context block per Action group.
	gt.Array(t, blocks).Length(2).Required()

	groupA := contextBlockText(t, blocks, 0)
	gt.String(t, groupA).Contains("<https://slack.test/A|Action A>")
	gt.String(t, groupA).Contains("first")
	gt.String(t, groupA).Contains("second")
	gt.Bool(t, strings.Contains(groupA, "09:30")).False()
	gt.Bool(t, strings.Contains(groupA, "third")).False()

	groupB := contextBlockText(t, blocks, 1)
	gt.String(t, groupB).Contains("<https://slack.test/B|Action B>")
	gt.String(t, groupB).Contains("third")
	gt.Bool(t, strings.Contains(groupB, "09:32")).False()
}

// slotClock is a tiny deterministic clock used by coordinator tests.
type slotClock struct {
	mu  sync.Mutex
	cur time.Time
}

func newSlotClock(start time.Time) *slotClock {
	return &slotClock{cur: start.UTC()}
}

func (c *slotClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cur
}

func (c *slotClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cur = c.cur.Add(d)
}
