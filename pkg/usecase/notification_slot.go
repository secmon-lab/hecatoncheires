package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	goslack "github.com/slack-go/slack"
)

// notificationSlotCoordinator folds short-burst channel-side notifications
// into a single rolling Slack message ("notification slot"). While a slot is
// active for a channel, new entries are appended via chat.update; once the
// slot expires, the next event posts a fresh channel message and replaces it.
//
// Entries are grouped by their Action's parent message timestamp when
// rendered: each Action gets one section with its title (linked back to the
// Action card) followed by the events for that Action — instead of every
// line repeating "Action X status changed".
//
// Thread-side notifications are still posted by the caller for every event —
// this coordinator only owns the channel side.
type notificationSlotCoordinator struct {
	repo         interfaces.NotificationSlotRepository
	slackService slack.Service
	slotDuration time.Duration
	now          func() time.Time
}

// slotEntry is the input passed by callers (action.go / action_step.go) to
// describe one change event. The coordinator resolves the permalink and
// folds the entry into the active slot.
type slotEntry struct {
	ActionMessageTS string
	ActionTitle     string
	Body            string
}

// newNotificationSlotCoordinator builds the coordinator. When slotDuration is
// non-positive or slackService/repo is nil, enqueueChannelLine becomes a
// no-op (legacy behaviour — channel broadcast is left to the caller).
func newNotificationSlotCoordinator(
	repo interfaces.NotificationSlotRepository,
	slackService slack.Service,
	slotDuration time.Duration,
	now func() time.Time,
) *notificationSlotCoordinator {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &notificationSlotCoordinator{
		repo:         repo,
		slackService: slackService,
		slotDuration: slotDuration,
		now:          now,
	}
}

// enabled reports whether slot aggregation is active. Callers consult this to
// decide whether to strip WithBroadcastToChannel() from their thread post —
// when disabled, they should keep the legacy reply_broadcast behaviour.
func (c *notificationSlotCoordinator) enabled() bool {
	return c != nil && c.slotDuration > 0 && c.slackService != nil && c.repo != nil
}

// enqueueChannelLine appends the entry to the active slot for channelID,
// posting a new aggregated channel message if no active slot exists or the
// previous one has expired. All errors are non-fatal and reported via
// errutil.Handle — thread audit logs are preserved by the caller's prior
// post regardless.
func (c *notificationSlotCoordinator) enqueueChannelLine(ctx context.Context, channelID string, in slotEntry) {
	if !c.enabled() {
		return
	}
	if channelID == "" || in.Body == "" {
		return
	}

	now := c.now().UTC()
	slot, err := c.repo.GetActive(ctx, channelID, now)
	if err != nil {
		errutil.Handle(ctx, err, "failed to load notification slot")
		return
	}

	entry := model.NotificationSlotEntry{
		ActionMessageTS: in.ActionMessageTS,
		ActionTitle:     strings.TrimSpace(in.ActionTitle),
		ActionPermalink: c.resolvePermalink(ctx, channelID, in.ActionMessageTS),
		Body:            in.Body,
		EventTime:       now,
	}

	if slot == nil {
		c.postNewSlot(ctx, channelID, entry, now)
		return
	}
	c.updateExistingSlot(ctx, slot, entry, now)
}

// resolvePermalink looks up the Slack permalink for the parent Action card.
// Best-effort: any failure returns "" and the renderer falls back to a plain
// (unlinked) title. ActionMessageTS may be empty (e.g. system events that
// somehow lack a card); skip the lookup in that case.
func (c *notificationSlotCoordinator) resolvePermalink(ctx context.Context, channelID, messageTS string) string {
	if messageTS == "" {
		return ""
	}
	permalink, err := c.slackService.GetPermalink(ctx, channelID, messageTS)
	if err != nil {
		errutil.Handle(ctx, err, "failed to resolve action permalink for slot entry")
		return ""
	}
	return permalink
}

func (c *notificationSlotCoordinator) postNewSlot(ctx context.Context, channelID string, entry model.NotificationSlotEntry, now time.Time) {
	entries := []model.NotificationSlotEntry{entry}
	blocks := buildSlotBlocks(ctx, entries)

	// Pass an empty fallback text: Slack would otherwise render it as a
	// duplicate body alongside the Block Kit content. The blocks themselves
	// already carry everything readers / notification clients need.
	ts, err := c.slackService.PostMessage(ctx, channelID, blocks, "", slack.WithDisableUnfurl())
	if err != nil {
		errutil.Handle(ctx, err, "failed to post notification slot message")
		return
	}

	slot := &model.NotificationSlot{
		ChannelID: channelID,
		MessageTS: ts,
		Entries:   entries,
		SlotStart: now,
		ExpiresAt: now.Add(c.slotDuration),
		UpdatedAt: now,
	}
	if err := c.repo.Save(ctx, slot); err != nil {
		errutil.Handle(ctx, err, "failed to save new notification slot")
	}
}

func (c *notificationSlotCoordinator) updateExistingSlot(ctx context.Context, slot *model.NotificationSlot, entry model.NotificationSlotEntry, now time.Time) {
	slot.Entries = append(slot.Entries, entry)
	blocks := buildSlotBlocks(ctx, slot.Entries)

	if err := c.slackService.UpdateMessage(ctx, slot.ChannelID, slot.MessageTS, blocks, ""); err != nil {
		errutil.Handle(ctx, err, "failed to update notification slot message")
		if delErr := c.repo.Delete(ctx, slot.ChannelID); delErr != nil {
			errutil.Handle(ctx, delErr, "failed to drop stale notification slot")
		}
		return
	}
	slot.UpdatedAt = now
	if err := c.repo.Save(ctx, slot); err != nil {
		errutil.Handle(ctx, err, "failed to save updated notification slot")
	}
}

// slotGroup is a per-Action bucket built during rendering. Order of first
// appearance is preserved via slotGrouping.order.
type slotGroup struct {
	actionTitle     string
	actionPermalink string
	lines           []string
}

type slotGrouping struct {
	order  []string // grouping keys, insertion order
	groups map[string]*slotGroup
}

func groupSlotEntries(_ context.Context, entries []model.NotificationSlotEntry) slotGrouping {
	g := slotGrouping{groups: make(map[string]*slotGroup)}
	for _, e := range entries {
		key := e.ActionMessageTS
		if key == "" {
			// Anonymous events (no parent Action card) share one bucket so
			// they aren't visually fragmented into per-event sections.
			key = "_anonymous_"
		}
		bucket, ok := g.groups[key]
		if !ok {
			bucket = &slotGroup{}
			g.groups[key] = bucket
			g.order = append(g.order, key)
		}
		// Latest non-empty title / permalink wins, so a mid-slot rename
		// is reflected on the next update.
		if e.ActionTitle != "" {
			bucket.actionTitle = e.ActionTitle
		}
		if e.ActionPermalink != "" {
			bucket.actionPermalink = e.ActionPermalink
		}
		// Render the change body verbatim. We deliberately skip a per-row
		// time prefix: the server records UTC, but readers across multiple
		// timezones would otherwise see a misleading absolute time. Slack
		// already shows the message's own timestamp in the channel UI.
		bucket.lines = append(bucket.lines, e.Body)
	}
	return g
}

// buildSlotBlocks renders the aggregated channel message as one Block Kit
// context block per Action group. Context blocks give the channel a compact,
// de-emphasised look (smaller / grey-ish text) so a busy slot doesn't
// dominate the channel scroll. Kept free of Slack network calls so it can be
// unit-tested without a live service.
func buildSlotBlocks(ctx context.Context, entries []model.NotificationSlotEntry) []goslack.Block {
	grouping := groupSlotEntries(ctx, entries)
	blocks := make([]goslack.Block, 0, len(grouping.order))
	for _, key := range grouping.order {
		bucket := grouping.groups[key]
		text := renderSlotGroupText(bucket)
		blocks = append(blocks, goslack.NewContextBlock("",
			goslack.NewTextBlockObject(goslack.MarkdownType, text, false, false),
		))
	}
	return blocks
}

func renderSlotGroupText(g *slotGroup) string {
	body := strings.Join(g.lines, "\n")
	title := g.actionTitle
	if title == "" {
		return body
	}
	escaped := slackTextEscaper.Replace(title)
	var titleLine string
	if g.actionPermalink != "" {
		titleLine = fmt.Sprintf("*<%s|%s>*", g.actionPermalink, escaped)
	} else {
		titleLine = "*" + escaped + "*"
	}
	return titleLine + "\n" + body
}
