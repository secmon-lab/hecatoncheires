// Package toolsim provides simulated implementations of the agent's tool client
// interfaces. When the agent (its sub-agents) calls a simulated tool, a
// ToolSimulator LLM produces a realistic response from the scenario's
// background description for that tool, and the call is recorded for
// verification and diagnosis.
//
// Coverage in v1: slack_search (SearchService) and notion_search (notiontool
// Client). The slack MessageRetriever is stubbed to return nothing (recorded).
// github_search is concrete (*githubtool.Client) and is therefore live-only in
// v1 — simulating it would require extracting an interface in production code,
// which is deferred (see the spec limitations).
package toolsim

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	slackservice "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/evaltype"
)

// Tool names usable in a scenario [tools.*] table that map to simulated clients.
const (
	ToolSlackSearch  = "slack_search"
	ToolNotionSearch = "notion_search"
	ToolGitHubSearch = "github_search"
	ToolWebFetch     = "webfetch"
)

// SimulatableTools is the catalog of tool names the harness can simulate.
// github_search is intentionally absent: it is live-only in v1.
func SimulatableTools() []string {
	return []string{ToolSlackSearch, ToolNotionSearch}
}

// Recorder collects tool-call records across the (parallel) sub-agent calls of
// one run. It is safe for concurrent use.
type Recorder struct {
	mu      sync.Mutex
	seq     int
	records []evaltype.ToolCallRecord
}

// NewRecorder builds an empty Recorder.
func NewRecorder() *Recorder { return &Recorder{} }

// Record appends one tool call and returns its 1-based sequence number.
func (r *Recorder) Record(tool, mode string, args, result any) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	r.records = append(r.records, evaltype.ToolCallRecord{
		Seq:    r.seq,
		Tool:   tool,
		Args:   args,
		Mode:   mode,
		Result: result,
	})
	return r.seq
}

// Records returns a copy of the collected records in call order.
func (r *Recorder) Records() []evaltype.ToolCallRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]evaltype.ToolCallRecord, len(r.records))
	copy(out, r.records)
	return out
}

// generate asks the ToolSimulator LLM to produce a realistic tool response from
// the tool's background and the actual query. Empty background yields an empty
// response (the tool found nothing).
func generate(ctx context.Context, completer evaltype.Completer, tool, background, query string) (string, error) {
	if strings.TrimSpace(background) == "" {
		return "", nil
	}
	sys := "You simulate the backend of a search tool used by an investigation agent. " +
		"Given the described data this tool can see (the background) and an actual query, " +
		"produce a concise, realistic result the tool would return. Only return information " +
		"consistent with the background; if nothing matches, say so plainly. Do not invent unrelated facts."
	user := fmt.Sprintf("# Tool\n%s\n\n# Background (data this tool can see)\n%s\n\n# Query\n%s\n\nReturn the tool's result as plain text.", tool, background, query)
	return completer.Complete(ctx, sys, user, nil)
}

// SlackSearch returns a simulated slacktool.SearchService backed by background.
func SlackSearch(completer evaltype.Completer, background string, rec *Recorder) slacktool.SearchService {
	return &slackSearchSim{completer: completer, background: background, rec: rec}
}

type slackSearchSim struct {
	completer  evaltype.Completer
	background string
	rec        *Recorder
}

func (s *slackSearchSim) SearchMessages(ctx context.Context, query string, _ slacktool.SearchOptions) (*slacktool.SearchResult, error) {
	text, err := generate(ctx, s.completer, ToolSlackSearch, s.background, query)
	if err != nil {
		return nil, err
	}
	res := &slacktool.SearchResult{}
	if text != "" {
		res.Total = 1
		res.Messages = []slacktool.SearchMessage{{
			ChannelID:   "C-SIM",
			ChannelName: "sim",
			UserID:      "U-SIM",
			Username:    "sim",
			Text:        text,
			Timestamp:   "0.0",
		}}
	}
	s.rec.Record(ToolSlackSearch, "sim", map[string]any{"query": query}, text)
	return res, nil
}

// SlackRetriever returns a simulated MessageRetriever that surfaces no extra
// thread messages (the eval thread is synthetic). Calls are recorded.
func SlackRetriever(rec *Recorder) slacktool.MessageRetriever {
	return &slackRetrieverSim{rec: rec}
}

type slackRetrieverSim struct{ rec *Recorder }

func (s *slackRetrieverSim) GetConversationReplies(_ context.Context, channelID, threadTS string, _ int) ([]slackservice.ConversationMessage, error) {
	s.rec.Record("slack_get_replies", "sim", map[string]any{"channel": channelID, "thread_ts": threadTS}, nil)
	return nil, nil
}

func (s *slackRetrieverSim) GetConversationHistory(_ context.Context, channelID string, _ time.Time, _ int) ([]slackservice.ConversationMessage, error) {
	s.rec.Record("slack_get_history", "sim", map[string]any{"channel": channelID}, nil)
	return nil, nil
}

// NotionSearch returns a simulated notiontool.Client backed by background.
func NotionSearch(completer evaltype.Completer, background string, rec *Recorder) notiontool.Client {
	return &notionSim{completer: completer, background: background, rec: rec}
}

type notionSim struct {
	completer  evaltype.Completer
	background string
	rec        *Recorder
}

func (n *notionSim) Search(ctx context.Context, query string, _ notiontool.SearchOptions) (*notiontool.SearchResult, error) {
	text, err := generate(ctx, n.completer, ToolNotionSearch, n.background, query)
	if err != nil {
		return nil, err
	}
	res := &notiontool.SearchResult{}
	if text != "" {
		res.Items = []notiontool.SearchItem{{
			ID:    "sim-page",
			Type:  "page",
			Title: firstLine(text),
			URL:   "https://notion.example/sim-page",
		}}
	}
	n.rec.Record(ToolNotionSearch, "sim", map[string]any{"query": query}, text)
	return res, nil
}

func (n *notionSim) GetPageMarkdown(ctx context.Context, pageID string) (*notiontool.PageMarkdown, error) {
	text, err := generate(ctx, n.completer, ToolNotionSearch, n.background, "page content: "+pageID)
	if err != nil {
		return nil, err
	}
	n.rec.Record("notion_get_page", "sim", map[string]any{"page_id": pageID}, text)
	return &notiontool.PageMarkdown{PageID: pageID, Markdown: text}, nil
}

// RecordingSlackSearch wraps a real SearchService so live calls are also
// captured in the trajectory (FR-12).
func RecordingSlackSearch(delegate slacktool.SearchService, rec *Recorder) slacktool.SearchService {
	return &recordingSlackSearch{delegate: delegate, rec: rec}
}

type recordingSlackSearch struct {
	delegate slacktool.SearchService
	rec      *Recorder
}

func (r *recordingSlackSearch) SearchMessages(ctx context.Context, query string, opts slacktool.SearchOptions) (*slacktool.SearchResult, error) {
	res, err := r.delegate.SearchMessages(ctx, query, opts)
	total := 0
	if res != nil {
		total = res.Total
	}
	r.rec.Record(ToolSlackSearch, "live", map[string]any{"query": query}, fmt.Sprintf("%d results", total))
	return res, err
}

// RecordingNotion wraps a real notiontool.Client so live calls are captured.
func RecordingNotion(delegate notiontool.Client, rec *Recorder) notiontool.Client {
	return &recordingNotion{delegate: delegate, rec: rec}
}

type recordingNotion struct {
	delegate notiontool.Client
	rec      *Recorder
}

func (r *recordingNotion) Search(ctx context.Context, query string, opts notiontool.SearchOptions) (*notiontool.SearchResult, error) {
	res, err := r.delegate.Search(ctx, query, opts)
	n := 0
	if res != nil {
		n = len(res.Items)
	}
	r.rec.Record(ToolNotionSearch, "live", map[string]any{"query": query}, fmt.Sprintf("%d items", n))
	return res, err
}

func (r *recordingNotion) GetPageMarkdown(ctx context.Context, pageID string) (*notiontool.PageMarkdown, error) {
	res, err := r.delegate.GetPageMarkdown(ctx, pageID)
	r.rec.Record("notion_get_page", "live", map[string]any{"page_id": pageID}, nil)
	return res, err
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	r := []rune(s)
	if len(r) > 80 {
		return string(r[:80])
	}
	return s
}
