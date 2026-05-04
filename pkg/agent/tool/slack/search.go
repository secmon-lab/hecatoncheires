package slacktool

import (
	"context"
	"fmt"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	slackservice "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"golang.org/x/sync/errgroup"
)

// searchMessagesTool searches Slack messages workspace-wide via search.messages.
// Requires a User OAuth Token with the search:read scope (provided as SearchService).
type searchMessagesTool struct {
	search SearchService
}

func (t *searchMessagesTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "slack__search_messages",
		Description: "Search Slack messages workspace-wide. Supports Slack search operators such as 'from:@user', 'in:#channel', 'before:YYYY-MM-DD', 'after:YYYY-MM-DD'. Requires a User OAuth Token with search:read scope.",
		Parameters: map[string]*gollem.Parameter{
			"query": {
				Type:        gollem.TypeString,
				Description: "Slack search query. Slack search operators are accepted.",
				Required:    true,
			},
			"count": {
				Type:        gollem.TypeInteger,
				Description: "Number of results to return (1-100, default 20).",
				Required:    false,
			},
			"sort": {
				Type:        gollem.TypeString,
				Description: "Sort order: 'score' (relevance, default) or 'timestamp' (newest first).",
				Required:    false,
				Enum:        []string{"score", "timestamp"},
			},
		},
	}
}

func (t *searchMessagesTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	opts := SearchOptions{}
	if v, err := tool.ExtractInt64(args, "count"); err == nil && v > 0 {
		opts.Count = int(v)
	}
	if s, ok := args["sort"].(string); ok {
		opts.Sort = s
	}

	tool.Update(ctx, fmt.Sprintf("Searching Slack: %s", query))

	res, err := t.search.SearchMessages(ctx, query, opts)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to search slack messages",
			goerr.V("query", query),
		)
	}

	messages := make([]map[string]any, 0, len(res.Messages))
	for _, m := range res.Messages {
		messages = append(messages, map[string]any{
			"channel_id":   m.ChannelID,
			"channel_name": m.ChannelName,
			"user_id":      m.UserID,
			"username":     m.Username,
			"text":         m.Text,
			"ts":           m.Timestamp,
			"permalink":    m.Permalink,
		})
	}

	return map[string]any{
		"total":    res.Total,
		"messages": messages,
	}, nil
}

// getMessagesTool fetches multiple Slack messages and their thread context in
// parallel. Each target is processed independently; partial failures are returned
// per-target rather than aborting the whole call.
type getMessagesTool struct {
	slack slackservice.Service
}

const (
	getMessagesMinTargets   = 1
	getMessagesMaxTargets   = 10
	getMessagesDefaultLimit = 20
	getMessagesMaxLimit     = 200
)

func (t *getMessagesTool) Spec() gollem.ToolSpec {
	minTargets := getMessagesMinTargets
	maxTargets := getMessagesMaxTargets
	return gollem.ToolSpec{
		Name:        "slack__get_messages",
		Description: "Fetch one or more Slack messages and their thread context in bulk (max 10 per call). Each target is fetched in parallel; per-target failures are reported in the response without aborting the whole call.",
		Parameters: map[string]*gollem.Parameter{
			"targets": {
				Type:        gollem.TypeArray,
				Description: "Array of message references. Each element must contain channel_id and ts.",
				Required:    true,
				MinItems:    &minTargets,
				MaxItems:    &maxTargets,
				Items: &gollem.Parameter{
					Type:        gollem.TypeObject,
					Description: "A Slack message reference.",
					Properties: map[string]*gollem.Parameter{
						"channel_id": {
							Type:        gollem.TypeString,
							Description: "Slack channel ID (e.g. C01234567).",
							Required:    true,
						},
						"ts": {
							Type:        gollem.TypeString,
							Description: "Slack message timestamp (e.g. 1700000000.000100).",
							Required:    true,
						},
					},
				},
			},
			"include_thread": {
				Type:        gollem.TypeBoolean,
				Description: "If true (default), return the full thread when ts is a thread root. If false, return only the message itself.",
				Required:    false,
			},
			"thread_limit": {
				Type:        gollem.TypeInteger,
				Description: "Max replies per thread (default 20, max 200).",
				Required:    false,
			},
		},
	}
}

type messageTarget struct {
	channelID string
	ts        string
}

func (t *getMessagesTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	rawTargets, ok := args["targets"].([]any)
	if !ok {
		return nil, fmt.Errorf("targets is required and must be an array")
	}
	if len(rawTargets) < getMessagesMinTargets || len(rawTargets) > getMessagesMaxTargets {
		return nil, fmt.Errorf("targets must contain %d..%d elements, got %d",
			getMessagesMinTargets, getMessagesMaxTargets, len(rawTargets))
	}

	targets := make([]messageTarget, len(rawTargets))
	for i, raw := range rawTargets {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("targets[%d] must be an object", i)
		}
		cid, _ := m["channel_id"].(string)
		ts, _ := m["ts"].(string)
		if cid == "" || ts == "" {
			return nil, fmt.Errorf("targets[%d] requires both channel_id and ts", i)
		}
		targets[i] = messageTarget{channelID: cid, ts: ts}
	}

	includeThread := true
	if v, ok := args["include_thread"].(bool); ok {
		includeThread = v
	}
	threadLimit := getMessagesDefaultLimit
	if v, err := tool.ExtractInt64(args, "thread_limit"); err == nil && v > 0 {
		if v > getMessagesMaxLimit {
			v = getMessagesMaxLimit
		}
		threadLimit = int(v)
	}

	tool.Update(ctx, fmt.Sprintf("Fetching %d Slack message(s)...", len(targets)))

	results := make([]map[string]any, len(targets))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(len(targets))
	for i, tgt := range targets {
		g.Go(func() error {
			results[i] = t.fetchOne(gctx, tgt, includeThread, threadLimit)
			return nil
		})
	}
	_ = g.Wait()

	successCount := 0
	for _, r := range results {
		if _, hasErr := r["error"]; !hasErr {
			successCount++
		}
	}
	if successCount == 0 {
		return nil, goerr.New("all slack message fetches failed",
			goerr.V("count", len(targets)),
		)
	}

	return map[string]any{
		"results": results,
	}, nil
}

func (t *getMessagesTool) fetchOne(ctx context.Context, tgt messageTarget, includeThread bool, threadLimit int) map[string]any {
	out := map[string]any{
		"channel_id": tgt.channelID,
		"ts":         tgt.ts,
	}

	permalink, err := t.slack.GetPermalink(ctx, tgt.channelID, tgt.ts)
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	out["permalink"] = permalink

	limit := threadLimit
	if !includeThread {
		limit = 1
	}
	msgs, err := t.slack.GetConversationReplies(ctx, tgt.channelID, tgt.ts, limit)
	if err != nil {
		out["error"] = err.Error()
		return out
	}

	if !includeThread && len(msgs) > 1 {
		msgs = msgs[:1]
	}

	out["messages"] = convertConversationMessages(msgs)
	return out
}

func convertConversationMessages(msgs []slackservice.ConversationMessage) []map[string]any {
	out := make([]map[string]any, len(msgs))
	for i, m := range msgs {
		out[i] = map[string]any{
			"user_id":   m.UserID,
			"username":  m.UserName,
			"text":      m.Text,
			"ts":        m.Timestamp,
			"thread_ts": m.ThreadTS,
		}
	}
	return out
}
