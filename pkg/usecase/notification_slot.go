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

// Slack imposes hard limits on chat.postMessage / chat.update payloads:
//   - blocks array: 50 entries max
//   - each Block Kit text object: 3000 characters max
//
// The constants below stay strictly inside those limits with headroom so a
// long-running busy slot never causes Update to fail. When a slot fills past
// these caps the renderer / storage silently drop the oldest content — the
// per-event thread reply is the canonical audit log, not this rolling summary.
const (
	slotMaxEntries        = 200  // storage cap: oldest entries are evicted first
	slotMaxBlocks         = 50   // Slack hard limit on blocks per message
	slotMaxBlockTextChars = 2900 // soft headroom under the 3000-char per-text limit
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
//
// Concurrency: the GetActive → UpdateMessage → Save sequence is intentionally
// non-atomic. Two events for the same channel that race across instances may
// each read the same slot snapshot, append their own entry, and overwrite
// each other in Save — losing at most one rolling-summary line. Per the spec
// (.spec/notification-slot/spec.md) this is the accepted tradeoff: the
// thread-side reply posted by the caller before reaching this method is the
// truth-of-record audit log, the slot is only a best-effort channel rollup,
// and adding distributed locking / leader election was explicitly ruled out.
// Worst case visible to operators is two near-simultaneous aggregate messages
// in the same slot window.
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

	// Reuse a permalink already resolved earlier in this slot for the same
	// Action card before falling back to a fresh chat.getPermalink call —
	// a busy Action that fires many events in one slot would otherwise hit
	// Slack's rate limits unnecessarily.
	permalink := lookupExistingPermalink(slot, in.ActionMessageTS)
	if permalink == "" {
		permalink = c.resolvePermalink(ctx, channelID, in.ActionMessageTS)
	}

	entry := model.NotificationSlotEntry{
		ActionMessageTS: in.ActionMessageTS,
		ActionTitle:     strings.TrimSpace(in.ActionTitle),
		ActionPermalink: permalink,
		Body:            in.Body,
		EventTime:       now,
	}

	if slot == nil {
		c.postNewSlot(ctx, channelID, entry, now)
		return
	}
	c.updateExistingSlot(ctx, slot, entry, now)
}

// lookupExistingPermalink returns the cached permalink for the given Action
// message ts from a prior entry in the same slot, or "" if no usable entry
// exists. Skipping the lookup avoids redundant Slack API calls for an Action
// that fires multiple events inside a single slot window.
func lookupExistingPermalink(slot *model.NotificationSlot, actionMessageTS string) string {
	if slot == nil || actionMessageTS == "" {
		return ""
	}
	for _, e := range slot.Entries {
		if e.ActionMessageTS == actionMessageTS && e.ActionPermalink != "" {
			return e.ActionPermalink
		}
	}
	return ""
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
	// Storage cap: keep only the most recent slotMaxEntries entries so the
	// document doesn't grow unbounded over a long slot and the rendered
	// message stays within Slack's per-message limits.
	if len(slot.Entries) > slotMaxEntries {
		slot.Entries = append([]model.NotificationSlotEntry(nil), slot.Entries[len(slot.Entries)-slotMaxEntries:]...)
	}
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
//
// Slack's hard limits (slotMaxBlocks blocks per message, slotMaxBlockTextChars
// chars per text object) are enforced here as a last line of defence — older
// content is silently dropped when the slot grows too large.
func buildSlotBlocks(ctx context.Context, entries []model.NotificationSlotEntry) []goslack.Block {
	grouping := groupSlotEntries(ctx, entries)
	order := grouping.order
	// Block cap: if too many distinct Actions have accumulated, keep the
	// most recently first-touched groups so the freshest activity wins.
	if len(order) > slotMaxBlocks {
		order = order[len(order)-slotMaxBlocks:]
	}
	blocks := make([]goslack.Block, 0, len(order))
	for _, key := range order {
		bucket := grouping.groups[key]
		text := renderSlotGroupText(bucket)
		text = clampSlotGroupText(text)
		blocks = append(blocks, goslack.NewContextBlock("",
			goslack.NewTextBlockObject(goslack.MarkdownType, text, false, false),
		))
	}
	return blocks
}

// clampSlotGroupText trims a rendered group's mrkdwn so it fits inside Slack's
// per-text 3000-char ceiling with headroom. The trim is done from the front
// (oldest lines first) so the latest events stay visible.
func clampSlotGroupText(text string) string {
	if len(text) <= slotMaxBlockTextChars {
		return text
	}
	// Truncate from the start, then re-align on the next line boundary so
	// we don't render a half-line. Keep at least one line.
	cut := len(text) - slotMaxBlockTextChars
	if nl := strings.IndexByte(text[cut:], '\n'); nl >= 0 {
		cut += nl + 1
	}
	if cut >= len(text) {
		// Pathological case (a single line longer than the limit): keep the
		// tail so the most recent body text is still visible.
		return text[len(text)-slotMaxBlockTextChars:]
	}
	return text[cut:]
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
