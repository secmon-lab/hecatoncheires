package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runNotificationSlotRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	channelID := func() string {
		return fmt.Sprintf("C%d", time.Now().UnixNano())
	}

	t.Run("GetActive returns nil when no slot exists", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		slot, err := repo.NotificationSlot().GetActive(ctx, channelID(), time.Now().UTC())
		gt.NoError(t, err).Required()
		gt.Value(t, slot).Nil()
	})

	t.Run("Save then GetActive roundtrips all fields", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		ch := channelID()

		now := time.Now().UTC().Truncate(time.Microsecond)
		want := &model.NotificationSlot{
			ChannelID: ch,
			MessageTS: "1700000000.000100",
			Entries: []model.NotificationSlotEntry{
				{
					ActionMessageTS: "1700000000.000050",
					ActionTitle:     "Title A",
					ActionPermalink: "https://slack.test/C/000050",
					Body:            "line 1",
					EventTime:       now,
				},
				{
					ActionMessageTS: "1700000000.000050",
					ActionTitle:     "Title A",
					ActionPermalink: "https://slack.test/C/000050",
					Body:            "line 2",
					EventTime:       now.Add(time.Minute),
				},
			},
			SlotStart: now,
			ExpiresAt: now.Add(time.Hour),
			UpdatedAt: now,
		}

		gt.NoError(t, repo.NotificationSlot().Save(ctx, want)).Required()

		got, err := repo.NotificationSlot().GetActive(ctx, ch, now)
		gt.NoError(t, err).Required()
		gt.Value(t, got).NotNil().Required()

		gt.Value(t, got.ChannelID).Equal(want.ChannelID)
		gt.Value(t, got.MessageTS).Equal(want.MessageTS)
		gt.Array(t, got.Entries).Length(2).Required()
		gt.Value(t, got.Entries[0].Body).Equal("line 1")
		gt.Value(t, got.Entries[0].ActionTitle).Equal("Title A")
		gt.Value(t, got.Entries[0].ActionMessageTS).Equal("1700000000.000050")
		gt.Value(t, got.Entries[0].ActionPermalink).Equal("https://slack.test/C/000050")
		gt.Value(t, got.Entries[1].Body).Equal("line 2")
		gt.Bool(t, withinTolerance(got.SlotStart, want.SlotStart, time.Second)).True()
		gt.Bool(t, withinTolerance(got.ExpiresAt, want.ExpiresAt, time.Second)).True()
		gt.Bool(t, withinTolerance(got.UpdatedAt, want.UpdatedAt, time.Second)).True()
	})

	t.Run("GetActive returns nil when slot has expired", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		ch := channelID()

		past := time.Now().UTC().Add(-2 * time.Hour)
		slot := &model.NotificationSlot{
			ChannelID: ch,
			MessageTS: "1700000000.000200",
			Entries: []model.NotificationSlotEntry{
				{ActionMessageTS: "ts-1", ActionTitle: "old", Body: "old body", EventTime: past},
			},
			SlotStart: past,
			ExpiresAt: past.Add(time.Hour), // still in the past
			UpdatedAt: past,
		}
		gt.NoError(t, repo.NotificationSlot().Save(ctx, slot)).Required()

		got, err := repo.NotificationSlot().GetActive(ctx, ch, time.Now().UTC())
		gt.NoError(t, err).Required()
		gt.Value(t, got).Nil()
	})

	t.Run("Save replaces existing slot", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		ch := channelID()
		now := time.Now().UTC()

		first := &model.NotificationSlot{
			ChannelID: ch,
			MessageTS: "ts.first",
			Entries: []model.NotificationSlotEntry{
				{ActionMessageTS: "ts-a", ActionTitle: "A", Body: "a", EventTime: now},
			},
			SlotStart: now,
			ExpiresAt: now.Add(time.Hour),
			UpdatedAt: now,
		}
		gt.NoError(t, repo.NotificationSlot().Save(ctx, first)).Required()

		second := &model.NotificationSlot{
			ChannelID: ch,
			MessageTS: "ts.second",
			Entries: []model.NotificationSlotEntry{
				{ActionMessageTS: "ts-a", ActionTitle: "A", Body: "a", EventTime: now},
				{ActionMessageTS: "ts-a", ActionTitle: "A", Body: "b", EventTime: now.Add(time.Minute)},
			},
			SlotStart: now,
			ExpiresAt: now.Add(time.Hour),
			UpdatedAt: now.Add(time.Minute),
		}
		gt.NoError(t, repo.NotificationSlot().Save(ctx, second)).Required()

		got, err := repo.NotificationSlot().GetActive(ctx, ch, now)
		gt.NoError(t, err).Required()
		gt.Value(t, got).NotNil().Required()
		gt.Value(t, got.MessageTS).Equal("ts.second")
		gt.Array(t, got.Entries).Length(2).Required()
		gt.Value(t, got.Entries[0].Body).Equal("a")
		gt.Value(t, got.Entries[1].Body).Equal("b")
	})

	t.Run("Delete removes the slot", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		ch := channelID()
		now := time.Now().UTC()

		gt.NoError(t, repo.NotificationSlot().Save(ctx, &model.NotificationSlot{
			ChannelID: ch,
			MessageTS: "ts.del",
			Entries: []model.NotificationSlotEntry{
				{ActionMessageTS: "ts-a", ActionTitle: "A", Body: "x", EventTime: now},
			},
			SlotStart: now,
			ExpiresAt: now.Add(time.Hour),
			UpdatedAt: now,
		})).Required()

		gt.NoError(t, repo.NotificationSlot().Delete(ctx, ch)).Required()

		got, err := repo.NotificationSlot().GetActive(ctx, ch, now)
		gt.NoError(t, err).Required()
		gt.Value(t, got).Nil()
	})

	t.Run("Delete on missing channel is no-op", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		gt.NoError(t, repo.NotificationSlot().Delete(ctx, channelID())).Required()
	})

	t.Run("Validation: empty channelID is rejected", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		_, err := repo.NotificationSlot().GetActive(ctx, "", time.Now().UTC())
		gt.Error(t, err)

		err = repo.NotificationSlot().Save(ctx, &model.NotificationSlot{})
		gt.Error(t, err)

		err = repo.NotificationSlot().Delete(ctx, "")
		gt.Error(t, err)
	})
}

func withinTolerance(a, b time.Time, tol time.Duration) bool {
	d := a.Sub(b)
	if d < 0 {
		d = -d
	}
	return d <= tol
}

func TestNotificationSlotRepository_Memory(t *testing.T) {
	runNotificationSlotRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestNotificationSlotRepository_Firestore(t *testing.T) {
	runNotificationSlotRepositoryTest(t, newFirestoreRepository)
}
