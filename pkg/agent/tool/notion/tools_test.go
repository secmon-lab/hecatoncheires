package notiontool_test

import (
	"context"
	"testing"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
)

// fakeNotionClient records what each tool asked for and answers with canned
// data. Every method is driven from a field so a test can shape one call
// without touching the others.
type fakeNotionClient struct {
	searchResult *notiontool.SearchResult
	database     *notiontool.Database
	databaseErr  error
	queryResult  *notiontool.QueryResult
	queryErr     error

	gotDatabaseIDs   []string
	gotDataSourceIDs []string
	gotQueryOptions  []notiontool.QueryOptions
}

func (f *fakeNotionClient) Search(context.Context, string, notiontool.SearchOptions) (*notiontool.SearchResult, error) {
	if f.searchResult != nil {
		return f.searchResult, nil
	}
	return &notiontool.SearchResult{}, nil
}

func (f *fakeNotionClient) GetPageMarkdown(_ context.Context, pageID string) (*notiontool.PageMarkdown, error) {
	return &notiontool.PageMarkdown{PageID: pageID}, nil
}

func (f *fakeNotionClient) GetDatabase(_ context.Context, databaseID string) (*notiontool.Database, error) {
	f.gotDatabaseIDs = append(f.gotDatabaseIDs, databaseID)
	if f.databaseErr != nil {
		return nil, f.databaseErr
	}
	return f.database, nil
}

func (f *fakeNotionClient) QueryDataSource(_ context.Context, dataSourceID string, opts notiontool.QueryOptions) (*notiontool.QueryResult, error) {
	f.gotDataSourceIDs = append(f.gotDataSourceIDs, dataSourceID)
	f.gotQueryOptions = append(f.gotQueryOptions, opts)
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	return f.queryResult, nil
}

func findTool(t *testing.T, tools []gollem.Tool, name string) gollem.Tool {
	t.Helper()
	var found gollem.Tool
	for _, tl := range tools {
		if tl.Spec().Name == name {
			found = tl
		}
	}
	gt.Value(t, found).NotNil().Required()
	return found
}

func TestNew(t *testing.T) {
	t.Run("returns no tools when the client is nil", func(t *testing.T) {
		gt.Array(t, notiontool.New(notiontool.Deps{})).Length(0)
	})

	t.Run("registers search, get_page and get_database", func(t *testing.T) {
		tools := notiontool.New(notiontool.Deps{Client: &fakeNotionClient{}})
		gt.Array(t, tools).Length(3).Required()

		names := make([]string, 0, len(tools))
		for _, tl := range tools {
			names = append(names, tl.Spec().Name)
		}
		gt.Array(t, names).Equal([]string{"notion__search", "notion__get_page", "notion__get_database"})
	})

	// A search hit typed "database" is not readable by notion__get_page, and the
	// model only learns that from these descriptions. Feeding one to the other is
	// what produced a 400 per database found.
	t.Run("search and get_page send a database hit to get_database", func(t *testing.T) {
		tools := notiontool.New(notiontool.Deps{Client: &fakeNotionClient{}})

		search := findTool(t, tools, "notion__search").Spec().Description
		gt.String(t, search).Contains("notion__get_database")
		gt.String(t, search).Contains("notion__get_page")
		gt.String(t, search).Contains("read_tool")

		getPage := findTool(t, tools, "notion__get_page").Spec().Description
		gt.String(t, getPage).Contains("notion__get_database")
	})
}

func TestSearchTool(t *testing.T) {
	// The type of a hit was already reported when the agent sent a database id to
	// notion__get_page anyway, so each hit also names the tool that
	// reads it.
	t.Run("names the reading tool on every hit", func(t *testing.T) {
		fake := &fakeNotionClient{searchResult: &notiontool.SearchResult{
			Items: []notiontool.SearchItem{
				{
					ID:         "page-1",
					Type:       "page",
					Title:      "Incident Playbook",
					URL:        "https://www.notion.so/page-1",
					LastEdited: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
				},
				{
					ID:         "db-1",
					Type:       "database",
					Title:      "Runbooks",
					URL:        "https://www.notion.so/db-1",
					LastEdited: time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC),
				},
			},
			HasMore:    true,
			NextCursor: "cursor-2",
		}}

		tools := notiontool.New(notiontool.Deps{Client: fake})
		got, err := findTool(t, tools, "notion__search").Run(context.Background(), map[string]any{"query": "runbook"})
		gt.NoError(t, err).Required()

		gt.Value(t, got["has_more"]).Equal(true)
		gt.Value(t, got["next_cursor"]).Equal("cursor-2")

		items := gt.Cast[[]map[string]any](t, got["items"])
		gt.Array(t, items).Length(2).Required()

		gt.Value(t, items[0]["id"]).Equal("page-1")
		gt.Value(t, items[0]["type"]).Equal("page")
		gt.Value(t, items[0]["title"]).Equal("Incident Playbook")
		gt.Value(t, items[0]["url"]).Equal("https://www.notion.so/page-1")
		gt.Value(t, items[0]["last_edited"]).Equal("2026-04-01T12:00:00Z")
		gt.Value(t, items[0]["read_tool"]).Equal("notion__get_page")

		gt.Value(t, items[1]["id"]).Equal("db-1")
		gt.Value(t, items[1]["type"]).Equal("database")
		gt.Value(t, items[1]["title"]).Equal("Runbooks")
		gt.Value(t, items[1]["url"]).Equal("https://www.notion.so/db-1")
		gt.Value(t, items[1]["last_edited"]).Equal("2026-04-02T09:00:00Z")
		gt.Value(t, items[1]["read_tool"]).Equal("notion__get_database")
	})

	t.Run("names no reading tool for an unrecognised hit type", func(t *testing.T) {
		fake := &fakeNotionClient{searchResult: &notiontool.SearchResult{
			Items: []notiontool.SearchItem{{ID: "x-1", Type: "data_source", Title: "Rows"}},
		}}

		tools := notiontool.New(notiontool.Deps{Client: fake})
		got, err := findTool(t, tools, "notion__search").Run(context.Background(), map[string]any{"query": "rows"})
		gt.NoError(t, err).Required()

		items := gt.Cast[[]map[string]any](t, got["items"])
		gt.Array(t, items).Length(1).Required()
		gt.Value(t, items[0]["read_tool"]).Equal("")
	})

	t.Run("returns error when query is absent", func(t *testing.T) {
		tools := notiontool.New(notiontool.Deps{Client: &fakeNotionClient{}})
		_, err := findTool(t, tools, "notion__search").Run(context.Background(), map[string]any{})
		gt.Value(t, err).NotNil()
	})
}

func TestGetDatabaseTool(t *testing.T) {
	rows := &notiontool.QueryResult{
		Items: []notiontool.SearchItem{{
			ID:         "row-1",
			Type:       "page",
			Title:      "Restart the ingest worker",
			URL:        "https://www.notion.so/row-1",
			LastEdited: time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC),
		}},
		HasMore:    true,
		NextCursor: "cursor-2",
	}

	newTool := func(c notiontool.Client) gollem.Tool {
		return findTool(t, notiontool.New(notiontool.Deps{Client: c}), "notion__get_database")
	}

	t.Run("lists the rows of the only data source", func(t *testing.T) {
		fake := &fakeNotionClient{
			database: &notiontool.Database{
				ID:          "db-1",
				Title:       "Runbooks",
				URL:         "https://www.notion.so/db-1",
				DataSources: []notiontool.DataSourceRef{{ID: "ds-1", Name: "Active"}},
			},
			queryResult: rows,
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id": "db-1",
			"page_size":   float64(50),
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotDatabaseIDs).Equal([]string{"db-1"})
		gt.Array(t, fake.gotDataSourceIDs).Equal([]string{"ds-1"})
		gt.Array(t, fake.gotQueryOptions).Length(1).Required()
		gt.Number(t, fake.gotQueryOptions[0].PageSize).Equal(50)

		gt.Value(t, got["database_id"]).Equal("db-1")
		gt.Value(t, got["title"]).Equal("Runbooks")
		gt.Value(t, got["url"]).Equal("https://www.notion.so/db-1")
		gt.Value(t, got["data_source_id"]).Equal("ds-1")
		gt.Value(t, got["has_more"]).Equal(true)
		gt.Value(t, got["next_cursor"]).Equal("cursor-2")

		sources := gt.Cast[[]map[string]any](t, got["data_sources"])
		gt.Array(t, sources).Length(1).Required()
		gt.Value(t, sources[0]["id"]).Equal("ds-1")
		gt.Value(t, sources[0]["name"]).Equal("Active")

		items := gt.Cast[[]map[string]any](t, got["items"])
		gt.Array(t, items).Length(1).Required()
		gt.Value(t, items[0]["id"]).Equal("row-1")
		gt.Value(t, items[0]["type"]).Equal("page")
		gt.Value(t, items[0]["title"]).Equal("Restart the ingest worker")
		gt.Value(t, items[0]["url"]).Equal("https://www.notion.so/row-1")
		gt.Value(t, items[0]["last_edited"]).Equal("2026-05-01T08:00:00Z")
	})

	t.Run("queries the requested data source when several exist", func(t *testing.T) {
		fake := &fakeNotionClient{
			database: &notiontool.Database{
				ID: "db-1",
				DataSources: []notiontool.DataSourceRef{
					{ID: "ds-1", Name: "Active"},
					{ID: "ds-2", Name: "Archived"},
				},
			},
			queryResult: rows,
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id":    "db-1",
			"data_source_id": "ds-2",
			"start_cursor":   "cursor-1",
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotDataSourceIDs).Equal([]string{"ds-2"})
		gt.Array(t, fake.gotQueryOptions).Length(1).Required()
		gt.String(t, fake.gotQueryOptions[0].StartCursor).Equal("cursor-1")
		gt.Value(t, got["data_source_id"]).Equal("ds-2")
		gt.Array(t, gt.Cast[[]map[string]any](t, got["items"])).Length(1)
	})

	// The three no-rows cases below are reported as results, not errors: the
	// model can act on each by calling again, and an error would be filed as a
	// tool failure by the strategy that reports them.
	t.Run("reports the choices instead of rows when several data sources exist", func(t *testing.T) {
		fake := &fakeNotionClient{
			database: &notiontool.Database{
				ID: "db-1",
				DataSources: []notiontool.DataSourceRef{
					{ID: "ds-1", Name: "Active"},
					{ID: "ds-2", Name: "Archived"},
				},
			},
			queryResult: rows,
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotDataSourceIDs).Length(0)
		gt.Value(t, got["data_source_id"]).Equal("")
		gt.Array(t, gt.Cast[[]map[string]any](t, got["items"])).Length(0)
		gt.String(t, gt.Cast[string](t, got["message"])).Contains("data_source_id")
		gt.Array(t, gt.Cast[[]map[string]any](t, got["data_sources"])).Length(2)
	})

	t.Run("reports an unknown data_source_id instead of querying it", func(t *testing.T) {
		fake := &fakeNotionClient{
			database: &notiontool.Database{
				ID:          "db-1",
				DataSources: []notiontool.DataSourceRef{{ID: "ds-1", Name: "Active"}},
			},
			queryResult: rows,
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id":    "db-1",
			"data_source_id": "ds-nope",
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotDataSourceIDs).Length(0)
		gt.Value(t, got["data_source_id"]).Equal("")
		gt.Array(t, gt.Cast[[]map[string]any](t, got["items"])).Length(0)
		gt.String(t, gt.Cast[string](t, got["message"])).Contains("not one of this database's data sources")
	})

	t.Run("reports a database that holds no data sources", func(t *testing.T) {
		fake := &fakeNotionClient{
			database:    &notiontool.Database{ID: "db-1", Title: "Empty"},
			queryResult: rows,
		}

		got, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotDataSourceIDs).Length(0)
		gt.Array(t, gt.Cast[[]map[string]any](t, got["items"])).Length(0)
		gt.String(t, gt.Cast[string](t, got["message"])).Contains("no data sources")
	})

	t.Run("returns error when database_id is missing", func(t *testing.T) {
		fake := &fakeNotionClient{}
		_, err := newTool(fake).Run(context.Background(), map[string]any{})
		gt.Value(t, err).NotNil().Required()
		gt.Array(t, fake.gotDatabaseIDs).Length(0)
	})

	t.Run("propagates the reason a database read failed", func(t *testing.T) {
		fake := &fakeNotionClient{databaseErr: goerr.New("notion database endpoint returned HTTP 404 (object_not_found)")}
		_, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("object_not_found")
	})

	t.Run("propagates the reason a data source query failed", func(t *testing.T) {
		fake := &fakeNotionClient{
			database: &notiontool.Database{
				ID:          "db-1",
				DataSources: []notiontool.DataSourceRef{{ID: "ds-1", Name: "Active"}},
			},
			queryErr: goerr.New("notion data source query endpoint returned HTTP 403 (restricted_resource)"),
		}
		_, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("restricted_resource")
		gt.String(t, err.Error()).Contains("failed to query notion data source")
	})
}
