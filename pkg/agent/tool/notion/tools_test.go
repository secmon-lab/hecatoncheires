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
	searchResult  *notiontool.SearchResult
	database      *notiontool.Database
	databaseErr   error
	dataSource    *notiontool.DataSource
	dataSourceErr error
	queryResult   *notiontool.QueryResult
	queryErr      error

	gotDatabaseIDs     []string
	gotSchemaDataSrcID []string
	gotDataSourceIDs   []string
	gotQueryOptions    []notiontool.QueryOptions
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

func (f *fakeNotionClient) GetDataSource(_ context.Context, dataSourceID string) (*notiontool.DataSource, error) {
	f.gotSchemaDataSrcID = append(f.gotSchemaDataSrcID, dataSourceID)
	if f.dataSourceErr != nil {
		return nil, f.dataSourceErr
	}
	if f.dataSource != nil {
		return f.dataSource, nil
	}
	return &notiontool.DataSource{ID: dataSourceID}, nil
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

	// gollem rejects a tool whose object parameter declares no properties and
	// whose array parameter declares no items, and it does so when the agent
	// runs rather than at startup.
	t.Run("every tool definition passes gollem's own validation", func(t *testing.T) {
		for _, tl := range notiontool.New(notiontool.Deps{Client: &fakeNotionClient{}}) {
			spec := tl.Spec()
			gt.NoError(t, spec.Validate())
		}
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

		// A choice the caller has to make, which is what this status means.
		gt.Value(t, got["status"]).Equal("data_source_ambiguous")
		gt.Value(t, got["matched"]).Equal(0)
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

		// An id this database does not hold is an argument to correct, not one
		// of the listed ids to choose between — so it is not the ambiguous
		// status, which would send the caller looking through a list its own id
		// is not in.
		gt.Value(t, got["status"]).Equal("invalid_request")
		gt.Value(t, got["matched"]).Equal(0)
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

		// A database with no data sources has no rows: an answer of zero, not
		// something to fix and not a choice to make. Reported as ambiguous, a
		// caller would keep asking for an id that does not exist.
		gt.Value(t, got["status"]).Equal("ok")
		gt.Value(t, got["matched"]).Equal(0)
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

// schemaFake builds a client whose database holds one data source with a small
// but type-varied schema.
func schemaFake(rows *notiontool.QueryResult) *fakeNotionClient {
	return &fakeNotionClient{
		database: &notiontool.Database{
			ID:          "db-1",
			Title:       "Knowledge Base",
			URL:         "https://www.notion.so/db-1",
			DataSources: []notiontool.DataSourceRef{{ID: "ds-1", Name: "Active"}},
		},
		dataSource: &notiontool.DataSource{
			ID:   "ds-1",
			Name: "Active",
			// In name order, as GetDataSource returns them.
			Properties: []notiontool.PropertySchema{
				{ID: "aikw", Name: "Keywords", Type: "multi_select", Options: []string{"network", "storage"}},
				{ID: "title", Name: "Name", Type: "title"},
				{ID: "stat", Name: "Status", Type: "select", Options: []string{"Published", "Draft"}, OptionsTruncated: true},
				{ID: "sumr", Name: "Summary", Type: "rich_text"},
			},
		},
		queryResult: rows,
	}
}

func TestGetDatabaseToolDescribesTheSchema(t *testing.T) {
	rows := &notiontool.QueryResult{Items: []notiontool.SearchItem{{ID: "row-1", Type: "page", Title: "Reset a stuck job"}}}

	newTool := func(c notiontool.Client) gollem.Tool {
		return findTool(t, notiontool.New(notiontool.Deps{Client: c}), "notion__get_database")
	}

	t.Run("reports every property's name and type on the first page", func(t *testing.T) {
		fake := schemaFake(rows)

		got, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotSchemaDataSrcID).Equal([]string{"ds-1"})
		gt.Value(t, got["status"]).Equal("ok")
		gt.Value(t, got["matched"]).Equal(1)

		schema := gt.Cast[[]map[string]any](t, got["property_schema"])
		gt.Array(t, schema).Length(4).Required()
		gt.Value(t, schema[0]["name"]).Equal("Keywords")
		gt.Value(t, schema[0]["type"]).Equal("multi_select")
		gt.Value(t, schema[1]["name"]).Equal("Name")
		gt.Value(t, schema[1]["type"]).Equal("title")
		gt.Value(t, schema[2]["name"]).Equal("Status")
		gt.Value(t, schema[2]["type"]).Equal("select")
		gt.Value(t, schema[3]["name"]).Equal("Summary")
		gt.Value(t, schema[3]["type"]).Equal("rich_text")

		// Choices are withheld until asked for: this data source has four
		// properties, a real one has dozens.
		for _, entry := range schema {
			gt.Value(t, entry["options"]).Nil()
		}

		operators := gt.Cast[map[string][]string](t, got["operators_by_type"])
		gt.Map(t, operators).HasKey("multi_select")
		gt.Map(t, operators).HasKey("rich_text")
		gt.Map(t, operators).HasKey("title")
		gt.Map(t, operators).HasKey("select")
		gt.Array(t, operators["multi_select"]).Equal([]string{"contains", "does_not_contain", "is_empty", "is_not_empty"})
		gt.Array(t, operators["select"]).Equal([]string{"equals", "does_not_equal", "is_empty", "is_not_empty"})
	})

	t.Run("repeats no schema on a later page", func(t *testing.T) {
		fake := schemaFake(rows)

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id":  "db-1",
			"start_cursor": "cursor-2",
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotSchemaDataSrcID).Length(0)
		gt.Value(t, got["property_schema"]).Nil()
		gt.Value(t, got["operators_by_type"]).Nil()
		gt.Value(t, got["status"]).Equal("ok")
	})

	t.Run("reports the choices of the properties asked about", func(t *testing.T) {
		fake := schemaFake(rows)

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id":         "db-1",
			"describe_properties": []any{"Status"},
		})
		gt.NoError(t, err).Required()

		schema := gt.Cast[[]map[string]any](t, got["property_schema"])
		gt.Array(t, schema).Length(4).Required()
		gt.Value(t, schema[0]["options"]).Nil()
		gt.Value(t, schema[2]["name"]).Equal("Status")
		gt.Array(t, gt.Cast[[]string](t, schema[2]["options"])).Equal([]string{"Published", "Draft"})
		gt.Value(t, schema[2]["options_truncated"]).Equal(true)
	})

	t.Run("asks for the schema on a later page when choices are requested", func(t *testing.T) {
		fake := schemaFake(rows)

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id":         "db-1",
			"start_cursor":        "cursor-2",
			"describe_properties": []any{"Keywords"},
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotSchemaDataSrcID).Equal([]string{"ds-1"})
		schema := gt.Cast[[]map[string]any](t, got["property_schema"])
		gt.Array(t, gt.Cast[[]string](t, schema[0]["options"])).Equal([]string{"network", "storage"})
	})

	// A bad property name is the model's own mistake, so it comes back as a
	// result it can repair from — with the schema attached — rather than as a
	// tool failure that the strategies would report to Sentry.
	t.Run("rejects an unknown property name without querying", func(t *testing.T) {
		fake := schemaFake(rows)

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id":         "db-1",
			"describe_properties": []any{"Owner"},
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotDataSourceIDs).Length(0)
		gt.Value(t, got["status"]).Equal("invalid_request")
		gt.Value(t, got["matched"]).Equal(0)
		gt.Array(t, gt.Cast[[]map[string]any](t, got["items"])).Length(0)

		message := gt.Cast[string](t, got["message"])
		gt.String(t, message).Contains("Owner")
		gt.String(t, message).Contains("available properties")
		gt.String(t, message).Contains("Summary")

		gt.Array(t, gt.Cast[[]map[string]any](t, got["property_schema"])).Length(4)
	})

	t.Run("rejects a describe_properties that is not an array", func(t *testing.T) {
		fake := schemaFake(rows)

		got, err := newTool(fake).Run(context.Background(), map[string]any{
			"database_id":         "db-1",
			"describe_properties": "Status",
		})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotSchemaDataSrcID).Length(0)
		gt.Value(t, got["status"]).Equal("invalid_request")
		gt.String(t, gt.Cast[string](t, got["message"])).Contains("must be an array")
	})

	// This one is NOT a rejection: the agent cannot repair a Notion outage or a
	// database it may not read, and reporting it as an empty result would let a
	// caller conclude there is no evidence.
	t.Run("propagates the reason a schema read failed", func(t *testing.T) {
		fake := schemaFake(rows)
		fake.dataSourceErr = goerr.New("notion data source endpoint returned HTTP 403 (restricted_resource)")

		_, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
		gt.Value(t, err).NotNil().Required()
		gt.String(t, err.Error()).Contains("restricted_resource")
		gt.String(t, err.Error()).Contains("failed to fetch notion data source schema")
		gt.Array(t, fake.gotDataSourceIDs).Length(0)
	})

	t.Run("says the data source is ambiguous rather than empty", func(t *testing.T) {
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

		gt.Array(t, fake.gotSchemaDataSrcID).Length(0)
		gt.Value(t, got["status"]).Equal("data_source_ambiguous")
		gt.Value(t, got["matched"]).Equal(0)
	})

	// An empty database is a real answer, and it must not read like a failure.
	t.Run("reports an empty database as a successful zero", func(t *testing.T) {
		fake := schemaFake(&notiontool.QueryResult{})

		got, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
		gt.NoError(t, err).Required()

		gt.Value(t, got["status"]).Equal("ok")
		gt.Value(t, got["matched"]).Equal(0)
		gt.Value(t, got["message"]).Nil()
	})

	// The listing half of this tool is untouched: it narrows nothing, orders
	// nothing and asks for no column values, so Notion receives what it always
	// received.
	t.Run("sends no filter, sorts or column selection", func(t *testing.T) {
		fake := schemaFake(rows)

		_, err := newTool(fake).Run(context.Background(), map[string]any{"database_id": "db-1"})
		gt.NoError(t, err).Required()

		gt.Array(t, fake.gotQueryOptions).Length(1).Required()
		gt.Value(t, fake.gotQueryOptions[0].Filter).Nil()
		gt.Array(t, fake.gotQueryOptions[0].Sorts).Length(0)
		gt.Array(t, fake.gotQueryOptions[0].Properties).Length(0)
	})
}
