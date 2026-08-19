// Package toolsim provides simulated implementations of the agent's tool client
// interfaces. When the agent (its sub-agents) calls a simulated tool, a
// ToolSimulator LLM produces a realistic response from the scenario's
// background description for that tool, and the call is recorded for
// verification and diagnosis.
//
// Coverage in v1: slack_search (SearchService) and notion_search (notiontool
// Client — the one key covers search, page read and database read, since a
// single simulated client serves all three Notion tools). The slack
// MessageRetriever is stubbed to return nothing (recorded).
// github_search is concrete (*githubtool.Client) and is therefore live-only in
// v1 — simulating it would require extracting an interface in production code,
// which is deferred (see the spec limitations). jira_search is likewise
// live-only: it wraps the external gollem-dev/tools/jira ToolSet, which has no
// simulatable interface seam either.
package toolsim

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gollem-dev/gollem"

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
	ToolJiraSearch   = "jira_search"
	ToolWebFetch     = "webfetch"

	// Knowledge tag management tools (in-process, simulatable).
	ToolKnowledgeCreateTag = "knowledge__create_tag"
	ToolKnowledgeUpdateTag = "knowledge__update_tag"
	ToolKnowledgeDeleteTag = "knowledge__delete_tag"
)

// Trajectory names for the Notion client calls that are not one agent tool call
// each. notion__search and notion__get_page map 1:1 onto a client method, so
// they are recorded under the scenario key and the tool name respectively; a
// single notion__get_database call makes TWO client calls (the database, then
// its data source), and recording both under one name would show one agent tool
// call as two entries in the trajectory the judge reads.
const (
	recordNotionGetPage         = "notion_get_page"
	recordNotionGetDatabase     = "notion_get_database"
	recordNotionQueryDataSource = "notion_query_data_source"
)

// SimulatableTools is the catalog of tool names the harness can simulate.
// github_search and jira_search are intentionally absent: both are live-only
// in v1.
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
	return generateWithSchema(ctx, completer, tool, background, query, nil)
}

// generateWithSchema is generate constrained to a JSON response schema. A nil
// schema asks for plain text, which is what every caller but the Notion search
// wants: only that one needs the answer parsed back into typed hits.
func generateWithSchema(ctx context.Context, completer evaltype.Completer, tool, background, query string, schema *gollem.Parameter) (string, error) {
	if strings.TrimSpace(background) == "" {
		return "", nil
	}
	sys := "You simulate the backend of a search tool used by an investigation agent. " +
		"Given the described data this tool can see (the background) and an actual query, " +
		"produce a concise, realistic result the tool would return. Only return information " +
		"consistent with the background; if nothing matches, say so plainly. Do not invent unrelated facts."
	shape := "Return the tool's result as plain text."
	if schema != nil {
		shape = "Return the tool's result as JSON matching the given schema."
	}
	user := fmt.Sprintf("# Tool\n%s\n\n# Background (data this tool can see)\n%s\n\n# Query\n%s\n\n%s", tool, background, query, shape)
	return completer.Complete(ctx, sys, user, schema)
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

// notionSearchHits is the structured shape the simulator asks for so a scenario
// whose background describes a database gets a hit typed "database". A search
// that can only ever answer "page" cannot exercise the notion__get_database
// path at all, which is the path ARGUS-91 was about.
type notionSearchHits struct {
	Items []struct {
		Type  string `json:"type"`
		Title string `json:"title"`
	} `json:"items"`
}

func notionSearchSchema() *gollem.Parameter {
	return &gollem.Parameter{
		Type:        gollem.TypeObject,
		Description: "The pages and databases this Notion search returns.",
		Properties: map[string]*gollem.Parameter{
			"items": {
				Type:        gollem.TypeArray,
				Description: "One entry per hit, most relevant first. Empty when the background describes nothing matching the query.",
				Items: &gollem.Parameter{
					Type: gollem.TypeObject,
					Properties: map[string]*gollem.Parameter{
						"type": {
							Type:        gollem.TypeString,
							Description: "Whether this hit is a Notion page or a Notion database.",
							Required:    true,
							Enum:        []string{"page", "database"},
						},
						"title": {
							Type:        gollem.TypeString,
							Description: "The title of the page or database.",
							Required:    true,
						},
					},
				},
			},
		},
	}
}

func (n *notionSim) Search(ctx context.Context, query string, _ notiontool.SearchOptions) (*notiontool.SearchResult, error) {
	text, err := generateWithSchema(ctx, n.completer, ToolNotionSearch, n.background, query, notionSearchSchema())
	if err != nil {
		return nil, err
	}

	res := &notiontool.SearchResult{Items: notionHitsFrom(text)}
	n.rec.Record(ToolNotionSearch, "sim", map[string]any{"query": query}, text)
	return res, nil
}

// notionHitsFrom converts the simulator's answer into search hits. A reply that
// does not parse as notionSearchHits falls back to one page titled after the
// first line: the simulator LLM is not guaranteed to honour the schema, and a
// scenario that only needs "the agent finds a page" must keep working when it
// does not. The fallback is deliberately the pre-schema behaviour.
func notionHitsFrom(text string) []notiontool.SearchItem {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	var decoded notionSearchHits
	if err := json.Unmarshal([]byte(text), &decoded); err != nil || len(decoded.Items) == 0 {
		return []notiontool.SearchItem{{
			ID:    "sim-page-1",
			Type:  "page",
			Title: firstLine(text),
			URL:   "https://notion.example/sim-page-1",
		}}
	}

	pages, databases := 0, 0
	out := make([]notiontool.SearchItem, 0, len(decoded.Items))
	for _, it := range decoded.Items {
		kind := "page"
		if it.Type == "database" {
			kind = "database"
		}
		// Numbered per kind so the id an agent carries into the next call says
		// which one it came from — a page id reaching notion__get_database (or
		// the reverse) is visible in the trajectory rather than opaque.
		var id string
		if kind == "database" {
			databases++
			id = fmt.Sprintf("sim-database-%d", databases)
		} else {
			pages++
			id = fmt.Sprintf("sim-page-%d", pages)
		}
		out = append(out, notiontool.SearchItem{
			ID:    id,
			Type:  kind,
			Title: it.Title,
			URL:   "https://notion.example/" + id,
		})
	}
	return out
}

func (n *notionSim) GetPageMarkdown(ctx context.Context, pageID string) (*notiontool.PageMarkdown, error) {
	text, err := generate(ctx, n.completer, ToolNotionSearch, n.background, "page content: "+pageID)
	if err != nil {
		return nil, err
	}
	n.rec.Record(recordNotionGetPage, "sim", map[string]any{"page_id": pageID}, text)
	return &notiontool.PageMarkdown{PageID: pageID, Markdown: text}, nil
}

func (n *notionSim) GetDatabase(ctx context.Context, databaseID string) (*notiontool.Database, error) {
	text, err := generate(ctx, n.completer, ToolNotionSearch, n.background, "database title: "+databaseID)
	if err != nil {
		return nil, err
	}
	n.rec.Record(recordNotionGetDatabase, "sim", map[string]any{"database_id": databaseID}, text)
	// One data source, so the tool lists rows without a second round trip. The
	// simulated database is a stand-in for the common Notion shape, not a
	// rehearsal of the ambiguous multi-source case.
	return &notiontool.Database{
		ID:          databaseID,
		Title:       firstLine(text),
		URL:         "https://notion.example/" + databaseID,
		DataSources: []notiontool.DataSourceRef{{ID: databaseID + "-data-source", Name: firstLine(text)}},
	}, nil
}

func (n *notionSim) QueryDataSource(ctx context.Context, dataSourceID string, _ notiontool.QueryOptions) (*notiontool.QueryResult, error) {
	text, err := generate(ctx, n.completer, ToolNotionSearch, n.background, "database rows: "+dataSourceID)
	if err != nil {
		return nil, err
	}
	res := &notiontool.QueryResult{}
	if text != "" {
		res.Items = []notiontool.SearchItem{{
			ID:    "sim-page-1",
			Type:  "page",
			Title: firstLine(text),
			URL:   "https://notion.example/sim-page-1",
		}}
	}
	n.rec.Record(recordNotionQueryDataSource, "sim", map[string]any{"data_source_id": dataSourceID}, text)
	return res, nil
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
	r.rec.Record(recordNotionGetPage, "live", map[string]any{"page_id": pageID}, nil)
	return res, err
}

func (r *recordingNotion) GetDatabase(ctx context.Context, databaseID string) (*notiontool.Database, error) {
	res, err := r.delegate.GetDatabase(ctx, databaseID)
	r.rec.Record(recordNotionGetDatabase, "live", map[string]any{"database_id": databaseID}, nil)
	return res, err
}

func (r *recordingNotion) QueryDataSource(ctx context.Context, dataSourceID string, opts notiontool.QueryOptions) (*notiontool.QueryResult, error) {
	res, err := r.delegate.QueryDataSource(ctx, dataSourceID, opts)
	n := 0
	if res != nil {
		n = len(res.Items)
	}
	r.rec.Record(recordNotionQueryDataSource, "live", map[string]any{"data_source_id": dataSourceID}, fmt.Sprintf("%d rows", n))
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
