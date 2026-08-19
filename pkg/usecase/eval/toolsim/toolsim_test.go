package toolsim_test

import (
	"context"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/toolsim"
)

type fakeCompleter struct {
	out string

	gotSchemas []*gollem.Parameter
}

func (f *fakeCompleter) Complete(_ context.Context, _, _ string, schema *gollem.Parameter) (string, error) {
	f.gotSchemas = append(f.gotSchemas, schema)
	return f.out, nil
}

func TestSlackSearch_GeneratesAndRecords(t *testing.T) {
	rec := toolsim.NewRecorder()
	svc := toolsim.SlackSearch(&fakeCompleter{out: "Found 2 threads about portal 503."}, "background here", rec)

	res, err := svc.SearchMessages(context.Background(), "portal 503", slacktool.SearchOptions{})
	gt.NoError(t, err)
	gt.Number(t, res.Total).Equal(1)
	gt.A(t, res.Messages).Length(1)
	gt.V(t, res.Messages[0].Text).Equal("Found 2 threads about portal 503.")

	recs := rec.Records()
	gt.A(t, recs).Length(1)
	gt.V(t, recs[0].Tool).Equal(toolsim.ToolSlackSearch)
	gt.V(t, recs[0].Mode).Equal("sim")
	gt.Number(t, recs[0].Seq).Equal(1)
}

func TestSlackSearch_EmptyBackgroundYieldsNoResults(t *testing.T) {
	rec := toolsim.NewRecorder()
	svc := toolsim.SlackSearch(&fakeCompleter{out: "should not be used"}, "   ", rec)
	res, err := svc.SearchMessages(context.Background(), "q", slacktool.SearchOptions{})
	gt.NoError(t, err)
	gt.Number(t, res.Total).Equal(0)
	gt.A(t, res.Messages).Length(0)
	gt.A(t, rec.Records()).Length(1) // still recorded
}

// The search asks for typed hits so a scenario can describe a Notion database
// and have the agent reach it through notion__get_database — the path ARGUS-91
// was about. A plain-text reply still yields a page, because the simulator LLM
// is not guaranteed to honour the schema.
func TestNotionSearch_TypesEachHit(t *testing.T) {
	rec := toolsim.NewRecorder()
	comp := &fakeCompleter{out: `{"items":[
		{"type":"page","title":"Runbook: Portal Incident Response"},
		{"type":"database","title":"Runbooks"},
		{"type":"page","title":"Portal On-call Rota"}
	]}`}
	cli := toolsim.NotionSearch(comp, "notion bg", rec)

	res, err := cli.Search(context.Background(), "portal", notiontool.SearchOptions{})
	gt.NoError(t, err)
	gt.A(t, res.Items).Length(3).Required()

	gt.V(t, res.Items[0].ID).Equal("sim-page-1")
	gt.V(t, res.Items[0].Type).Equal("page")
	gt.V(t, res.Items[0].Title).Equal("Runbook: Portal Incident Response")
	gt.V(t, res.Items[0].URL).Equal("https://notion.example/sim-page-1")

	gt.V(t, res.Items[1].ID).Equal("sim-database-1")
	gt.V(t, res.Items[1].Type).Equal("database")
	gt.V(t, res.Items[1].Title).Equal("Runbooks")
	gt.V(t, res.Items[1].URL).Equal("https://notion.example/sim-database-1")

	gt.V(t, res.Items[2].ID).Equal("sim-page-2")
	gt.V(t, res.Items[2].Type).Equal("page")
	gt.V(t, res.Items[2].Title).Equal("Portal On-call Rota")

	gt.A(t, comp.gotSchemas).Length(1).Required()
	gt.V(t, comp.gotSchemas[0]).NotNil()

	recs := rec.Records()
	gt.A(t, recs).Length(1).Required()
	gt.V(t, recs[0].Tool).Equal(toolsim.ToolNotionSearch)
	gt.V(t, recs[0].Mode).Equal("sim")
}

func TestNotionSearch_PlainTextReplyYieldsOnePage(t *testing.T) {
	rec := toolsim.NewRecorder()
	cli := toolsim.NotionSearch(&fakeCompleter{out: "Runbook: Portal Incident Response\nsteps..."}, "notion bg", rec)

	res, err := cli.Search(context.Background(), "portal", notiontool.SearchOptions{})
	gt.NoError(t, err)
	gt.A(t, res.Items).Length(1).Required()
	gt.V(t, res.Items[0].ID).Equal("sim-page-1")
	gt.V(t, res.Items[0].Type).Equal("page")
	gt.V(t, res.Items[0].Title).Equal("Runbook: Portal Incident Response")
}

func TestNotionSearch_EmptyBackgroundYieldsNoHits(t *testing.T) {
	rec := toolsim.NewRecorder()
	cli := toolsim.NotionSearch(&fakeCompleter{out: "should not be used"}, "  ", rec)

	res, err := cli.Search(context.Background(), "portal", notiontool.SearchOptions{})
	gt.NoError(t, err)
	gt.A(t, res.Items).Length(0)
	gt.A(t, rec.Records()).Length(1) // still recorded
}

func TestNotionGetPageMarkdown_GeneratesContent(t *testing.T) {
	rec := toolsim.NewRecorder()
	cli := toolsim.NotionSearch(&fakeCompleter{out: "# Runbook\nsteps..."}, "notion bg", rec)

	md, err := cli.GetPageMarkdown(context.Background(), "sim-page-1")
	gt.NoError(t, err)
	gt.V(t, md.PageID).Equal("sim-page-1")
	gt.V(t, md.Markdown).Equal("# Runbook\nsteps...")

	recs := rec.Records()
	gt.A(t, recs).Length(1).Required()
	gt.V(t, recs[0].Tool).Equal("notion_get_page")
	gt.V(t, recs[0].Args).Equal(map[string]any{"page_id": "sim-page-1"})
}

// One notion__get_database call makes two client calls, so each is recorded
// under its own name: collapsing both onto notion_get_database would show one
// agent tool call as two entries in the trajectory the judge reads.
func TestNotionGetDatabase_ListsRowsThroughOneDataSource(t *testing.T) {
	rec := toolsim.NewRecorder()
	cli := toolsim.NotionSearch(&fakeCompleter{out: "Runbooks\nrows..."}, "notion bg", rec)

	db, err := cli.GetDatabase(context.Background(), "sim-database-1")
	gt.NoError(t, err)
	gt.V(t, db.ID).Equal("sim-database-1")
	gt.V(t, db.Title).Equal("Runbooks")
	gt.V(t, db.URL).Equal("https://notion.example/sim-database-1")
	gt.A(t, db.DataSources).Length(1).Required()
	gt.V(t, db.DataSources[0].ID).Equal("sim-database-1-data-source")
	gt.V(t, db.DataSources[0].Name).Equal("Runbooks")

	rows, err := cli.QueryDataSource(context.Background(), db.DataSources[0].ID, notiontool.QueryOptions{})
	gt.NoError(t, err)
	gt.A(t, rows.Items).Length(1).Required()
	gt.V(t, rows.Items[0].ID).Equal("sim-page-1")
	gt.V(t, rows.Items[0].Type).Equal("page")
	gt.V(t, rows.Items[0].Title).Equal("Runbooks")

	recs := rec.Records()
	gt.A(t, recs).Length(2).Required()
	gt.V(t, recs[0].Tool).Equal("notion_get_database")
	gt.V(t, recs[0].Args).Equal(map[string]any{"database_id": "sim-database-1"})
	gt.V(t, recs[1].Tool).Equal("notion_query_data_source")
	gt.V(t, recs[1].Args).Equal(map[string]any{"data_source_id": "sim-database-1-data-source"})
}

// stubNotion is a notiontool.Client with canned answers, used to check what
// RecordingNotion writes into the trajectory around a live client.
type stubNotion struct {
	gotDatabaseID   string
	gotDataSourceID string
}

func (s *stubNotion) Search(context.Context, string, notiontool.SearchOptions) (*notiontool.SearchResult, error) {
	return &notiontool.SearchResult{Items: []notiontool.SearchItem{{ID: "p1", Type: "page"}}}, nil
}

func (s *stubNotion) GetPageMarkdown(_ context.Context, pageID string) (*notiontool.PageMarkdown, error) {
	return &notiontool.PageMarkdown{PageID: pageID, Markdown: "body"}, nil
}

func (s *stubNotion) GetDatabase(_ context.Context, databaseID string) (*notiontool.Database, error) {
	s.gotDatabaseID = databaseID
	return &notiontool.Database{
		ID:          databaseID,
		Title:       "Runbooks",
		DataSources: []notiontool.DataSourceRef{{ID: "ds-1", Name: "Active"}},
	}, nil
}

func (s *stubNotion) QueryDataSource(_ context.Context, dataSourceID string, _ notiontool.QueryOptions) (*notiontool.QueryResult, error) {
	s.gotDataSourceID = dataSourceID
	return &notiontool.QueryResult{Items: []notiontool.SearchItem{
		{ID: "row-1", Type: "page"},
		{ID: "row-2", Type: "page"},
	}}, nil
}

func TestRecordingNotion_RecordsEachLiveCall(t *testing.T) {
	rec := toolsim.NewRecorder()
	stub := &stubNotion{}
	cli := toolsim.RecordingNotion(stub, rec)

	db, err := cli.GetDatabase(context.Background(), "db-1")
	gt.NoError(t, err)
	gt.V(t, db.Title).Equal("Runbooks")
	gt.V(t, stub.gotDatabaseID).Equal("db-1")

	rows, err := cli.QueryDataSource(context.Background(), "ds-1", notiontool.QueryOptions{PageSize: 10})
	gt.NoError(t, err)
	gt.A(t, rows.Items).Length(2)
	gt.V(t, stub.gotDataSourceID).Equal("ds-1")

	recs := rec.Records()
	gt.A(t, recs).Length(2).Required()
	gt.V(t, recs[0].Tool).Equal("notion_get_database")
	gt.V(t, recs[0].Mode).Equal("live")
	gt.V(t, recs[0].Args).Equal(map[string]any{"database_id": "db-1"})
	gt.V(t, recs[1].Tool).Equal("notion_query_data_source")
	gt.V(t, recs[1].Mode).Equal("live")
	gt.V(t, recs[1].Args).Equal(map[string]any{"data_source_id": "ds-1"})
	gt.V(t, recs[1].Result).Equal("2 rows")
}

func TestRecorder_SequenceOrder(t *testing.T) {
	rec := toolsim.NewRecorder()
	rec.Record("a", "sim", nil, nil)
	rec.Record("b", "live", nil, nil)
	recs := rec.Records()
	gt.A(t, recs).Length(2)
	gt.Number(t, recs[0].Seq).Equal(1)
	gt.Number(t, recs[1].Seq).Equal(2)
	gt.V(t, recs[1].Mode).Equal("live")
}

func TestSimulatableTools(t *testing.T) {
	got := toolsim.SimulatableTools()
	gt.A(t, got).Length(2)
}
